package agent

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookHandlerAsync(t *testing.T) {
	h := WebhookHandler()

	payload := `{"event":"alert_bot_offline","source":"uptimekuma","prompt":"Verifique porta 3002","async":true}`
	req := httptest.NewRequest("POST", "/webhook", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperado status 200, obteve %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"status":"queued"`) {
		t.Fatalf("resposta esperada com status 'queued', obteve: %s", body)
	}
}
