package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Xelckis/floppyURL/pkg/watchdog"
)

// sessions holds per-session Agents (keyed by X-Session-ID header).
var (
	sessions   = map[string]*Agent{}
	sessionsMu sync.Mutex
)

type agentRequest struct {
	Message string `json:"message"`
	Model   string `json:"model,omitempty"`
}


type agentResponse struct {
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Handler returns an http.HandlerFunc for the /agent endpoint.
func Handler() http.HandlerFunc {
	agentKey := os.Getenv("AGENT_KEY")

	return func(w http.ResponseWriter, r *http.Request) {
		// CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-ID")
		if r.Method == http.MethodOptions {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		// Optional bearer-token auth
		if agentKey != "" {
			auth := r.Header.Get("Authorization")
			token := strings.TrimPrefix(auth, "Bearer ")
			if token != agentKey {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
			return
		}
		var req agentRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil || strings.TrimSpace(req.Message) == "" {
			log.Printf("[agent] invalid request payload (%d bytes): %q err=%v", len(bodyBytes), string(bodyBytes), err)
			http.Error(w, fmt.Sprintf(`{"error":"'message' field required","details":%q}`, err), http.StatusBadRequest)
			return
		}

		// Session-aware agent lookup
		sessionID := r.Header.Get("X-Session-ID")
		var ag *Agent
		if sessionID != "" {
			sessionsMu.Lock()
			ag = sessions[sessionID]
			if ag == nil {
				ag = New()
				sessions[sessionID] = ag
			}
			sessionsMu.Unlock()
		} else {
			ag = NewStateless()
		}

		if req.Model != "" {
			ag.SetModel(req.Model)
		}

		log.Printf("[agent] session=%q model=%q msg=%q", sessionID, req.Model, req.Message)

		response, err := ag.Run(r.Context(), req.Message)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			log.Printf("[agent] error: %v", err)
			json.NewEncoder(w).Encode(agentResponse{Error: err.Error()})
			return
		}

		json.NewEncoder(w).Encode(agentResponse{Response: response})
	}
}

// WebhookHandler returns an http.HandlerFunc for the event-driven /webhook endpoint.
func WebhookHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var payload struct {
			Event  string `json:"event"`
			Source string `json:"source"`
			Prompt string `json:"prompt"`
			Async  bool   `json:"async"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
			return
		}

		prompt := strings.TrimSpace(payload.Prompt)
		if prompt == "" {
			prompt = fmt.Sprintf("Evento externo recebido [Fonte: %s, Evento: %s]. Analise a situação dos bots ou do sistema e tome ações preventivas ou corretivas se necessário.", payload.Source, payload.Event)
		}

		if payload.Async {
			go func(p, ev, src string) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				res, err := NewStateless().Run(ctx, p)
				if err != nil {
					log.Printf("[webhook] erro ao processar evento assíncrono (%s/%s): %v", src, ev, err)
					watchdog.SendTelegram(fmt.Sprintf("⚠️ *FALHA NO WEBHOOK (%s/%s)*\n\nErro: %v", src, ev, err))
					return
				}
				watchdog.SendTelegram(fmt.Sprintf("🔔 *EVENTO WEBHOOK PROCESSADO (%s/%s)*\n\n%s", src, ev, res))
			}(prompt, payload.Event, payload.Source)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "queued",
				"message": "Evento recebido e despachado em segundo plano para o Agent Go.",
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		res, err := NewStateless().Run(ctx, prompt)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "response": res})
	}
}
