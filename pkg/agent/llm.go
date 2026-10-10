package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []ToolDef `json:"tools,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

type ChatChoice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type ChatResponse struct {
	ID      string       `json:"id"`
	Choices []ChatChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type LLMClient struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

func NewLLMClient(baseURL, apiKey, model string) *LLMClient {
	if baseURL == "" {
		baseURL = os.Getenv("LLM_BASE_URL")
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1"
		}
	}
	if apiKey == "" {
		apiKey = os.Getenv("LLM_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("OPENROUTER_API_KEY")
		}
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
	}
	if model == "" {
		model = os.Getenv("LLM_MODEL")
		if model == "" {
			model = "nousresearch/hermes-3-llama-3.1-8b"
		}
	}

	return &LLMClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *LLMClient) Complete(ctx context.Context, messages []Message, tools []ToolDef) (*Message, string, error) {
	if c.APIKey == "" {
		return nil, "", fmt.Errorf("API key is not configured. Set LLM_API_KEY, OPENROUTER_API_KEY, or pass -llm-key")
	}

	reqBody := ChatRequest{
		Model:       c.Model,
		Messages:    messages,
		Tools:       tools,
		Temperature: 0.2,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal chat request: %w", err)
	}

	url := c.BaseURL
	if url[len(url)-1] == '/' {
		url += "chat/completions"
	} else {
		url += "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/Xelckis/floppyURL")
	httpReq.Header.Set("X-Title", "FloppyAgent Go")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, "", fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return nil, "", fmt.Errorf("failed to decode json response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, "", fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, "", fmt.Errorf("API returned empty choices list")
	}

	choice := chatResp.Choices[0]
	return &choice.Message, choice.FinishReason, nil
}
