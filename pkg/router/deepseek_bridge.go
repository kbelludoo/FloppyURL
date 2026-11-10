package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kbelludoo/FloppyURL/pkg/deepseek"
)

// deepseek-web backend: an OpenAI-compatible front for chat.deepseek.com that
// needs no API key, only a captured browser session (see `agent-go -deepseek-login`).

var (
	dsClient *deepseek.Client
	dsMu     sync.Mutex
)

func getDeepSeekClient(ctx context.Context) (*deepseek.Client, error) {
	dsMu.Lock()
	defer dsMu.Unlock()
	if dsClient != nil {
		return dsClient, nil
	}
	c, err := deepseek.NewClient(ctx, "", false)
	if err != nil {
		return nil, err
	}
	dsClient = c
	return c, nil
}

// isDeepSeekWebModel reports whether the requested model targets the free
// DeepSeek Web backend.
func isDeepSeekWebModel(model string) bool {
	return strings.HasPrefix(model, "deepseek-web")
}

func deepSeekOptions(model string) deepseek.StreamOptions {
	opts := deepseek.StreamOptions{Model: deepseek.ModelDefault}
	if strings.Contains(model, "expert") {
		opts.Model = deepseek.ModelExpert
	}
	if strings.Contains(model, "thinking") {
		opts.Thinking = true
	}
	return opts
}

// buildDeepSeekPrompt flattens an OpenAI message list into a single transcript,
// since the DeepSeek web endpoint takes a plain prompt string.
func buildDeepSeekPrompt(messages []map[string]interface{}) string {
	var b strings.Builder
	for _, m := range messages {
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		if content == "" {
			continue
		}
		switch role {
		case "system":
			b.WriteString("System: ")
		case "assistant":
			b.WriteString("Assistant: ")
		case "user":
			b.WriteString("User: ")
		default:
			b.WriteString(role + ": ")
		}
		b.WriteString(content)
		b.WriteString("\n")
	}
	b.WriteString("Assistant:")
	return b.String()
}

// handleDeepSeekWeb executes a completion through the free DeepSeek Web
// backend and writes an OpenAI-compatible response.
func (rt *Router) handleDeepSeekWeb(w http.ResponseWriter, r *http.Request, model string, messages []map[string]interface{}, isStream bool) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	client, err := getDeepSeekClient(r.Context())
	if err != nil {
		writeOpenAIError(w, deepSeekLoginHint(err))
		return
	}

	prompt := buildDeepSeekPrompt(messages)
	opts := deepSeekOptions(model)

	if isStream {
		rt.streamDeepSeek(w, r, client, model, prompt, opts)
		return
	}

	reply, err := client.Chat(r.Context(), prompt, opts)
	if err != nil {
		writeOpenAIError(w, deepSeekLoginHint(err))
		return
	}

	resp := map[string]interface{}{
		"id":      "chatcmpl-deepseek-web",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       map[string]string{"role": "assistant", "content": reply.Text},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     len(prompt) / 4,
			"completion_tokens": len(reply.Text) / 4,
			"total_tokens":      (len(prompt) + len(reply.Text)) / 4,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (rt *Router) streamDeepSeek(w http.ResponseWriter, r *http.Request, client *deepseek.Client, model, prompt string, opts deepseek.StreamOptions) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenAIError(w, "streaming not supported by this server")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	id := fmt.Sprintf("chatcmpl-deepseek-%d", time.Now().UnixNano())
	send := func(delta map[string]interface{}, finish interface{}) {
		chunk := map[string]interface{}{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]interface{}{
				{"index": 0, "delta": delta, "finish_reason": finish},
			},
		}
		payload, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
	}

	send(map[string]interface{}{"role": "assistant"}, nil)

	_, err := client.Stream(r.Context(), prompt, opts, func(delta string) error {
		send(map[string]interface{}{"content": delta}, nil)
		return nil
	})
	if err != nil {
		send(map[string]interface{}{"content": "\n[erro DeepSeek Web] " + deepSeekLoginHint(err)}, "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}
	send(map[string]interface{}{}, "stop")
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func writeOpenAIError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"message": msg,
			"type":    "deepseek_web_error",
		},
	})
}

func deepSeekLoginHint(err error) string {
	if err == nil {
		return ""
	}
	if deepseek.IsLoginRequired(err) {
		return "Nenhuma sessão DeepSeek encontrada. Rode: agent-go -deepseek-login (com o Chrome aberto em modo --remote-debugging-port=9222 e logado em chat.deepseek.com)."
	}
	return err.Error()
}
