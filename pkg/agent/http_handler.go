package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
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
