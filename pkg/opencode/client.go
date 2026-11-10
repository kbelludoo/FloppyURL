package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// NewHTTP builds a client from explicit config/auth files. Empty paths fall
// back to the default opencode discovery locations.
func NewHTTP(configFile, authFile string, timeout time.Duration) (*Client, error) {
	cfg, cfgFiles, err := loadConfigFile(configFile)
	if err != nil {
		if os.Getenv("OPENCODE_API_KEY") == "" {
			return nil, err
		}
		cfg = &Config{Provider: map[string]ProviderConfig{}}
	}
	auth, authFiles, err := loadAuthFile(authFile)
	if err != nil {
		return nil, err
	}

	if timeout <= 0 {
		timeout = 10 * time.Minute
	}

	c := buildClient(cfg, cfgFiles, authFiles, auth)

	// Optional opencode Zen gateway (OPENCODE_API_KEY from
	// https://opencode.ai/console). Free-tier access is decided by opencode's
	// server; a key is required here.
	zen := false
	if key := strings.TrimSpace(os.Getenv("OPENCODE_API_KEY")); key != "" {
		baseURL := strings.TrimRight(os.Getenv("OPENCODE_ZEN_BASE_URL"), "/")
		if baseURL == "" {
			baseURL = "https://opencode.ai/zen/v1"
		}
		zen = addZenModels(c, baseURL, key, timeout)
	}

	if len(c.models) == 0 {
		return nil, fmt.Errorf("no usable opencode models found (config: %s)", strings.Join(cfgFiles, ", "))
	}
	sortModels(c)

	// OPENCODE_MODEL can pin any configured model as the default.
	if pinned := strings.TrimSpace(os.Getenv("OPENCODE_MODEL")); pinned != "" {
		if m, err := c.Resolve(pinned); err == nil {
			c.defaultModel = m.ID
		}
	}
	if c.defaultModel == "" && zen {
		c.defaultModel = "opencode/deepseek-v4.1-flash-free"
	}

	// Use a plain client: provider auth is tied to the account, not to a
	// rotated egress IP, and binding failures must not break chat.
	c.httpClient = &http.Client{Timeout: timeout}
	return c, nil
}

func addZenModels(c *Client, baseURL, key string, timeout time.Duration) bool {
	ids := fetchZenModelIDs(baseURL, key, timeout)
	if len(ids) == 0 {
		ids = []string{
			"deepseek-v4.1-flash-free",
			"glm-5.3-flash-free",
			"mimo-v2.6-flash-free",
			"big-pickle",
			"beam-free",
			"nemotron-3.5-super-vl-free",
			"nemotron-3.5-lightning-free",
			"jev-1.13-free",
			"longcat-2.5-preview-free",
			"space-bunny-free",
		}
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		full := "opencode/" + id
		c.models[full] = &Model{
			ID:         full,
			ProviderID: "opencode",
			ModelID:    id,
			Name:       id,
			BaseURL:    strings.TrimRight(baseURL, "/"),
			APIKey:     key,
			Protocol:   "openai",
		}
	}
	return len(ids) > 0
}

func fetchZenModelIDs(baseURL, key string, timeout time.Duration) []string {
	if timeout <= 0 || timeout > 8*time.Second {
		timeout = 8 * time.Second
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/models", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "AgentGo-OpenCode/1.0")
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil
	}
	ids := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
		ids = append(ids, m.ID)
	}
	return ids
}

func sortModels(c *Client) {
	c.ordered = make([]*Model, 0, len(c.models))
	for _, m := range c.models {
		c.ordered = append(c.ordered, m)
	}
	sort.Slice(c.ordered, func(i, j int) bool { return c.ordered[i].ID < c.ordered[j].ID })
}

func loadConfigFile(path string) (*Config, []string, error) {
	if path == "" {
		return loadConfig()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(stripJSONC(data), &cfg); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &cfg, []string{path}, nil
}

func loadAuthFile(path string) (map[string]AuthEntry, []string, error) {
	if path == "" {
		return loadAuth()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	var entries map[string]AuthEntry
	if err := json.Unmarshal(stripJSONC(data), &entries); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return entries, []string{path}, nil
}

// Proxy forwards an OpenAI-shaped chat request to the provider that backs
// modelID and returns the raw upstream response. The caller owns resp.Body.
func (c *Client) Proxy(ctx context.Context, modelID string, reqMap map[string]interface{}) (*http.Response, *Model, error) {
	if c == nil {
		return nil, nil, fmt.Errorf("opencode client is not configured")
	}
	model, err := c.Resolve(modelID)
	if err != nil {
		return nil, nil, err
	}
	if model.Protocol != "openai" {
		return nil, model, fmt.Errorf("model %s uses the %q protocol; only openai-compatible providers are supported by the direct bridge", model.ID, model.Protocol)
	}
	reqMap["model"] = model.ModelID

	payload, err := json.Marshal(reqMap)
	if err != nil {
		return nil, model, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := strings.TrimRight(model.BaseURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, model, fmt.Errorf("build request: %w", err)
	}
	applyHeaders(req, model)

	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, model, fmt.Errorf("call %s: %w", model.ID, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, model, fmt.Errorf("provider %s returned HTTP %d: %s", model.ID, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return resp, model, nil
}

// Headers returns a copy of the headers that would be sent for a model.
func (c *Client) Headers(modelID string) map[string]string {
	model, err := c.Resolve(modelID)
	if err != nil {
		return nil
	}
	return mergeHeaders(model.Headers)
}

func applyHeaders(req *http.Request, model *Model) {
	for k, v := range model.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AgentGo-OpenCode/1.0")
	if model.APIKey != "" && req.Header.Get("Authorization") == "" && req.Header.Get("X-Api-Key") == "" {
		req.Header.Set("Authorization", "Bearer "+model.APIKey)
	}
}

func buildClient(cfg *Config, configFiles []string, authFiles []string, auth map[string]AuthEntry) *Client {
	c := &Client{
		models:       make(map[string]*Model),
		defaultModel: cfg.Model,
		configFiles:  configFiles,
		authFiles:    authFiles,
	}
	for providerID, pc := range cfg.Provider {
		baseURL := strings.TrimRight(strings.TrimSpace(pc.Options.BaseURL), "/")
		if baseURL == "" {
			continue
		}
		apiKey := pc.Options.APIKey
		if apiKey == "" {
			if a, ok := auth[providerID]; ok && a.Type == "api" {
				apiKey = a.Key
			}
		}
		protocol := detectProtocol(pc.Npm)
		for modelID, mc := range pc.Models {
			id := providerID + "/" + modelID
			name := mc.Name
			if name == "" {
				name = modelID
			}
			c.models[id] = &Model{
				ID:         id,
				ProviderID: providerID,
				ModelID:    modelID,
				Name:       name,
				BaseURL:    baseURL,
				APIKey:     apiKey,
				Headers:    mergeHeaders(pc.Options.Headers, mc.Headers),
				Protocol:   protocol,
			}
		}
	}
	sortModels(c)
	return c
}
