package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenCodeBridge executes models through the local opencode serve instance (port 4096)
// converting standard OpenAI format into OpenCode Session and returning standard OpenAI response.
type OpenCodeBridge struct {
	BaseURL    string
	HTTPClient *http.Client
}

var defaultBridge = &OpenCodeBridge{
	BaseURL: "http://127.0.0.1:4096",
	HTTPClient: &http.Client{
		Timeout: 90 * time.Second,
	},
}

// Complete executes a prompt on opencode serve and formats it as an OpenAI ChatCompletion response.
func (b *OpenCodeBridge) Complete(ctx context.Context, modelID string, messages []map[string]interface{}) (*http.Response, error) {
	// 1. Create a session on OpenCode server
	sessReq, err := http.NewRequestWithContext(ctx, "POST", b.BaseURL+"/session", bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("failed to create session request: %w", err)
	}
	sessReq.Header.Set("Content-Type", "application/json")

	sessResp, err := b.HTTPClient.Do(sessReq)
	if err != nil {
		return nil, fmt.Errorf("opencode server unreachable: %w", err)
	}
	defer sessResp.Body.Close()

	if sessResp.StatusCode >= 300 {
		body, _ := io.ReadAll(sessResp.Body)
		return nil, fmt.Errorf("failed to create opencode session: %s", string(body))
	}

	var sessData struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(sessResp.Body).Decode(&sessData); err != nil || sessData.ID == "" {
		return nil, fmt.Errorf("invalid session response from opencode")
	}

	// 2. Extract last user message or combine context
	var userPrompt string
	for i := len(messages) - 1; i >= 0; i-- {
		role, _ := messages[i]["role"].(string)
		content, _ := messages[i]["content"].(string)
		if role == "user" && content != "" {
			userPrompt = content
			break
		}
	}
	if userPrompt == "" {
		userPrompt = "Olá"
	}

	// Clean model ID: e.g. "opencode/mimo-v2.6-flash-free" -> providerID=opencode, modelID=mimo-v2.6-flash-free
	// "codecraft/claude-fable-5" -> providerID=codecraft, modelID=claude-fable-5
	providerID := "opencode"
	cleanModel := modelID
	if parts := strings.SplitN(modelID, "/", 2); len(parts) == 2 {
		providerID = parts[0]
		cleanModel = parts[1]
	} else if strings.Contains(modelID, "fable") || strings.Contains(modelID, "kimi-k3") {
		// Modelo bare sem prefixo: assume CodeCraft (ex: "claude-fable-5")
		providerID = "codecraft"
		cleanModel = modelID
	}

	msgPayload := map[string]interface{}{
		"model": map[string]string{
			"providerID": providerID,
			"modelID":    cleanModel,
		},
		"parts": []map[string]string{
			{
				"type": "text",
				"text": userPrompt,
			},
		},
	}

	payloadBytes, err := json.Marshal(msgPayload)
	if err != nil {
		return nil, err
	}

	msgReq, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/session/%s/message", b.BaseURL, sessData.ID), bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}
	msgReq.Header.Set("Content-Type", "application/json")

	msgResp, err := b.HTTPClient.Do(msgReq)
	if err != nil {
		return nil, fmt.Errorf("opencode message execution failed: %w", err)
	}
	defer msgResp.Body.Close()

	respBytes, err := io.ReadAll(msgResp.Body)
	if err != nil {
		return nil, err
	}

	if msgResp.StatusCode >= 300 {
		return nil, fmt.Errorf("opencode message error (HTTP %d): %s", msgResp.StatusCode, string(respBytes))
	}

	// 3. Extract text response from parts
	var ocResult struct {
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	json.Unmarshal(respBytes, &ocResult)

	var finalText string
	for _, p := range ocResult.Parts {
		if p.Type == "text" && p.Text != "" {
			finalText += p.Text
		}
	}
	if finalText == "" {
		finalText = "(Sem resposta de texto retornada pelo OpenCode)"
	}

	// 4. Return synthesized standard OpenAI ChatCompletion
	openAIResp := map[string]interface{}{
		"id":      "chatcmpl-" + sessData.ID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   modelID,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": finalText,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     10,
			"completion_tokens": len(finalText) / 4,
			"total_tokens":      10 + len(finalText)/4,
		},
	}

	aiJSON, _ := json.Marshal(openAIResp)
	syntheticResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(aiJSON)),
	}
	syntheticResp.Header.Set("Content-Type", "application/json")
	return syntheticResp, nil
}
