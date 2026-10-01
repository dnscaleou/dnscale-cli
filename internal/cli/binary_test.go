package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	dnscale "github.com/dnscaleou/dnscale-go"
)

func buildTestBinary(t *testing.T) string {
	t.Helper()
	name := "dnscale"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-mod=readonly", "-o", binary, "./cmd/dnscale")
	cmd.Dir = "../.."
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build binary: %v\n%s", err, output)
	}
	return binary
}
func binaryEnvironment(config, base, key string) []string {
	var env []string
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "DNSCALE_") {
			env = append(env, value)
		}
	}
	return append(env, "DNSCALE_CONFIG_DIR="+config, "DNSCALE_BASE_URL="+base, "DNSCALE_API_KEY="+key)
}
func runTestBinary(t *testing.T, binary string, env []string, args ...string) json.RawMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = env
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, &errOut)
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", &errOut)
	}
	var result struct{ Data json.RawMessage }
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid CLI envelope: %v %s", err, &out)
	}
	return result.Data
}

// Run the actual binary through create/list/get/update/delete, retaining the
// IDs returned by each mutation. The fixture keeps both TXT values separately.
func TestCompiledBinaryRecordLifecycle(t *testing.T) {
	var mu sync.Mutex
	records := []map[string]any{}
	sequence := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "binary-fixture")
		respond := func(status int, data any) {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": data})
		}
		if r.Header.Get("Authorization") != "Bearer dnscale_binary-fixture" {
			t.Error("unexpected credential")
		}
		if r.URL.Path == "/v1/zones" && r.Method == "POST" {
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input["region"] != "EU_GLOBAL" || input["type"] != "master" || input["status"] != "active" {
				t.Errorf("zone defaults: %v", input)
			}
			respond(201, map[string]any{"zone": map[string]any{"id": testZone, "name": "example.com"}})
			return
		}
		base := "/v1/zones/" + testZone + "/records"
		if r.URL.Path == base && r.Method == "GET" {
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			end := min(offset+limit, len(records))
			start := min(offset, len(records))
			respond(200, map[string]any{"records": records[start:end], "pagination": map[string]any{"offset": offset, "limit": limit, "count": end - start, "total": len(records), "has_more": end < len(records)}})
			return
		}
		if r.URL.Path == base && r.Method == "POST" {
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			sequence++
			input["id"] = fmt.Sprintf("opaque+%d==", sequence)
			input["name"] = "_demo.example.com."
			input["disabled"] = false
			records = append(records, input)
			respond(201, map[string]any{"record": input})
			return
		}
		if strings.HasPrefix(r.URL.Path, base+"/") {
			id := strings.TrimPrefix(r.URL.Path, base+"/")
			for i, record := range records {
				if record["id"] != id {
					continue
				}
				switch r.Method {
				case "GET":
					respond(200, map[string]any{"record": record})
				case "PUT":
					var input map[string]any
					_ = json.NewDecoder(r.Body).Decode(&input)
					sequence++
					input["id"] = fmt.Sprintf("opaque+%d==", sequence)
					input["name"] = "_demo.example.com."
					input["disabled"] = false
					records[i] = input
					respond(200, map[string]any{"record": input})
				case "DELETE":
					records = append(records[:i], records[i+1:]...)
					w.WriteHeader(204)
				default:
					t.Errorf("unexpected method %s", r.Method)
					w.WriteHeader(405)
				}
				return
			}
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		w.WriteHeader(404)
	}))
	defer server.Close()
	binary := buildTestBinary(t)
	env := binaryEnvironment(t.TempDir(), server.URL+"/v1", "dnscale_binary-fixture")
	runTestBinary(t, binary, env, "zones", "create", "example.com")
	var selected dnscale.Record
	for _, content := range []string{"sibling", "selected"} {
		data := runTestBinary(t, binary, env, "records", "create", testZone, "--name", "_demo", "--type", "TXT", "--content", content, "--ttl", "300")
		if err := json.Unmarshal(data, &selected); err != nil {
			t.Fatal(err)
		}
	}
	var list []dnscale.Record
	data := runTestBinary(t, binary, env, "records", "list", testZone, "--limit", "1", "--all")
	if err := json.Unmarshal(data, &list); err != nil || len(list) != 2 {
		t.Fatalf("listing: %v %s", err, data)
	}
	runTestBinary(t, binary, env, "records", "get", testZone, selected.Id)
	data = runTestBinary(t, binary, env, "records", "update", testZone, selected.Id, "--name", "_demo", "--type", "TXT", "--content", "replacement", "--ttl", "300")
	var updated dnscale.Record
	_ = json.Unmarshal(data, &updated)
	if updated.Id == selected.Id || updated.Id == "" {
		t.Fatal("replacement ID missing")
	}
	runTestBinary(t, binary, env, "records", "get", testZone, updated.Id)
	runTestBinary(t, binary, env, "records", "delete", testZone, updated.Id, "--yes")
	data = runTestBinary(t, binary, env, "records", "list", testZone, "--all")
	if err := json.Unmarshal(data, &list); err != nil || len(list) != 1 || list[0].Content != "sibling" {
		t.Fatalf("sibling lost: %v %s", err, data)
	}
}
