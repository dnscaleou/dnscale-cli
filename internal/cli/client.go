package cli

import (
	"context"
	"net/http"
	"sync"

	dnscale "github.com/dnscaleou/dnscale-go"
	"github.com/spf13/cobra"
)

func (a *app) withClient(cmd *cobra.Command, fn func(context.Context, *dnscale.Client) error) error {
	if a.timeout <= 0 || a.retries < 0 || a.retries > 10 {
		return usageError("timeout must be positive and retries must be between 0 and 10")
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), a.timeout)
	defer cancel()
	cred, err := a.resolveCredentials()
	if err != nil {
		return err
	}
	a.credentials = cred
	base := a.transport
	if base == nil {
		base = http.DefaultTransport.(*http.Transport).Clone()
	}
	a.metadata = &commandTransport{base: base, version: a.version}
	client, err := dnscale.New(dnscale.Options{
		APIKey: cred.key, BaseURL: cred.BaseURL, Timeout: a.timeout,
		MaxRetries: dnscale.Ptr(a.retries), HTTPClient: &http.Client{Transport: a.metadata},
	})
	if err != nil {
		return err
	}
	defer client.Close()
	return fn(ctx, client)
}

// The SDK owns authentication, redirects, and retries. This wrapper only adds
// the CLI user agent and captures metadata discarded by resource wrappers.
type commandTransport struct {
	base      http.RoundTripper
	version   string
	mu        sync.Mutex
	requestID string
}

func (t *commandTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.Header.Set("User-Agent", "dnscale-cli/"+t.version+" "+req.Header.Get("User-Agent"))
	response, err := t.base.RoundTrip(copy)
	t.mu.Lock()
	t.requestID = ""
	if response != nil {
		t.requestID = response.Header.Get("X-Request-ID")
	}
	t.mu.Unlock()
	return response, err
}
func (t *commandTransport) lastRequestID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.requestID
}
func (t *commandTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
