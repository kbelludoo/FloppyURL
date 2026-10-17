package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kbelludoo/FloppyURL/pkg/opencode"
	acp "github.com/coder/acp-go-sdk"
	opencodesdk "github.com/ethpandaops/opencode-agent-sdk-go"
)

func TestACPJSONAndSSEResponses(t *testing.T) {
	result := &opencodesdk.QueryResult{SessionID: "test", AssistantText: "Olá\nAna", StopReason: acp.StopReasonEndTurn,
		Usage: &acp.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 6}}
	for _, stream := range []bool{false, true} {
		w := httptest.NewRecorder()
		writeACPResponse(w, "opencode/test-free", result, stream)
		if w.Code != http.StatusOK || w.Header().Get("X-Router-Provider") != "opencode-acp/opencode/test-free" {
			t.Fatalf("unexpected status or provider: %d %v", w.Code, w.Header())
		}
		if stream {
			if !strings.HasSuffix(w.Body.String(), "data: [DONE]\n\n") {
				t.Fatal("SSE terminator missing")
			}
			for _, frame := range strings.Split(strings.TrimSpace(w.Body.String()), "\n\n") {
				data := strings.TrimPrefix(frame, "data: ")
				if data != "[DONE]" && !json.Valid([]byte(data)) {
					t.Fatalf("invalid SSE JSON: %s", data)
				}
			}
		} else {
			var response struct {
				Choices []struct{ Message struct{ Content string } }
				Usage   map[string]int
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Choices[0].Message.Content != result.AssistantText || response.Usage["total_tokens"] != 6 {
				t.Fatal("text or usage lost")
			}
		}
	}
}

func TestACPRoutingRejectsExternalToolsBeforeStartingCLI(t *testing.T) {
	rt := &Router{acp: &opencode.ACPClient{CLIPath: "missing-cli", Model: "opencode/test-free", Timeout: time.Second}}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"opencode/test-free","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function"}]}`))
	rt.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "does not support OpenAI tools") {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
}

func TestACPFailureDoesNotFallBackToDirectHTTP(t *testing.T) {
	rt := &Router{acp: &opencode.ACPClient{CLIPath: "missing-cli", Model: "opencode/test-free", Timeout: time.Second}}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hello"}]}`))
	rt.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "OpenCode ACP") {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
}
