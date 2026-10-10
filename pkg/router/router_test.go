package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRTKCompressToolOutput(t *testing.T) {
	rawInput := "\x1b[31mError:\x1b[0m File not found\n\n\n\nline 1   \nline 2\n"
	compressed := CompressToolOutput(rawInput)

	if strings.Contains(compressed, "\x1b[") {
		t.Fatalf("ANSI escape codes were not removed: %q", compressed)
	}

	if strings.Contains(compressed, "\n\n\n") {
		t.Fatalf("Consecutive newlines were not collapsed: %q", compressed)
	}

	if !strings.Contains(compressed, "Error: File not found") {
		t.Fatalf("Original content lost: %q", compressed)
	}
}

func TestRouterEndpoints(t *testing.T) {
	r := NewRouter()
	handler := r.Handler()

	// 1. Test /v1/health
	req := httptest.NewRequest("GET", "/v1/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on /v1/health, got %d", w.Result().StatusCode)
	}

	// 2. Test /v1/models
	req = httptest.NewRequest("GET", "/v1/models", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on /v1/models, got %d", w.Result().StatusCode)
	}
	if !strings.Contains(w.Body.String(), "moonshotai/kimi-k3") {
		t.Fatalf("missing kimi model in list: %s", w.Body.String())
	}
}
