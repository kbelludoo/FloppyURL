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

	"github.com/Xelckis/floppyURL/pkg/network"
	"github.com/Xelckis/floppyURL/pkg/opencode"
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
	Name     string
	BaseURL  string
	APIKey   string
	Model    string
	Headers  map[string]string
	NoRotate bool
}

type LLMClient struct {
	Providers  []UpstreamProvider
	HTTPClient *http.Client
}

func (c *LLMClient) SetModel(model string) {
	if model == "" || len(c.Providers) == 0 {
		return
	}
	mLower := strings.ToLower(model)

	// Check if any provider explicitly supports or is dedicated to this model family
	for i, p := range c.Providers {
		pLower := strings.ToLower(p.Name)
		isMatch := false

		if (strings.Contains(mLower, "codestral") || strings.Contains(mLower, "mistral")) && strings.Contains(pLower, "mistral") {
			isMatch = true
		} else if strings.Contains(mLower, "deepseek") && (strings.Contains(pLower, "deepseek") || strings.Contains(strings.ToLower(p.Model), "deepseek")) {
			isMatch = true
		} else if strings.Contains(mLower, "gemini") && (strings.Contains(pLower, "gemini") || strings.Contains(strings.ToLower(p.Model), "gemini")) {
			isMatch = true
		} else if strings.Contains(mLower, "luna") && (strings.Contains(pLower, "luna") || strings.Contains(strings.ToLower(p.Model), "luna")) {
			isMatch = true
		} else if strings.Contains(mLower, "llama") && (strings.Contains(pLower, "groq") || strings.Contains(pLower, "openrouter")) {
			isMatch = true
		} else if strings.Contains(mLower, "kilo") && strings.Contains(pLower, "kilo") {
			isMatch = true
		} else if strings.EqualFold(p.Model, model) {
			isMatch = true
		}

		if isMatch {
			c.Providers[i].Model = model
			if i > 0 {
				c.Providers[0], c.Providers[i] = c.Providers[i], c.Providers[0]
			}
			return
		}
	}

	c.Providers[0].Model = model
}

func NewLLMClient(baseURL, apiKey, model string) *LLMClient {
	var providers []UpstreamProvider

	// 0. opencode providers (same models/auth as the opencode CLI, called
	// directly — no `opencode serve` process required).
	if oc, err := opencode.New(); err == nil {
		defaultID := os.Getenv("OPENCODE_MODEL")
		if defaultID == "" {
			defaultID = oc.DefaultModel()
		}

		// One model per provider is enough for the fallback chain: the
		// default model first, then the first model of every other provider.
		seenProvider := make(map[string]bool)
		var ordered []*opencode.Model
		add := func(m *opencode.Model) {
			if m == nil || seenProvider[m.ProviderID] {
				return
			}
			seenProvider[m.ProviderID] = true
			ordered = append(ordered, m)
		}
		if defaultID != "" {
			if m, err := oc.Resolve(defaultID); err == nil {
				add(m)
			}
		}
		for _, m := range oc.Models() {
			add(m)
		}

		for _, m := range ordered {
			providers = append(providers, UpstreamProvider{
				Name:     "opencode-" + m.ID,
				BaseURL:  m.BaseURL,
				APIKey:   m.APIKey,
				Model:    m.ModelID,
				Headers:  m.Headers,
				NoRotate: true,
			})
		}
		if len(ordered) > 0 {
			log.Printf("[llm] opencode: %d modelo(s) carregado(s) de %s", len(ordered), strings.Join(oc.ConfigFiles(), ", "))
		}
	}

	// 1. Apinex Gateway (Modelos 100% Free: Gemini 3.8 Flash, DeepSeek v4.1 Flash, GPT-6 Luna)
	if apinexKey := os.Getenv("APINEX_API_KEY"); apinexKey != "" {
		primaryModel := os.Getenv("APINEX_MODEL")
		if primaryModel == "" {
			primaryModel = "free/gemini-3.8-flash"
		}
		providers = append(providers, UpstreamProvider{
			Name:    "apinex-primary",
			BaseURL: "https://api.apinex.bond/v1",
			APIKey:  apinexKey,
			Model:   primaryModel,
		})
		if primaryModel != "free/deepseek-v4.1-flash" {
			providers = append(providers, UpstreamProvider{
				Name:    "apinex-deepseek-free",
				BaseURL: "https://api.apinex.bond/v1",
				APIKey:  apinexKey,
				Model:   "free/deepseek-v4.1-flash",
			})
		}
		if primaryModel != "free/gpt-6-luna" {
			providers = append(providers, UpstreamProvider{
				Name:    "apinex-luna-free",
				BaseURL: "https://api.apinex.bond/v1",
				APIKey:  apinexKey,
				Model:   "free/gpt-6-luna",
			})
		}
	}

	// 2. TokenHarbor Gateway (Modelos 100% Free: DeepSeek v4.1 Flash Free, DeepSeek v4 Flash Free)
	if thKey := os.Getenv("TOKENHARBOR_API_KEY"); thKey != "" {
		thModel := os.Getenv("TOKENHARBOR_MODEL")
		if thModel == "" {
			thModel = "deepseek-v4.1-flash:free"
		}
		providers = append(providers, UpstreamProvider{
			Name:    "tokenharbor-primary",
			BaseURL: "https://tokenharbor.ai/v1",
			APIKey:  thKey,
			Model:   thModel,
		})
		if thModel != "deepseek-v4-flash:free" {
			providers = append(providers, UpstreamProvider{
				Name:    "tokenharbor-v4-free",
				BaseURL: "https://tokenharbor.ai/v1",
				APIKey:  thKey,
				Model:   "deepseek-v4-flash:free",
			})
		}
	}

	// 2.5 Mistral AI (Modelos 100% Free / Codestral)
	if mistralKey := os.Getenv("MISTRAL_API_KEY"); mistralKey != "" {
		mModel := os.Getenv("MISTRAL_MODEL")
		if mModel == "" {
			mModel = "codestral-latest"
		}
		providers = append(providers, UpstreamProvider{
			Name:    "mistral-codestral",
			BaseURL: "https://api.mistral.ai/v1",
			APIKey:  mistralKey,
			Model:   mModel,
		})
	}

	// 3. Fallback Kilo Gateway (Modelo 100% Free: kilo-auto/free)
	if kiloKey := os.Getenv("KILO_API_KEY"); kiloKey != "" {
		providers = append(providers, UpstreamProvider{
			Name:    "kilo-gateway-free",
			BaseURL: "https://api.kilo.ai/api/gateway",
			APIKey:  kiloKey,
			Model:   "kilo-auto/free",
		})
	}

	// 4. Fallback Groq (Llama-3.3 ultra-rápido, tier gratuito)
	if groqKey := os.Getenv("GROQ_API_KEY"); groqKey != "" {
		providers = append(providers, UpstreamProvider{
			Name:    "groq-fast",
			BaseURL: "https://api.groq.com/openai/v1",
			APIKey:  groqKey,
			Model:   "llama-3.3-70b-versatile",
		})
	}

	// 5. Fallback OpenRouter (Modelos gratuitos da comunidade)
	if orKey := os.Getenv("OPENROUTER_API_KEY"); orKey != "" {
		providers = append(providers, UpstreamProvider{
			Name:    "openrouter-free",
			BaseURL: "https://openrouter.ai/api/v1",
			APIKey:  orKey,
			Model:   "meta-llama/llama-3.3-70b-instruct:free",
		})
	}

	// 6. Provedor customizado explícito (se informado e não for NVIDIA NIM expirado)
	if apiKey != "" && !strings.Contains(baseURL, "integrate.api.nvidia.com") {
		providers = append([]UpstreamProvider{{
			Name:    "custom-override",
			BaseURL: baseURL,
			APIKey:  apiKey,
			Model:   model,
		}}, providers...)
	}

	if model != "" && len(providers) > 0 {
		providers[0].Model = model
	}

	return &LLMClient{
		Providers:  providers,
		HTTPClient: network.NewRotatingHTTPClient(120 * time.Second),
	}
}

func (c *LLMClient) Complete(ctx context.Context, messages []Message, tools []ToolDef) (*Message, string, error) {
	if len(c.Providers) == 0 {
		return nil, "", fmt.Errorf("nenhum provedor LLM configurado. Defina MISTRAL_API_KEY, APINEX_API_KEY, TOKENHARBOR_API_KEY, KILO_API_KEY, GROQ_API_KEY ou OPENROUTER_API_KEY")
	}

	var lastErr error
	for i, prov := range c.Providers {
		msg, finishReason, err := c.completeWithProvider(ctx, prov, messages, tools)
		if err == nil {
			if i > 0 {
				log.Printf("[llm-fallback] ✅ Resposta atendida com sucesso pelo fallback gratuito: %s (%s)", prov.Name, prov.Model)
			}
			return msg, finishReason, nil
		}

		log.Printf("[llm-fallback] ⚠️ Falha no provedor %s (%s): %v. Acionando próximo da cadeia...", prov.Name, prov.Model, err)
		lastErr = err
	}

	return nil, "", fmt.Errorf("todos os %d provedores gratuitos falharam. Ultimo erro: %w", len(c.Providers), lastErr)
}

func (c *LLMClient) completeWithProvider(ctx context.Context, prov UpstreamProvider, messages []Message, tools []ToolDef) (*Message, string, error) {
	// Provedores que não suportam tools (como Kilo gateway) recebem tools=nil
	activeTools := tools
	if prov.Model == "kilo-auto/free" || strings.HasPrefix(prov.Model, "kilo-") {
		activeTools = nil
	}

	reqBody := ChatRequest{
		Model:       prov.Model,
		Messages:    messages,
		Tools:       activeTools,
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

	// Timeout de até 35s por provedor individual para chavear rápido no fallback
	provCtx, provCancel := context.WithTimeout(ctx, 35*time.Second)
	defer provCancel()

	httpReq, err := http.NewRequestWithContext(provCtx, "POST", url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create http request: %w", err)
	}

	for k, v := range prov.Headers {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if prov.APIKey != "" && httpReq.Header.Get("Authorization") == "" && httpReq.Header.Get("X-Api-Key") == "" {
		httpReq.Header.Set("Authorization", "Bearer "+prov.APIKey)
	}
	httpReq.Header.Set("HTTP-Referer", "https://github.com/kbelludoo/FloppyURL")
	httpReq.Header.Set("X-Title", "Agent Go")

	httpClient := c.HTTPClient
	if prov.NoRotate {
		httpClient = &http.Client{}
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, "", fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Se falhou com 400 e tinha tools ativas, tenta uma vez sem tools antes de descartar
		if resp.StatusCode == 400 && len(activeTools) > 0 {
			reqBodyNoTools := reqBody
			reqBodyNoTools.Tools = nil
			if retryBytes, err := json.Marshal(reqBodyNoTools); err == nil {
				retryReq, err := http.NewRequestWithContext(provCtx, "POST", url, bytes.NewReader(retryBytes))
				if err == nil {
					for k, v := range prov.Headers {
						retryReq.Header.Set(k, v)
					}
					retryReq.Header.Set("Content-Type", "application/json")
					if prov.APIKey != "" && retryReq.Header.Get("Authorization") == "" && retryReq.Header.Get("X-Api-Key") == "" {
						retryReq.Header.Set("Authorization", "Bearer "+prov.APIKey)
					}
					if retryResp, err := httpClient.Do(retryReq); err == nil {
						defer retryResp.Body.Close()
						if retryResp.StatusCode >= 200 && retryResp.StatusCode < 300 {
							if rBody, err := io.ReadAll(retryResp.Body); err == nil {
								var chatResp ChatResponse
								if err := json.Unmarshal(rBody, &chatResp); err == nil && len(chatResp.Choices) > 0 {
									choice := chatResp.Choices[0]
									return &choice.Message, choice.FinishReason, nil
								}
							}
						}
					}
				}
			}
		}
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
