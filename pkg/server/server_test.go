package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	HealthHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperado status 200, obteve %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "PocketWeb Global Search & Packager Online") {
		t.Fatalf("corpo inesperado: %s", body)
	}
}

func TestStaticAndMuxHandler(t *testing.T) {
	h := NewHandler("../../Website")
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("health falhou no mux")
	}
}
