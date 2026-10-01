package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testZone = "11111111-1111-4111-8111-111111111111"
const otherZone = "22222222-2222-4222-8222-222222222222"

func apiHarness(t *testing.T, handler http.HandlerFunc) (*harness, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	h := newHarness(t)
	h.env["DNSCALE_API_KEY"] = "dnscale_fixture-secret"
	h.env["DNSCALE_BASE_URL"] = server.URL + "/v1"
	return h, server
}
func fixtureJSON(w http.ResponseWriter, status int, data string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", "fixture-request")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, data)
}
func zoneFixture(id, name string) string {
	return fmt.Sprintf(`{"id":%q,"name":%q,"region":"EU_GLOBAL","type":"master","status":"active"}`, id, name)
}
func recordFixture(id, content string) string {
	return fmt.Sprintf(`{"id":%q,"name":"_demo.example.com.","type":"TXT","content":%q,"ttl":300,"disabled":false}`, id, content)
}
func successRecord(id, content string) string {
	return `{"status":"success","data":{"record":` + recordFixture(id, content) + `}}`
}
func pageFixture(key, items string, offset, limit, count, total int, more bool) string {
	return fmt.Sprintf(`{"status":"success","data":{%q:%s,"pagination":{"offset":%d,"limit":%d,"count":%d,"total":%d,"has_more":%t}}}`, key, items, offset, limit, count, total, more)
}

func TestRecordInputIsValidatedOfflineBeforeCredentials(t *testing.T) {
	for _, test := range []struct {
		input string
		extra []string
		code  int
	}{
		{`{"name":"_demo","type":"TXT","content":"value","disabled":false,"ttl":0,"priority":0,"comment":null}`, nil, 0},
		{`{"name":"_demo","type":"TXT","content":"value","priority":null,"comment":""}`, nil, 0},
		{`{"name":"_demo","type":"TXT","content":"value","ttl":120}`, nil, 0},
		{`{"name":"_demo","type":"TXT","content":"value","surprise":true}`, nil, 2},
		{`{"name":"_demo","type":"TXT","content":"value"} {}`, nil, 2},
		{`{"name":"_demo","type":"TXT","content":" value"}`, nil, 2},
		{`{"name":"_demo","type":"A","content":"192.0.2.1","ttl":-1}`, nil, 2},
		{`null`, nil, 2}, {`[]`, nil, 2}, {`{}`, nil, 2},
		{strings.Repeat(" ", maxRecordInput+1), nil, 2},
		{`{"name":"x","type":"TXT","content":"x"}`, []string{"--disabled=false"}, 2},
	} {
		h := newHarness(t)
		h.keys.err = fmt.Errorf("keychain must not be used")
		args := append([]string{"records", "create", "example.com", "--file", "-", "--dry-run", "--json"}, test.extra...)
		if code := h.run(test.input, args...); code != test.code {
			t.Fatalf("input %.150s: exit %d: %s", test.input, code, &h.stderr)
		}
		if test.code == 0 && (!strings.Contains(h.stdout.String(), `"submitted":false`) || strings.Contains(h.stdout.String(), `"context"`)) {
			t.Fatalf("dry-run: %s", &h.stdout)
		}
	}
	h := newHarness(t)
	for _, args := range [][]string{
		{"records", "create", testZone, "--file", "missing-file.json"},
		{"records", "delete", testZone, "opaque-id"},
		{"records", "update", testZone, "opaque-id", "--content", "new"},
		{"zones", "list", "--limit", "101"}, {"records", "list", testZone, "--limit", "1001"},
		{"zones", "create", "example.com", "--region", "invalid"},
		{"usage", "summary", "--month", "2026-99"},
		{"usage", "zone", testZone, "--start-date", "2026-10-02", "--end-date", "2026-10-01"},
	} {
		if code := h.run("", args...); code != 2 || !strings.Contains(h.stderr.String(), "usage_error") {
			t.Fatalf("%v: %d %s", args, code, &h.stderr)
		}
	}
}

func TestRecordWireValuesAndReplacementID(t *testing.T) {
	for _, input := range []string{
		`{"name":"_demo","type":"TXT","content":"new","ttl":0,"disabled":false,"priority":0,"comment":null}`,
		`{"name":"_demo","type":"TXT","content":"new","priority":null,"comment":""}`,
		`{"name":"_demo","type":"TXT","content":"new"}`,
	} {
		calls := 0
		h, _ := apiHarness(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Method != "PUT" || r.URL.Path != "/v1/zones/"+testZone+"/records/old+opaque==" {
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
			var got, want map[string]any
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Error(err)
			}
			_ = json.Unmarshal([]byte(input), &want)
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("wire payload: %s, want %s", gotJSON, wantJSON)
			}
			fixtureJSON(w, 200, successRecord("new+opaque==", "new"))
		})
		if code := h.run(input, "records", "update", testZone, "old+opaque==", "--file", "-", "--json"); code != 0 || calls != 1 || !strings.Contains(h.stdout.String(), `"id":"new+opaque=="`) {
			t.Fatalf("update: %d %s %s", code, &h.stdout, &h.stderr)
		}
	}
}

func TestPaginationAndExactZoneResolution(t *testing.T) {
	var requests []string
	h, _ := apiHarness(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		switch r.URL.Path {
		case "/v1/zones":
			if r.URL.Query().Get("offset") == "0" {
				fixtureJSON(w, 200, pageFixture("zones", "["+zoneFixture(otherZone, "other.example")+"]", 0, 100, 1, 2, true))
			} else {
				fixtureJSON(w, 200, pageFixture("zones", "["+zoneFixture(testZone, "EXAMPLE.COM.")+"]", 1, 100, 1, 2, false))
			}
		case "/v1/zones/" + testZone + "/records":
			if r.URL.Query().Get("offset") == "0" {
				fixtureJSON(w, 200, pageFixture("records", "["+recordFixture("one", "first")+"]", 0, 2, 1, 2, true))
			} else {
				fixtureJSON(w, 200, pageFixture("records", "["+recordFixture("two", "second")+"]", 1, 2, 1, 2, false))
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	if code := h.run("", "records", "list", "example.com", "--limit", "2", "--all", "--json"); code != 0 {
		t.Fatalf("exit %d: %s", code, &h.stderr)
	}
	var output struct {
		Data       []map[string]any
		Pagination paginationInfo
		RequestID  string `json:"request_id"`
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 4 || len(output.Data) != 2 || output.Pagination.Returned != 2 || output.Pagination.HasMore || output.RequestID != "fixture-request" {
		t.Fatalf("output %s requests %v", &h.stdout, requests)
	}
}

func TestPaginationRejectsInconsistentAndRepeatedPages(t *testing.T) {
	for _, mode := range []string{"wrong offset", "wrong count", "no progress", "repeated ID", "wrong total", "null list"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			h, _ := apiHarness(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				offset, count, total, more, items := 0, 1, 1, false, "["+zoneFixture(testZone, "example.com")+"]"
				switch mode {
				case "wrong offset":
					offset = 3
				case "wrong count":
					count = 0
				case "no progress":
					count = 0
					items = "[]"
					more = true
				case "wrong total":
					total = 0
				case "null list":
					items = "null"
					count = 0
					total = 0
				case "repeated ID":
					total = 2
					more = calls == 1
					if calls > 1 {
						offset = 1
					}
				}
				fixtureJSON(w, 200, pageFixture("zones", items, offset, 50, count, total, more))
			})
			if code := h.run("", "zones", "list", "--all"); code != 1 || h.stdout.Len() != 0 || calls > 2 {
				t.Fatalf("code %d, calls %d, output %s, error %s", code, calls, &h.stdout, &h.stderr)
			}
		})
	}
}

func TestMissingAndAmbiguousZoneNames(t *testing.T) {
	for _, items := range []string{"[]", "[" + zoneFixture(testZone, "example.com") + "," + zoneFixture(otherZone, "example.com.") + "]"} {
		count := 0
		if items != "[]" {
			count = 2
		}
		h, _ := apiHarness(t, func(w http.ResponseWriter, r *http.Request) {
			fixtureJSON(w, 200, pageFixture("zones", items, 0, 100, count, count, false))
		})
		if code := h.run("", "zones", "get", "example.com"); code != 1 {
			t.Fatalf("exit %d", code)
		}
		want := "not found among accessible zones"
		if count == 2 {
			want = "ambiguous_zone"
		}
		if !strings.Contains(h.stderr.String(), want) {
			t.Fatalf("error %s", &h.stderr)
		}
	}
}

func TestDNSSECAndUsageContracts(t *testing.T) {
	usage := fmt.Sprintf(`{"customer_id":%q,"billing_month":"2026-10-01T00:00:00Z","total_queries":10}`, testZone)
	for _, test := range []struct {
		args              []string
		path, query, data string
		code              int
	}{
		{[]string{"dnssec", "status", testZone}, "/zones/" + testZone + "/dnssec/status", "", `{"dnssec":false}`, 0},
		{[]string{"dnssec", "ds", testZone}, "/zones/" + testZone + "/dnssec/ds", "", `{"ds_records":null}`, 0},
		{[]string{"dnssec", "ds", testZone}, "/zones/" + testZone + "/dnssec/ds", "", `{"ds_records":["12345 13 2 ABCDEF"]}`, 0},
		{[]string{"usage", "current"}, "/usage/current", "", usage, 0},
		{[]string{"usage", "summary", "--month", "2026-10"}, "/usage/summary", "month=2026-10", usage, 0},
		{[]string{"usage", "zone", testZone, "--start-date", "2026-10-01", "--end-date", "2026-10-02"}, "/zones/" + testZone + "/usage", "end_date=2026-10-02&start_date=2026-10-01", `{"zone_name":"example.com","period_start":"2026-10-01T00:00:00Z","period_end":"2026-10-02T00:00:00Z","total_queries":0}`, 0},
		{[]string{"dnssec", "status", testZone}, "/zones/" + testZone + "/dnssec/status", "", `{}`, 1},
		{[]string{"usage", "current"}, "/usage/current", "", `{}`, 1},
	} {
		t.Run(strings.Join(test.args, " ")+test.data, func(t *testing.T) {
			h, _ := apiHarness(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1"+test.path || r.URL.Query().Encode() != test.query {
					t.Errorf("request %s", r.URL.String())
				}
				fixtureJSON(w, 200, `{"status":"success","data":`+test.data+`}`)
			})
			if code := h.run("", test.args...); code != test.code {
				t.Fatalf("exit %d: %s", code, &h.stderr)
			}
		})
	}
}

func TestErrorExitCodesMetadataAndRedaction(t *testing.T) {
	for _, test := range []struct {
		status, exit int
		body         string
	}{
		{401, 3, `{"error":"Authorization header is required"}`},
		{403, 3, `{"status":"error","error":{"code":"API_KEY_RECORD_SCOPE_DENIED","message":"dnscale_fixture-secret cannot access this owner"}}`},
		{429, 4, `{"status":"error","error":{"code":"RATE_LIMITED","message":"wait"}}`},
		{502, 1, `<html>gateway</html>`},
		{200, 1, `{"status":"success","data":{}}`},
	} {
		h, _ := apiHarness(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "0")
			fixtureJSON(w, test.status, test.body)
		})
		if code := h.run("", "zones", "get", testZone, "--retries", "0", "--json"); code != test.exit {
			t.Fatalf("status %d: code %d error %s", test.status, code, &h.stderr)
		}
		if h.stdout.Len() != 0 || strings.Contains(h.stderr.String(), "dnscale_fixture-secret") {
			t.Fatal("error leaked data to stdout or a credential to stderr")
		}
		if test.status != 200 && !strings.Contains(h.stderr.String(), `"request_id":"fixture-request"`) {
			t.Fatalf("lost metadata: %s", &h.stderr)
		}
		if test.status == 429 && !strings.Contains(h.stderr.String(), `"retry_after_seconds":0`) {
			t.Fatalf("lost explicit retry delay: %s", &h.stderr)
		}
	}
}

func TestRedirectDeadlineCancellationAndBrokenOutput(t *testing.T) {
	t.Run("redirect", func(t *testing.T) {
		forwarded := 0
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded++ }))
		defer target.Close()
		h, _ := apiHarness(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) })
		if code := h.run("", "zones", "list"); code != 1 || forwarded != 0 {
			t.Fatalf("redirect: %d forwarded=%d", code, forwarded)
		}
	})
	t.Run("whole command deadline", func(t *testing.T) {
		h, _ := apiHarness(t, func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(60 * time.Millisecond):
			}
			if r.URL.Path == "/v1/zones" {
				fixtureJSON(w, 200, pageFixture("zones", "["+zoneFixture(testZone, "example.com")+"]", 0, 100, 1, 1, false))
			} else {
				fixtureJSON(w, 200, `{"status":"success","data":{"zone":`+zoneFixture(testZone, "example.com")+`}}`)
			}
		})
		if code := h.run("", "zones", "get", "example.com", "--timeout", "90ms", "--retries", "0"); code != 5 {
			t.Fatalf("deadline: %d %s", code, &h.stderr)
		}
	})
	t.Run("canceled", func(t *testing.T) {
		h := newHarness(t)
		h.env["DNSCALE_API_KEY"] = "dnscale_test"
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if code := h.app.execute(ctx, []string{"zones", "list"}); code != 130 {
			t.Fatalf("cancel: %d %s", code, &h.stderr)
		}
	})
	t.Run("output failure", func(t *testing.T) {
		h := newHarness(t)
		h.app.out = failingWriter{}
		if code := h.run("", "records", "create", testZone, "--name", "x", "--type", "A", "--content", "192.0.2.1", "--dry-run"); code != 1 {
			t.Fatalf("output failure: %d", code)
		}
	})
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
