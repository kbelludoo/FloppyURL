package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Xelckis/floppyURL/pkg/network"
	"golang.org/x/net/proxy"
)

// Provider represents a single LLM upstream backend (e.g., NVIDIA NIM, Groq, OpenRouter).
type Provider struct {
	Name       string
	BaseURL    string
	APIKey     string
	DefaultMod string
	Active     bool
	LocalIP    net.IP
	// Circuit breaker metrics
	failCount  int64
	lastFailed time.Time
}

// ModelRoute defines a requested model alias and its prioritized fallback chain of providers.
type ModelRoute struct {
	Alias     string     // e.g. "default", "fast", "smart", "kimi"
	Providers []Provider // In order of fallback priority
}

// Pool coordinates multi-provider routing, load balancing, and auto-fallback.
type Pool struct {
	mu        sync.RWMutex
	routes    map[string][]Provider
	client    *http.Client
	sp2Client *http.Client
	rrCounter uint64
}

// NewPool initializes a routing pool with direct client and optional SP2 tunnel client.
func NewPool() *Pool {
	directClient := network.NewRotatingHTTPClient(120 * time.Second)

	var sp2Client *http.Client
	// Setup SOCKS5 dialer for SP2 if tunnel is available on 127.0.0.1:1080
	if dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:1080", nil, proxy.Direct); err == nil {
		sp2Client = &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.Dial(network, addr)
				},
			},
			Timeout: 120 * time.Second,
		}
	}

	return &Pool{
		routes:    make(map[string][]Provider),
		client:    directClient,
		sp2Client: sp2Client,
	}
}

// RegisterRoute adds or updates a route with fallbacks.
func (p *Pool) RegisterRoute(alias string, providers []Provider) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.routes[alias] = providers
}

// ForwardWithFallback sends the request payload to the providers in sequence until one succeeds.
func (p *Pool) ForwardWithFallback(ctx context.Context, modelAlias string, reqMap map[string]interface{}, isStream bool) (*http.Response, *Provider, error) {
	p.mu.RLock()
	providers, ok := p.routes[modelAlias]
	if !ok {
		// Fallback to "default" if requested model alias isn't explicitly configured
		providers = p.routes["default"]
	}
	p.mu.RUnlock()

	if len(providers) == 0 {
		return nil, nil, fmt.Errorf("no upstream providers configured for model alias %q", modelAlias)
	}

	var lastErr error
	for i := range providers {
		prov := &providers[i]

		// Map model alias to upstream provider model ID
		targetModel := modelAlias
		switch modelAlias {
		case "default", "":
			targetModel = "free/gemini-3.8-flash"
		case "gemini", "gemini-flash", "gemini-3.8-flash":
			targetModel = "free/gemini-3.8-flash"
		case "deepseek", "deepseek-flash", "deepseek-v4.1":
			targetModel = "deepseek-v4.1-flash:free"
		case "deepseek-v4":
			targetModel = "deepseek-v4-flash:free"
		case "luna", "gpt-6-luna":
			targetModel = "free/gpt-6-luna"
		case "kilo", "kilo-free", "kilo-auto/free":
			targetModel = "kilo-auto/free"
		case "mistral", "codestral", "codestral-latest":
			targetModel = "codestral-latest"
		case "claude", "claude-sonnet", "claude-sonnet-5":
			targetModel = "claude-sonnet-5"
		case "claude-opus", "claude-opus-5.5":
			targetModel = "claude-opus-5.5"
		case "llama", "llama-3.3":
			targetModel = "llama-3.3-70b-versatile"
		}

		// If this is a fallback provider (i > 0) and the primary model failed, use provider's DefaultMod
		if i > 0 && prov.DefaultMod != "" {
			targetModel = prov.DefaultMod
		}
		reqMap["model"] = targetModel

		payloadBytes, err := json.Marshal(reqMap)

		if err != nil {
			lastErr = err
			continue
		}

		url := prov.BaseURL
		if url[len(url)-1] == '/' {
			url += "chat/completions"
		} else {
			url += "/chat/completions"
		}

		// Per-provider timeout context (max 30s per attempt before trying next fallback)
		provCtx, provCancel := context.WithTimeout(ctx, 30*time.Second)

		httpReq, err := http.NewRequestWithContext(provCtx, "POST", url, io.NopCloser(bytesNewReader(payloadBytes)))
		if err != nil {
			provCancel()
			lastErr = err
			continue
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+prov.APIKey)
		httpReq.Header.Set("User-Agent", "RouterGo/1.0 (FloppyURL-Omni)")

		// Automatic IP Egress rotation: Bind to dedicated LocalIP if set, else alternate SP1/SP2
		clientToUse := p.client
		if prov.LocalIP != nil {
			clientToUse = &http.Client{
				Transport: &http.Transport{
					DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						dialer := &net.Dialer{
							Timeout:   15 * time.Second,
							KeepAlive: 30 * time.Second,
							LocalAddr: &net.TCPAddr{IP: prov.LocalIP},
						}
						return dialer.DialContext(ctx, network, addr)
					},
					DisableKeepAlives: true,
				},
				Timeout: 30 * time.Second,
			}
		} else if p.sp2Client != nil && atomic.AddUint64(&p.rrCounter, 1)%2 == 1 {
			clientToUse = p.sp2Client
		}

		resp, err := clientToUse.Do(httpReq)
		if err != nil {
			provCancel()
			atomic.AddInt64(&prov.failCount, 1)
			lastErr = fmt.Errorf("provider %s failed: %w", prov.Name, err)
			continue
		}

		// Check for error status codes (4xx client error / expired model, 5xx server error, 429 rate limit) -> Trigger Fallback!
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			atomic.AddInt64(&prov.failCount, 1)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			provCancel()
			lastErr = fmt.Errorf("provider %s returned HTTP %d: %s", prov.Name, resp.StatusCode, string(body))
			continue
		}

		// Success! Note: provCancel is not called here so the response body can be streamed
		_ = provCancel
		return resp, prov, nil
	}

	return nil, nil, fmt.Errorf("all providers failed in fallback chain. Last error: %w", lastErr)
}

func bytesNewReader(b []byte) io.Reader {
	return &byteReader{b: b, i: 0}
}

type byteReader struct {
	b []byte
	i int
}

func (r *byteReader) Read(p []byte) (n int, err error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n = copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
