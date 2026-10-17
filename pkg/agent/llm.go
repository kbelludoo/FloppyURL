package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
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

type UpstreamProvider struct {
	Name    string
	BaseURL string
	APIKey  string
	Model   string
}

type LLMClient struct {
	Providers  []UpstreamProvider
	HTTPClient *http.Client
}

func (c *LLMClient) SetModel(model string) {
	if model != "" && len(c.Providers) > 0 {
		c.Providers[0].Model = model
	}
}

func NewLLMClient(baseURL, apiKey, model string) *LLMClient {
	var providers []UpstreamProvider

	// 1. Provedor Primário (NVIDIA NIM ou o configurado em LLM_BASE_URL)
	pKey := apiKey
	if pKey == "" {
		pKey = os.Getenv("LLM_API_KEY")
	}
	pBase := baseURL
	if pBase == "" {
		pBase = os.Getenv("LLM_BASE_URL")
	}
	if pBase == "" {
		pBase = "https://integrate.api.nvidia.com/v1"
	}
	pModel := model
	if pModel == "" {
		pModel = os.Getenv("LLM_MODEL")
	}
	if pModel == "" {
		pModel = "openai/gpt-oss-20b"
	}

	if pKey != "" {
		providers = append(providers, UpstreamProvider{
			Name:    "nvidia-primary",
			BaseURL: pBase,
			APIKey:  pKey,
			Model:   pModel,
		})

		// Backup automático na própria NVIDIA com outro modelo de ponta
		if strings.Contains(pBase, "nvidia.com") {
			providers = append(providers, UpstreamProvider{
				Name:    "nvidia-backup (step-5)",
				BaseURL: pBase,
				APIKey:  pKey,
				Model:   "stepfun-ai/step-5-preview",
			})
		}
	}

	// 2. Fallback Groq (Llama-3.3 ultra-rápido, tier gratuito)
	if groqKey := os.Getenv("GROQ_API_KEY"); groqKey != "" {
		providers = append(providers, UpstreamProvider{
			Name:    "groq-fast",
			BaseURL: "https://api.groq.com/openai/v1",
			APIKey:  groqKey,
			Model:   "llama-3.3-70b-versatile",
		})
	}

	// 3. Fallback OpenRouter (Modelos gratuitos da comunidade)
	if orKey := os.Getenv("OPENROUTER_API_KEY"); orKey != "" {
		providers = append(providers, UpstreamProvider{
			Name:    "openrouter-free",
			BaseURL: "https://openrouter.ai/api/v1",
			APIKey:  orKey,
			Model:   "meta-llama/llama-3.3-70b-instruct:free",
		})
	}

	// 4. Fallback Kilo Gateway (Modelos gratuitos da Kilo)
	if kiloKey := os.Getenv("KILO_API_KEY"); kiloKey != "" {
		providers = append(providers, UpstreamProvider{
			Name:    "kilo-gateway-free",
			BaseURL: "https://api.kilo.ai/api/gateway",
			APIKey:  kiloKey,
			Model:   "kilo-auto/free",
		})
	}

	return &LLMClient{
		Providers: providers,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *LLMClient) Complete(ctx context.Context, messages []Message, tools []ToolDef) (*Message, string, error) {
	if len(c.Providers) == 0 {
		return nil, "", fmt.Errorf("nenhum provedor LLM configurado. Defina LLM_API_KEY, GROQ_API_KEY, OPENROUTER_API_KEY ou KILO_API_KEY")
	}

	var lastErr error
	for i, prov := range c.Providers {
		msg, finishReason, err := c.completeWithProvider(ctx, prov, messages, tools)
		if err == nil {
			if i > 0 {
				log.Printf("[llm-fallback] ✅ Resposta atendida com sucesso pelo fallback: %s (%s)", prov.Name, prov.Model)
			}
			return msg, finishReason, nil
		}

		log.Printf("[llm-fallback] ⚠️ Falha no provedor %s (%s): %v. Acionando próximo da cadeia...", prov.Name, prov.Model, err)
		lastErr = err
	}

	return nil, "", fmt.Errorf("todos os %d provedores falharam. Ultimo erro: %w", len(c.Providers), lastErr)
}

func (c *LLMClient) completeWithProvider(ctx context.Context, prov UpstreamProvider, messages []Message, tools []ToolDef) (*Message, string, error) {
	reqBody := ChatRequest{
		Model:       prov.Model,
		Messages:    messages,
		Tools:       tools,
		Temperature: 0.2,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal chat request: %w", err)
	}

	url := prov.BaseURL
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	url += "chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+prov.APIKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/kbelludoo/FloppyURL")
	httpReq.Header.Set("X-Title", "Agent Go")

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
