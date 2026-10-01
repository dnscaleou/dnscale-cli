package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	dnscale "github.com/dnscaleou/dnscale-go"
)

type result struct {
	Data       any               `json:"data"`
	Context    *executionContext `json:"context,omitempty"`
	Pagination *paginationInfo   `json:"pagination,omitempty"`
	RequestID  string            `json:"request_id,omitempty"`
}
type paginationInfo struct {
	Offset     int  `json:"offset"`
	Limit      int  `json:"limit"`
	Returned   int  `json:"returned"`
	Total      int  `json:"total"`
	HasMore    bool `json:"has_more"`
	NextOffset *int `json:"next_offset,omitempty"`
}

func (a *app) encode(out io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	// Redact decoded strings, not raw JSON bytes: a short token must never
	// corrupt an escape sequence such as \n or change a fixed envelope key.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	// Redact the resolved key and ambient key even in echoed response metadata.
	keys := []string{strings.TrimSpace(a.getenv("DNSCALE_API_KEY"))}
	if a.credentials != nil {
		keys = append(keys, a.credentials.key)
	}
	e := json.NewEncoder(out)
	e.SetEscapeHTML(false)
	if !a.compact {
		e.SetIndent("", "  ")
	}
	return e.Encode(redactJSON(decoded, keys))
}

func redactJSON(value any, keys []string) any {
	switch v := value.(type) {
	case string:
		for _, key := range keys {
			if key != "" {
				v = strings.ReplaceAll(v, key, "[REDACTED]")
			}
		}
		return v
	case []any:
		for i := range v {
			v[i] = redactJSON(v[i], keys)
		}
	case map[string]any:
		for key, item := range v {
			v[key] = redactJSON(item, keys)
		}
	}
	return value
}
func (a *app) print(data any) error { return a.printResult(result{Data: data}) }
func (a *app) printResult(value result) error {
	if a.credentials != nil {
		value.Context = &a.credentials.executionContext
	}
	if a.metadata != nil {
		value.RequestID = a.metadata.lastRequestID()
	}
	return a.encode(a.out, value)
}

type errorDetail struct {
	Code              string   `json:"code"`
	Message           string   `json:"message"`
	HTTPStatus        int      `json:"http_status,omitempty"`
	RequestID         string   `json:"request_id,omitempty"`
	RetryAfterSeconds *float64 `json:"retry_after_seconds,omitempty"`
}

func retryAfterSeconds(header string, now time.Time) *float64 {
	if header == "" {
		return nil
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(header), 64)
	if err != nil {
		date, parseErr := http.ParseTime(header)
		if parseErr != nil {
			return nil
		}
		seconds = date.Sub(now).Seconds()
	}
	if math.IsInf(seconds, 0) || math.IsNaN(seconds) {
		return nil
	}
	if seconds < 0 {
		seconds = 0
	}
	return &seconds
}

func (a *app) writeError(err error) int {
	detail := errorDetail{Code: "command_failed", Message: err.Error()}
	exit := 1
	var local *commandError
	var api *dnscale.APIError
	var rate *dnscale.RateLimitError
	var protocol *dnscale.ProtocolError
	var network net.Error
	switch {
	case errors.Is(err, context.Canceled):
		detail.Code, detail.Message, exit = "canceled", "command canceled", 130
	case errors.Is(err, context.DeadlineExceeded):
		detail.Code, detail.Message, exit = "timeout", "command timed out", 5
	case errors.As(err, &local):
		detail.Code, detail.Message, exit = local.code, local.message, local.exit
	case errors.As(err, &api):
		detail.Code, detail.Message = api.Code, api.Message
		detail.HTTPStatus, detail.RequestID = api.StatusCode, api.RequestID
		if api.StatusCode == 401 || api.StatusCode == 403 {
			exit = 3
		}
		if api.StatusCode == 429 {
			exit = 4
		}
		if errors.As(err, &rate) {
			detail.RetryAfterSeconds = retryAfterSeconds(rate.RetryAfter, time.Now())
		}
	case errors.As(err, &protocol):
		detail.Code = "protocol_error"
	case errors.As(err, &network):
		detail.Code, detail.Message, exit = "network_error", "cannot complete API request", 5
		if network.Timeout() {
			detail.Code = "timeout"
		}
	}
	if a.mutation && (exit == 5 || exit == 130 || detail.HTTPStatus >= 500) {
		detail.Message += "; the mutation may have completed, so inspect its outcome before resubmitting"
	}
	var execution *executionContext
	if a.credentials != nil {
		execution = &a.credentials.executionContext
	}
	_ = a.encode(a.errOut, struct {
		Error   errorDetail       `json:"error"`
		Context *executionContext `json:"context,omitempty"`
	}{detail, execution})
	return exit
}
