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

	// 1. Apinex Gateway (Modelos 100% Free: Gemini 3.8 Flash, DeepSeek v4.1, GPT-6 Luna)
	apinexKey := os.Getenv("APINEX_API_KEY")
	apinexBase := "https://api.apinex.bond/v1"

	// 2. TokenHarbor Gateway (Modelos 100% Free: DeepSeek v4.1 Flash Free, DeepSeek v4 Flash Free)
	thKey := os.Getenv("TOKENHARBOR_API_KEY")
	thBase := "https://tokenharbor.ai/v1"

	// 3. Kilo Gateway (kilo-auto/free)
	kiloKey := os.Getenv("KILO_API_KEY")
	kiloBase := "https://api.kilo.ai/api/gateway"

	// 4. Groq Fallback (tier gratuito)
	groqKey := os.Getenv("GROQ_API_KEY")
	groqBase := "https://api.groq.com/openai/v1"

	// 5. OpenRouter Fallback (modelos gratuitos)
	orKey := os.Getenv("OPENROUTER_API_KEY")
	orBase := "https://openrouter.ai/api/v1"

	// Setup primary provider chain com modelos 100% gratuitos
	var defaultChain []Provider

	if apinexKey != "" {
		defaultChain = append(defaultChain, Provider{
			Name:       "apinex-gemini-free",
			BaseURL:    apinexBase,
			APIKey:     apinexKey,
			DefaultMod: "free/gemini-3.8-flash",
			Active:     true,
		})
		defaultChain = append(defaultChain, Provider{
			Name:       "apinex-deepseek-free",
			BaseURL:    apinexBase,
			APIKey:     apinexKey,
			DefaultMod: "free/deepseek-v4.1-flash",
			Active:     true,
		})
		defaultChain = append(defaultChain, Provider{
			Name:       "apinex-luna-free",
			BaseURL:    apinexBase,
			APIKey:     apinexKey,
			DefaultMod: "free/gpt-6-luna",
			Active:     true,
		})
	}

	if thKey != "" {
		defaultChain = append(defaultChain, Provider{
			Name:       "tokenharbor-deepseek-free",
			BaseURL:    thBase,
			APIKey:     thKey,
			DefaultMod: "deepseek-v4.1-flash:free",
			Active:     true,
		})
		defaultChain = append(defaultChain, Provider{
			Name:       "tokenharbor-v4-free",
			BaseURL:    thBase,
			APIKey:     thKey,
			DefaultMod: "deepseek-v4-flash:free",
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
			DefaultMod: "meta-llama/llama-3.3-70b-instruct:free",
			Active:     true,
		})
	}

	pool.RegisterRoute("default", defaultChain)
	pool.RegisterRoute("gemini", defaultChain)
	pool.RegisterRoute("gemini-flash", defaultChain)
	pool.RegisterRoute("free/gemini-3.8-flash", defaultChain)
	pool.RegisterRoute("deepseek", defaultChain)
	pool.RegisterRoute("deepseek-flash", defaultChain)
	pool.RegisterRoute("deepseek-v4.1-flash:free", defaultChain)
	pool.RegisterRoute("free/deepseek-v4.1-flash", defaultChain)
	pool.RegisterRoute("deepseek-v4-flash:free", defaultChain)
	pool.RegisterRoute("luna", defaultChain)
	pool.RegisterRoute("gpt-6-luna", defaultChain)
	pool.RegisterRoute("free/gpt-6-luna", defaultChain)
	pool.RegisterRoute("kilo", defaultChain)
	pool.RegisterRoute("kilo-auto/free", defaultChain)
	pool.RegisterRoute("llama", defaultChain)

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
		{ID: "codecraft/claude-fable-5", Object: "model", Created: time.Now().Unix(), OwnedBy: "codecraft"},
		{ID: "codecraft/claude-fable-5.1", Object: "model", Created: time.Now().Unix(), OwnedBy: "codecraft"},
		{ID: "codecraft/kimi-k3", Object: "model", Created: time.Now().Unix(), OwnedBy: "codecraft"},
		{ID: "claude-fable-5", Object: "model", Created: time.Now().Unix(), OwnedBy: "codecraft"},
		{ID: "kimi-k3", Object: "model", Created: time.Now().Unix(), OwnedBy: "codecraft"},
		{ID: "moonshotai/kimi-k3", Object: "model", Created: time.Now().Unix(), OwnedBy: "nvidia-nim"},
		{ID: "meta/llama-3.3-70b-instruct", Object: "model", Created: time.Now().Unix(), OwnedBy: "nvidia-nim"},
		{ID: "meta/llama-3.1-8b-instruct", Object: "model", Created: time.Now().Unix(), OwnedBy: "nvidia-nim"},
		{ID: "nvidia/llama-3.1-nemotron-70b-instruct", Object: "model", Created: time.Now().Unix(), OwnedBy: "nvidia-nim"},
		{ID: "arena/battle", Object: "model", Created: time.Now().Unix(), OwnedBy: "arena.ai"},
		{ID: "arena/auto", Object: "model", Created: time.Now().Unix(), OwnedBy: "arena.ai"},
		{ID: "deepseek-web", Object: "model", Created: time.Now().Unix(), OwnedBy: "deepseek-web"},
		{ID: "deepseek-web-thinking", Object: "model", Created: time.Now().Unix(), OwnedBy: "deepseek-web"},
		{ID: "deepseek-web-expert", Object: "model", Created: time.Now().Unix(), OwnedBy: "deepseek-web"},
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

	// 2. Free DeepSeek Web backend (no API key, uses the captured browser
	// session). Intercept before the pool dispatch.
	if isDeepSeekWebModel(requestedModel) {
		var msgs []map[string]interface{}
		if rawMsgs, ok := reqMap["messages"].([]interface{}); ok {
			for _, m := range rawMsgs {
				if mMap, ok := m.(map[string]interface{}); ok {
					msgs = append(msgs, mMap)
				}
			}
		}
		rt.handleDeepSeekWeb(w, r, requestedModel, msgs, isStream)
		return
	}

	// 3. If model is an OpenCode native model (e.g. mimo, opencode/mimo-v2.6-flash-free,
	// codecraft/claude-fable-5), route via OpenCodeBridge with fallback to pool!
	if strings.HasPrefix(requestedModel, "opencode/") || strings.HasPrefix(requestedModel, "codecraft/") || strings.Contains(requestedModel, "mimo") || strings.Contains(requestedModel, "fable") {
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

	// 4. Dispatch through pool with automatic multi-tier fallback
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
