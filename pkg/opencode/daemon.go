package opencode

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DaemonClient interacts with the headless OpenCode daemon (opencode serve --port 4096).
type DaemonClient struct {
	BaseURL     string
	AuthHeader  string
	Credentials []string // List of credential IDs to rotate
	rrIndex     uint64
	client      *http.Client
	mu          sync.Mutex
}

// NewDaemonClient creates a daemon client connected to the local opencode serve instance.
func NewDaemonClient() *DaemonClient {
	baseURL := os.Getenv("OPENCODE_DAEMON_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:4096"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	password := os.Getenv("OPENCODE_SERVER_PASSWORD")
	if password == "" {
		password = "opencode_agent_secret_2026"
	}
	auth := base64.StdEncoding.EncodeToString([]byte("opencode:" + password))

	dc := &DaemonClient{
		BaseURL:    baseURL,
		AuthHeader: "Basic " + auth,
		client:     &http.Client{Timeout: 90 * time.Second},
	}

	// Discover available credentials from /api/credential
	if creds, err := dc.fetchCredentials(); err == nil && len(creds) > 0 {
		dc.Credentials = creds
		log.Printf("[opencode-daemon] Conectado a %s com %d conta(s) pronta(s) para rotacao", baseURL, len(creds))
	} else {
		if envCreds := os.Getenv("OPENCODE_CREDENTIALS"); envCreds != "" {
			for _, part := range strings.Split(envCreds, ",") {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					dc.Credentials = append(dc.Credentials, trimmed)
				}
			}
		}
	}

	return dc
}

// IsAvailable checks if the OpenCode daemon is online and responsive.
func (d *DaemonClient) IsAvailable() bool {
	req, err := http.NewRequest(http.MethodGet, d.BaseURL+"/api/info", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", d.AuthHeader)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := d.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (d *DaemonClient) fetchCredentials() ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, d.BaseURL+"/api/credential", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", d.AuthHeader)
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	var ids []string
	for _, item := range payload.Data {
		if item.ID != "" {
			ids = append(ids, item.ID)
		}
	}
	return ids, nil
}

// Complete executes a prompt on the OpenCode daemon, automatically rotating through accounts.
func (d *DaemonClient) Complete(ctx context.Context, prompt string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 1. Rotate active account if multiple credentials exist
	if len(d.Credentials) > 0 {
		idx := atomic.AddUint64(&d.rrIndex, 1) % uint64(len(d.Credentials))
		credID := d.Credentials[idx]
		_ = d.activateCredential(ctx, credID)
	}

	// 2. Create clean temporary session
	sessionID, err := d.createSession(ctx)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	defer d.deleteSession(context.Background(), sessionID)

	// 3. Send prompt
	if err := d.sendPrompt(ctx, sessionID, prompt); err != nil {
		return "", fmt.Errorf("send prompt: %w", err)
	}

	// 4. Wait for agent loop completion
	if err := d.waitSession(ctx, sessionID); err != nil {
		return "", fmt.Errorf("wait session: %w", err)
	}

	// 5. Read projected assistant response
	answer, err := d.readAssistantMessage(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	return answer, nil
}

func (d *DaemonClient) activateCredential(ctx context.Context, credID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/credential/%s/activate", d.BaseURL, credID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", d.AuthHeader)
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (d *DaemonClient) createSession(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.BaseURL+"/api/session", bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", d.AuthHeader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	if res.Data.ID == "" {
		return "", fmt.Errorf("empty session id")
	}
	return res.Data.ID, nil
}

func (d *DaemonClient) sendPrompt(ctx context.Context, sessionID, text string) error {
	payload, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/session/%s/prompt", d.BaseURL, sessionID), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", d.AuthHeader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (d *DaemonClient) waitSession(ctx context.Context, sessionID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/experimental/session/%s/wait", d.BaseURL, sessionID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", d.AuthHeader)

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (d *DaemonClient) readAssistantMessage(ctx context.Context, sessionID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/session/%s/message", d.BaseURL, sessionID), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", d.AuthHeader)

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var payload struct {
		Data []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	for _, msg := range payload.Data {
		if msg.Type == "assistant" {
			for _, part := range msg.Content {
				if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
					return part.Text, nil
				}
			}
		}
	}
	return "", fmt.Errorf("nenhuma resposta de texto encontrada na sessao")
}

func (d *DaemonClient) deleteSession(ctx context.Context, sessionID string) {
	if sessionID == "" {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/api/session/%s", d.BaseURL, sessionID), nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", d.AuthHeader)
	resp, err := d.client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}
