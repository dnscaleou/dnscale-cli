package cli

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	dnscale "github.com/dnscaleou/dnscale-go"
	"github.com/zalando/go-keyring"
)

const keyringService = "eu.dnscale.cli"

type keyStore interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

type systemKeyStore struct{}

func (systemKeyStore) Get(service, user string) (string, error) { return keyring.Get(service, user) }
func (systemKeyStore) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}
func (systemKeyStore) Delete(service, user string) error { return keyring.Delete(service, user) }

type profile struct {
	BaseURL string `json:"base_url"`
}

type configuration struct {
	Version        int                `json:"version"`
	DefaultProfile string             `json:"default_profile,omitempty"`
	Profiles       map[string]profile `json:"profiles"`
}

var profileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func validProfile(name string) error {
	if !profileName.MatchString(name) {
		return usageError("profile names must contain 1–64 letters, digits, underscores or hyphens, starting with a letter or digit")
	}
	return nil
}

func normalizeBaseURL(value string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "/v1" && u.Path != "/v1/") || u.RawPath != "" || u.Opaque != "" {
		return "", usageError("base URL must end in /v1 with no credentials, query, fragment, or other path")
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || !(strings.EqualFold(u.Hostname(), "localhost") || (ip != nil && ip.IsLoopback())) {
			return "", usageError("base URL must use HTTPS; HTTP is allowed only on loopback for local development")
		}
	}
	u.Path = "/v1"
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

func credentialID(name, baseURL string) string {
	return fmt.Sprintf("%s:%x", name, sha256.Sum256([]byte(baseURL)))
}

func (a *app) configPath() (string, error) {
	dir := strings.TrimSpace(a.getenv("DNSCALE_CONFIG_DIR"))
	if dir == "" {
		base, err := a.userConfigDir()
		if err != nil {
			return "", localError("cannot locate configuration directory; set DNSCALE_CONFIG_DIR")
		}
		dir = filepath.Join(base, "dnscale")
	}
	return filepath.Join(dir, "config.json"), nil
}

func (a *app) readConfig() (*configuration, error) {
	path, err := a.configPath()
	if err != nil {
		return nil, err
	}
	cfg := &configuration{Version: 1, Profiles: make(map[string]profile)}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, localError("cannot read profile configuration")
	}
	cfg = &configuration{}
	if err := json.Unmarshal(data, cfg); err != nil || !strings.HasPrefix(strings.TrimSpace(string(data)), "{") || cfg.Version != 1 || cfg.Profiles == nil {
		return nil, localError("invalid profile configuration; expected version 1 with a profiles object")
	}
	for name, p := range cfg.Profiles {
		if validProfile(name) != nil {
			return nil, localError("invalid profile name in configuration")
		}
		if normalized, err := normalizeBaseURL(p.BaseURL); err != nil || normalized != p.BaseURL {
			return nil, localError("invalid endpoint in profile configuration")
		}
	}
	if cfg.DefaultProfile != "" {
		if _, exists := cfg.Profiles[cfg.DefaultProfile]; !exists {
			return nil, localError("default profile does not exist in configuration")
		}
	}
	return cfg, nil
}

func (a *app) writeConfig(cfg *configuration) error {
	path, err := a.configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return localError("cannot create profile configuration directory")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return localError("cannot create profile configuration file")
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(data, '\n')); err != nil {
		f.Close()
		return localError("cannot write profile configuration")
	}
	if err = f.Close(); err != nil {
		return localError("cannot close profile configuration")
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return localError("cannot replace profile configuration")
	}
	return nil
}

type executionContext struct {
	Profile          string `json:"profile"`
	BaseURL          string `json:"base_url"`
	CredentialSource string `json:"credential_source"`
}

type credentials struct {
	executionContext
	key string
}

func validateKey(key string) error {
	if key == "" || strings.ContainsAny(key, " \t\r\n") || len(key) > 4096 {
		return localError("API key must be a nonempty token without whitespace")
	}
	return nil
}

func (a *app) selectedProfile() string {
	if a.profile != "" {
		return a.profile
	}
	return strings.TrimSpace(a.getenv("DNSCALE_PROFILE"))
}

func (a *app) resolveCredentials() (*credentials, error) {
	name := a.selectedProfile()
	if name == "" {
		if key := strings.TrimSpace(a.getenv("DNSCALE_API_KEY")); key != "" {
			baseURL := a.baseURL
			if baseURL == "" {
				baseURL = strings.TrimSpace(a.getenv("DNSCALE_BASE_URL"))
			}
			if baseURL == "" {
				baseURL = dnscale.DefaultBaseURL
			}
			baseURL, err := normalizeBaseURL(baseURL)
			if err != nil {
				return nil, err
			}
			if err := validateKey(key); err != nil {
				return nil, err
			}
			return &credentials{executionContext{"environment", baseURL, "environment"}, key}, nil
		}
	}
	cfg, err := a.readConfig()
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = cfg.DefaultProfile
	}
	if name == "" {
		return nil, localError("set DNSCALE_API_KEY or run auth login --profile NAME --key-stdin")
	}
	if err := validProfile(name); err != nil {
		return nil, err
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return nil, localError("selected profile does not exist; run auth profiles")
	}
	if a.baseURL != "" {
		u, err := normalizeBaseURL(a.baseURL)
		if err != nil {
			return nil, err
		}
		if u != p.BaseURL {
			return nil, localError("stored credentials are bound to the profile endpoint; create a separate profile for another endpoint")
		}
	}
	key, err := a.keys.Get(keyringService, credentialID(name, p.BaseURL))
	if err != nil {
		return nil, localError("cannot read profile credential from the OS keychain; unlock it or use DNSCALE_API_KEY without an explicit profile")
	}
	if err := validateKey(key); err != nil {
		return nil, err
	}
	return &credentials{executionContext{name, p.BaseURL, "keychain"}, key}, nil
}
