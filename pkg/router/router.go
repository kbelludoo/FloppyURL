package router

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Router implements an OpenAI-compatible gateway with token compression and auto-fallback.
type Router struct {
	pool *Pool
}

// NewRouter creates and initializes a Router using environment variables.
func NewRouter() *Router {
	pool := NewPool()

	// 1. Primary NVIDIA NIM config
	nvKey := os.Getenv("LLM_API_KEY")
	if nvKey == "" {
		nvKey = os.Getenv("NVIDIA_API_KEY")
	}
	nvBase := os.Getenv("LLM_BASE_URL")
	if nvBase == "" {
		nvBase = "https://integrate.api.nvidia.com/v1"
	}

	// 2. Kilo Gateway (390+ models, including 16 free models)
	kiloKey := os.Getenv("KILO_API_KEY")
	kiloBase := "https://api.kilo.ai/api/gateway"


	// 3. Groq Fallback (optional)
	groqKey := os.Getenv("GROQ_API_KEY")
	groqBase := "https://api.groq.com/openai/v1"

	// 4. OpenRouter Fallback (optional)
	orKey := os.Getenv("OPENROUTER_API_KEY")
	orBase := "https://openrouter.ai/api/v1"

	// Setup primary provider chain
	var defaultChain []Provider

	if nvKey != "" {
		defaultChain = append(defaultChain, Provider{
			Name:       "nvidia-kimi-k3",
			BaseURL:    nvBase,
			APIKey:     nvKey,
			DefaultMod: "moonshotai/kimi-k3",
			Active:     true,
		})
	}

	if kiloKey != "" {
		defaultChain = append(defaultChain, Provider{
			Name:       "kilo-gateway-free",
			BaseURL:    kiloBase,
			APIKey:     kiloKey,
			DefaultMod: "kilo-auto/free",
			Active:     true,
		})
	}

	if groqKey != "" {
		defaultChain = append(defaultChain, Provider{
			Name:       "groq-fallback",
			BaseURL:    groqBase,
			APIKey:     groqKey,
			DefaultMod: "llama-3.3-70b-versatile",
			Active:     true,
		})
	}

	if orKey != "" {
		defaultChain = append(defaultChain, Provider{
			Name:       "openrouter-fallback",
			BaseURL:    orBase,
			APIKey:     orKey,
			DefaultMod: "meta-llama/llama-3.3-70b-instruct",
			Active:     true,
		})
	}

	pool.RegisterRoute("default", defaultChain)
	pool.RegisterRoute("moonshotai/kimi-k3", defaultChain)
	pool.RegisterRoute("kimi", defaultChain)
	pool.RegisterRoute("kilo", defaultChain)
	pool.RegisterRoute("kilo-auto/free", defaultChain)
	pool.RegisterRoute("stepfun/step-5-preview-free", defaultChain)
	pool.RegisterRoute("nemotron", defaultChain)
	pool.RegisterRoute("deepseek", defaultChain)


	return &Router{pool: pool}
}

// Handler returns the http.Handler for the router subroutes (/v1/chat/completions, /v1/models, /v1/health).
func (rt *Router) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/chat/completions", rt.handleChatCompletions)
	mux.HandleFunc("/v1/models", rt.handleModels)
	mux.HandleFunc("/v1/health", rt.handleHealth)

	return mux
}

func (rt *Router) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"online","engine":"RouterGo (FloppyURL Gateway)","rtk":"enabled"}`))
}

func (rt *Router) handleModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	type ModelItem struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}

	models := []ModelItem{
		{ID: "moonshotai/kimi-k3", Object: "model", Created: time.Now().Unix(), OwnedBy: "nvidia-nim"},
		{ID: "meta/llama-3.3-70b-instruct", Object: "model", Created: time.Now().Unix(), OwnedBy: "nvidia-nim"},
		{ID: "meta/llama-3.1-8b-instruct", Object: "model", Created: time.Now().Unix(), OwnedBy: "nvidia-nim"},
		{ID: "nvidia/llama-3.1-nemotron-70b-instruct", Object: "model", Created: time.Now().Unix(), OwnedBy: "nvidia-nim"},
		{ID: "arena/battle", Object: "model", Created: time.Now().Unix(), OwnedBy: "arena.ai"},
		{ID: "arena/auto", Object: "model", Created: time.Now().Unix(), OwnedBy: "arena.ai"},
		{ID: "default", Object: "model", Created: time.Now().Unix(), OwnedBy: "routergo"},
	}

	// Dynamic live models from arena.ai
	if arenaClient := GetArenaClient(); arenaClient != nil {
		if cat, err := arenaClient.GetCatalog(); err == nil {
			for _, m := range cat {
				models = append(models, ModelItem{
					ID:      "arena/" + m.PublicName,
					Object:  "model",
					Created: time.Now().Unix(),
					OwnedBy: m.Organization,
				})
			}
		}
	}

	resp := map[string]interface{}{
		"object": "list",
		"data":   models,
	}
	json.NewEncoder(w).Encode(resp)
}

func (rt *Router) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	// CORS support
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == http.MethodOptions {
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"Failed to read request body"}`, http.StatusBadRequest)
		return
	}

	var reqMap map[string]interface{}
	if err := json.Unmarshal(rawBody, &reqMap); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	requestedModel, _ := reqMap["model"].(string)
	if requestedModel == "" {
		requestedModel = "default"
	}

	// 1. Apply RTK (Result Token Kompressor) on tool messages
	if msgs, ok := reqMap["messages"].([]interface{}); ok {
		var castMsgs []map[string]interface{}
		for _, m := range msgs {
			if mMap, ok := m.(map[string]interface{}); ok {
				castMsgs = append(castMsgs, mMap)
			}
		}
		castMsgs = CompressMessages(castMsgs)
		reqMap["messages"] = castMsgs
	}

	isStream, _ := reqMap["stream"].(bool)

	// Re-serialize compressed body
	compressedBody, err := json.Marshal(reqMap)
	if err != nil {
		compressedBody = rawBody
	}

	origSize := len(rawBody)
	optSize := len(compressedBody)
	if origSize > optSize {
		log.Printf("[RouterGo RTK] Saved %d bytes (%.1f%% token reduction)", origSize-optSize, float64(origSize-optSize)/float64(origSize)*100.0)
	}

	// 2. If model is an OpenCode native model (e.g. mimo, opencode/mimo-v2.6-flash-free), route via OpenCodeBridge!
	if strings.HasPrefix(requestedModel, "opencode/") || strings.Contains(requestedModel, "mimo") {
		var msgs []map[string]interface{}
		if rawMsgs, ok := reqMap["messages"].([]interface{}); ok {
			for _, m := range rawMsgs {
				if mMap, ok := m.(map[string]interface{}); ok {
					msgs = append(msgs, mMap)
				}
			}
		}
		bridgeModel := requestedModel
		if requestedModel == "mimo" {
			bridgeModel = "mimo-v2.6-flash-free"
		}
		bResp, err := defaultBridge.Complete(r.Context(), bridgeModel, msgs)
		if err == nil {
			for k, vv := range bResp.Header {
				for _, v := range vv {
					w.Header().Add(k, v)
				}
			}
			w.Header().Set("X-Router-Provider", "opencode-bridge")
			w.WriteHeader(http.StatusOK)
			io.Copy(w, bResp.Body)
			bResp.Body.Close()
			return
		}
		log.Printf("[RouterGo] OpenCodeBridge fallback to pool: %v", err)
	}

	// 3. Dispatch through pool with automatic multi-tier fallback
	resp, providerUsed, err := rt.pool.ForwardWithFallback(r.Context(), requestedModel, reqMap, isStream)

	if err != nil {
		log.Printf("[RouterGo Error] Fallback chain failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"message": err.Error(),
				"type":    "routergo_upstream_failure",
			},
		})
		return
	}
	defer resp.Body.Close()

	// 3. Transparently proxy response (streaming or json) back to client
	for k, vv := range resp.Header {
		// Avoid copying hop-by-hop headers
		if strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Router-Provider", providerUsed.Name)
	w.WriteHeader(resp.StatusCode)

	if flusher, ok := w.(http.Flusher); ok && isStream {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				flusher.Flush()
			}
			if err != nil {
				break
			}
		}
	} else {
		io.Copy(w, resp.Body)
	}
}
