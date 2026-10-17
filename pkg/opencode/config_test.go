package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testConfig = `{
  // default model with a comment
  "model": "codecraft/claude-fable-5.1",
  "provider": {
    "codecraft": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "CodeCraft API",
      "options": {
        "baseURL": "https://codecraftapi.com/v1",
        "headers": { "x-api-key": "from-options" }
      },
      "models": {
        "claude-fable-5.1": { "name": "Claude Fable 5.1" },
        "kimi-k3": { "name": "Kimi K3" },
      }
    }
  }
}`

const testAuth = `{
  "codecraft": { "type": "api", "key": "from-auth" }
}`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestNewHTTPResolvesModels(t *testing.T) {
	cfg := writeTemp(t, "opencode.json", testConfig)
	auth := writeTemp(t, "auth.json", testAuth)

	c, err := NewHTTP(cfg, auth, time.Second)
	if err != nil {
		t.Fatalf("NewHTTP: %v", err)
	}
	if got := c.DefaultModel(); got != "codecraft/claude-fable-5.1" {
		t.Fatalf("DefaultModel = %q", got)
	}
	if len(c.Models()) != 2 {
		t.Fatalf("expected 2 models, got %d", len(c.Models()))
	}

	m, err := c.Resolve("codecraft/kimi-k3")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if m.ModelID != "kimi-k3" || m.BaseURL != "https://codecraftapi.com/v1" {
		t.Fatalf("unexpected model: %+v", m)
	}
	if m.Protocol != "openai" {
		t.Fatalf("protocol = %q", m.Protocol)
	}
	if m.APIKey != "from-auth" {
		t.Fatalf("expected key from auth.json, got %q", m.APIKey)
	}
	if m.Headers["x-api-key"] != "from-options" {
		t.Fatalf("expected headers from options, got %v", m.Headers)
	}
}

func TestResolveBareAndAmbiguous(t *testing.T) {
	cfg := writeTemp(t, "opencode.json", testConfig)
	auth := writeTemp(t, "auth.json", testAuth)
	c, err := NewHTTP(cfg, auth, time.Second)
	if err != nil {
		t.Fatalf("NewHTTP: %v", err)
	}

	if _, err := c.Resolve("kimi-k3"); err != nil {
		t.Fatalf("bare resolve failed: %v", err)
	}
	if _, err := c.Resolve("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown model")
	}
}

func TestStripJSONC(t *testing.T) {
	in := []byte(`{"url":"https://x/y", /* block */ "n": 1, // line` + "\n" + `"m": [1,2,],}`)
	out := stripJSONC(in)
	var v interface{}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("stripped output is not valid JSON: %v\n%s", err, out)
	}
}

func TestSkipProvidersWithoutBaseURL(t *testing.T) {
	cfg := writeTemp(t, "opencode.json", `{
	  "provider": {
	    "named-only": {
	      "npm": "@ai-sdk/openai-compatible",
	      "models": { "x": { "name": "X" } }
	    }
	  }
	}`)
	auth := writeTemp(t, "auth.json", `{}`)
	if _, err := NewHTTP(cfg, auth, time.Second); err == nil {
		t.Fatal("expected error when no provider has a baseURL")
	}
}
