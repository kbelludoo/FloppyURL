package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	opencodesdk "github.com/ethpandaops/opencode-agent-sdk-go"
)

func (rt *Router) handleACP(w http.ResponseWriter, r *http.Request, model string, request map[string]any, stream bool) {
	// ACP executes tools inside OpenCode; it cannot return OpenAI tool_calls
	// for an external caller to execute. Host tools are registered on /agent.
	for _, field := range []string{"tools", "functions"} {
		if value, ok := request[field].([]any); ok && len(value) > 0 {
			writeRouterError(w, http.StatusBadRequest, fmt.Errorf("OpenCode ACP does not support OpenAI %s; use /agent for registered host tools", field))
			return
		}
	}
	raw, err := json.Marshal(request["messages"])
	if err != nil {
		writeRouterError(w, http.StatusBadRequest, err)
		return
	}
	var messages []map[string]any
	if err := json.Unmarshal(raw, &messages); err != nil || len(messages) == 0 {
		writeRouterError(w, http.StatusBadRequest, fmt.Errorf("ACP requires a non-empty messages array"))
		return
	}
	for _, message := range messages {
		role, _ := message["role"].(string)
		if role == "" {
			writeRouterError(w, http.StatusBadRequest, fmt.Errorf("ACP messages require a role"))
			return
		}
		if _, ok := message["content"].(string); !ok {
			writeRouterError(w, http.StatusBadRequest, fmt.Errorf("ACP gateway currently accepts text content only"))
			return
		}
	}
	result, err := rt.acp.Complete(r.Context(), model, messages)
	if err != nil {
		// Return the real CLI error. Do not silently switch models or accounts.
		writeRouterError(w, http.StatusBadGateway, err)
		return
	}
	writeACPResponse(w, rt.acp.ResolveModel(model), result, stream)
}

func writeACPResponse(w http.ResponseWriter, model string, result *opencodesdk.QueryResult, stream bool) {
	id := "chatcmpl-" + result.SessionID
	created := time.Now().Unix()
	finish := "stop"
	switch string(result.StopReason) {
	case "max_tokens":
		finish = "length"
	case "refusal":
		finish = "content_filter"
	}
	w.Header().Set("X-Router-Provider", "opencode-acp/"+model)
	if stream {
		// Query aggregates ACP updates. Emit a complete SSE turn after it
		// finishes; this is compatibility streaming, not live token delivery.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		for _, choice := range []map[string]any{
			{"index": 0, "delta": map[string]string{"role": "assistant", "content": result.AssistantText}, "finish_reason": nil},
			{"index": 0, "delta": map[string]string{}, "finish_reason": finish},
		} {
			chunk, _ := json.Marshal(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{choice}})
			if _, err := fmt.Fprintf(w, "data: %s\n\n", chunk); err != nil {
				return
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	response := map[string]any{
		"id": id, "object": "chat.completion", "created": created, "model": model,
		"choices": []any{map[string]any{
			"index": 0, "message": map[string]string{"role": "assistant", "content": result.AssistantText}, "finish_reason": finish,
		}},
	}
	if result.Usage != nil {
		response["usage"] = map[string]int{
			"prompt_tokens": result.Usage.InputTokens, "completion_tokens": result.Usage.OutputTokens, "total_tokens": result.Usage.TotalTokens,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
