// Package opencode gives agent-go direct access to the providers configured in
// a local opencode installation, without spawning or connecting to an opencode
// server. It reads opencode's config (opencode.json) and auth.json, then talks
// to each provider's OpenAI-compatible endpoint itself.
package opencode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// AuthEntry mirrors one entry of opencode's auth.json.
type AuthEntry struct {
	Type    string `json:"type"`
	Key     string `json:"key"`
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
	Expires int64  `json:"expires"`
}

// ProviderOptions mirrors the "options" block of an opencode provider.
type ProviderOptions struct {
	BaseURL string            `json:"baseURL"`
	APIKey  string            `json:"apiKey"`
	Headers map[string]string `json:"headers"`
}

// ModelConfig mirrors one entry of a provider's "models" map.
type ModelConfig struct {
	Name    string            `json:"name"`
	Headers map[string]string `json:"headers"`
}

// ProviderConfig mirrors one entry of opencode.json "provider".
type ProviderConfig struct {
	Npm     string                  `json:"npm"`
	Name    string                  `json:"name"`
	Options ProviderOptions         `json:"options"`
	Models  map[string]ModelConfig  `json:"models"`
}

// Config is the subset of opencode.json that matters for chat completions.
type Config struct {
	Model    string                    `json:"model"`
	Provider map[string]ProviderConfig `json:"provider"`
}

// Model is a resolved, ready-to-call opencode model.
type Model struct {
	ID         string // "provider/model"
	ProviderID string
	ModelID    string
	Name       string
	BaseURL    string
	APIKey     string
	Headers    map[string]string
	Protocol   string // "openai" (default) or "anthropic"
}

// Client exposes models configured by opencode plus the auth needed to call them.
type Client struct {
	models       map[string]*Model
	ordered      []*Model
	defaultModel string
	configFiles  []string
	authFiles    []string
	httpClient   *http.Client
}

// New locates and parses the local opencode configuration.
func New() (*Client, error) {
	return NewHTTP("", "", 10*time.Minute)
}

// Models returns every resolved model, sorted by ID.
func (c *Client) Models() []*Model {
	if c == nil {
		return nil
	}
	return c.ordered
}

// DefaultModel returns the model configured as default in opencode.json.
func (c *Client) DefaultModel() string {
	if c == nil {
		return ""
	}
	return c.defaultModel
}

// ConfigFiles returns the opencode.json paths that were read.
func (c *Client) ConfigFiles() []string {
	if c == nil {
		return nil
	}
	return c.configFiles
}

// AuthFiles returns the auth.json paths that were read.
func (c *Client) AuthFiles() []string {
	if c == nil {
		return nil
	}
	return c.authFiles
}

// Resolve accepts "provider/model" or a bare model ID and returns the model.
func (c *Client) Resolve(id string) (*Model, error) {
	if c == nil {
		return nil, fmt.Errorf("opencode client is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("empty model id")
	}
	if m, ok := c.models[id]; ok {
		return m, nil
	}
	if m, ok := c.models[strings.TrimPrefix(id, "opencode/")]; ok {
		return m, nil
	}

	var matches []*Model
	for _, m := range c.ordered {
		if m.ModelID == id || strings.EqualFold(m.Name, id) {
			matches = append(matches, m)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("model %q is not configured in opencode", id)
	default:
		ids := make([]string, 0, len(matches))
		for _, m := range matches {
			ids = append(ids, m.ID)
		}
		return nil, fmt.Errorf("model %q is ambiguous, use one of: %s", id, strings.Join(ids, ", "))
	}
}

func detectProtocol(npm string) string {
	npm = strings.ToLower(npm)
	switch {
	case strings.Contains(npm, "anthropic"):
		return "anthropic"
	case strings.Contains(npm, "google"):
		return "google"
	default:
		return "openai"
	}
}

func mergeHeaders(sets ...map[string]string) map[string]string {
	out := make(map[string]string)
	for _, set := range sets {
		for k, v := range set {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func loadConfig() (*Config, []string, error) {
	paths := configCandidates()
	merged := &Config{Provider: make(map[string]ProviderConfig)}
	var used []string

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, used, fmt.Errorf("read %s: %w", path, err)
		}

		var cfg Config
		if err := json.Unmarshal(stripJSONC(data), &cfg); err != nil {
			return nil, used, fmt.Errorf("parse %s: %w", path, err)
		}
		used = append(used, path)
		if cfg.Model != "" {
			merged.Model = cfg.Model
		}
		for id, pc := range cfg.Provider {
			prev, ok := merged.Provider[id]
			if ok {
				if pc.Npm == "" {
					pc.Npm = prev.Npm
				}
				if pc.Name == "" {
					pc.Name = prev.Name
				}
				if pc.Options.BaseURL == "" {
					pc.Options.BaseURL = prev.Options.BaseURL
				}
				if pc.Options.APIKey == "" {
					pc.Options.APIKey = prev.Options.APIKey
				}
				pc.Options.Headers = mergeHeaders(prev.Options.Headers, pc.Options.Headers)
				if pc.Models == nil {
					pc.Models = make(map[string]ModelConfig)
				}
				for mid, mc := range prev.Models {
					if _, exists := pc.Models[mid]; !exists {
						pc.Models[mid] = mc
					}
				}
			}
			merged.Provider[id] = pc
		}
	}

	if len(used) == 0 {
		return nil, nil, fmt.Errorf("opencode.json not found (looked in: %s)", strings.Join(paths, ", "))
	}
	return merged, used, nil
}

func loadAuth() (map[string]AuthEntry, []string, error) {
	out := make(map[string]AuthEntry)
	var used []string

	for _, path := range authCandidates() {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return out, used, fmt.Errorf("read %s: %w", path, err)
		}
		var entries map[string]AuthEntry
		if err := json.Unmarshal(stripJSONC(data), &entries); err != nil {
			return out, used, fmt.Errorf("parse %s: %w", path, err)
		}
		used = append(used, path)
		for id, entry := range entries {
			out[id] = entry
		}
	}
	return out, used, nil
}

func configCandidates() []string {
	var paths []string
	if p := os.Getenv("OPENCODE_CONFIG"); p != "" {
		paths = append(paths, p)
	}
	for _, dir := range configDirs() {
		paths = append(paths,
			filepath.Join(dir, "opencode.json"),
			filepath.Join(dir, "opencode.jsonc"),
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths,
			filepath.Join(cwd, "opencode.json"),
			filepath.Join(cwd, ".opencode", "opencode.json"),
		)
	}
	return dedupe(paths)
}

func configDirs() []string {
	var dirs []string
	if p := os.Getenv("XDG_CONFIG_HOME"); p != "" {
		dirs = append(dirs, filepath.Join(p, "opencode"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config", "opencode"))
		if runtime.GOOS == "windows" {
			dirs = append(dirs, filepath.Join(home, ".opencode"))
		}
	}
	if runtime.GOOS == "windows" {
		if p := os.Getenv("APPDATA"); p != "" {
			dirs = append(dirs, filepath.Join(p, "opencode"))
		}
	}
	return dedupe(dirs)
}

func authCandidates() []string {
	var paths []string
	if p := os.Getenv("OPENCODE_AUTH"); p != "" {
		paths = append(paths, p)
	}
	for _, dir := range dataDirs() {
		paths = append(paths, filepath.Join(dir, "auth.json"))
	}
	return dedupe(paths)
}

func dataDirs() []string {
	var dirs []string
	if p := os.Getenv("XDG_DATA_HOME"); p != "" {
		dirs = append(dirs, filepath.Join(p, "opencode"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "share", "opencode"))
		dirs = append(dirs, filepath.Join(home, ".opencode"))
	}
	if runtime.GOOS == "windows" {
		if p := os.Getenv("APPDATA"); p != "" {
			dirs = append(dirs, filepath.Join(p, "opencode"))
		}
	}
	return dedupe(dirs)
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		key := strings.ToLower(filepath.Clean(v))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	return out
}

// stripJSONC removes // and /* */ comments plus trailing commas, preserving
// string contents (so "https://..." is not treated as a comment).
func stripJSONC(in []byte) []byte {
	out := make([]byte, 0, len(in))
	inString := false
	escaped := false
	for i := 0; i < len(in); i++ {
		ch := in[i]
		if inString {
			out = append(out, ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		switch {
		case ch == '"':
			inString = true
			out = append(out, ch)
		case ch == '/' && i+1 < len(in) && in[i+1] == '/':
			for i < len(in) && in[i] != '\n' {
				i++
			}
			if i < len(in) {
				out = append(out, '\n')
			}
		case ch == '/' && i+1 < len(in) && in[i+1] == '*':
			i += 2
			for i+1 < len(in) && !(in[i] == '*' && in[i+1] == '/') {
				i++
			}
			i++
		default:
			out = append(out, ch)
		}
	}
	return removeTrailingCommas(out)
}

func removeTrailingCommas(in []byte) []byte {
	out := make([]byte, 0, len(in))
	inString := false
	escaped := false
	for i := 0; i < len(in); i++ {
		ch := in[i]
		if inString {
			out = append(out, ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out = append(out, ch)
			continue
		}
		if ch == ',' {
			j := i + 1
			for j < len(in) && (in[j] == ' ' || in[j] == '\t' || in[j] == '\r' || in[j] == '\n') {
				j++
			}
			if j < len(in) && (in[j] == '}' || in[j] == ']') {
				continue
			}
		}
		out = append(out, ch)
	}
	return out
}
