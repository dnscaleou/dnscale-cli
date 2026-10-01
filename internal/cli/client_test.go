package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dnscale "github.com/dnscaleou/dnscale-go"
	"github.com/spf13/cobra"
)

func TestClientUsesSDKPolicyAndCapturesMetadata(t *testing.T) {
	for _, mutation := range []bool{false, true} {
		t.Run(map[bool]string{false: "read retries", true: "write once"}[mutation], func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer dnscale_test-key" || !strings.Contains(r.UserAgent(), "dnscale-cli/test dnscale-go/") {
					t.Error("missing credential or user agent")
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-ID", "request-123")
				if calls == 1 {
					w.WriteHeader(503)
					_, _ = w.Write([]byte(`{"status":"error","error":{"code":"UNAVAILABLE","message":"try later"}}`))
					return
				}
				_, _ = w.Write([]byte(`{"status":"success","data":{"zones":[],"pagination":{"count":0,"limit":10,"offset":0,"total":0,"has_more":false}}}`))
			}))
			defer server.Close()
			h := newHarness(t)
			h.env["DNSCALE_API_KEY"], h.env["DNSCALE_BASE_URL"] = "dnscale_test-key", server.URL+"/v1"
			h.app.timeout, h.app.retries = time.Second, 1
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			err := h.app.withClient(cmd, func(ctx context.Context, client *dnscale.Client) error {
				if mutation {
					_, err := client.Zones.Create(ctx, dnscale.CreateZoneRequest{Name: "example.com"})
					return err
				}
				value, err := client.Zones.List(ctx, dnscale.PageOptions{})
				if err != nil {
					return err
				}
				return h.app.print(value.Zones)
			})
			if mutation {
				if err == nil || calls != 1 {
					t.Fatalf("write: calls=%d error=%v", calls, err)
				}
			} else if err != nil || calls != 2 || !strings.Contains(h.stdout.String(), "request-123") {
				t.Fatalf("read: calls=%d error=%v output=%s", calls, err, &h.stdout)
			}
		})
	}
}

func TestRetryAfterDoesNotInventDelay(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		raw  string
		want *float64
	}{
		{"", nil}, {"invalid", nil}, {"NaN", nil}, {"Inf", nil}, {"0", dnscale.Ptr(0.0)}, {"2", dnscale.Ptr(2.0)}, {now.Add(3 * time.Second).Format(http.TimeFormat), dnscale.Ptr(3.0)},
	} {
		got := retryAfterSeconds(test.raw, now)
		if (got == nil) != (test.want == nil) || (got != nil && *got != *test.want) {
			t.Fatalf("%q: %v", test.raw, got)
		}
	}
}

func TestRedactionPreservesJSONEscapesKeysAndIntegers(t *testing.T) {
	h := newHarness(t)
	h.env["DNSCALE_API_KEY"] = "n"
	if err := h.app.print(map[string]any{"name": "n\n\t", "count": int64(9007199254740993)}); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data struct {
			Name  string
			Count int64
		}
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Name != "[REDACTED]\n\t" || result.Data.Count != 9007199254740993 {
		t.Fatalf("redacted output: %s", &h.stdout)
	}
}
