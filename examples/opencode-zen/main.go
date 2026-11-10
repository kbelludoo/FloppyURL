// Command opencode-zen calls the OpenCode Console (Zen) model gateway directly,
// without installing the opencode CLI. It speaks the OpenAI-compatible endpoint,
// so any model listed at https://opencode.ai/docs/zen works by its model id.
//
// Usage:
//
//	OPENCODE_API_KEY=sk-... go run ./examples/opencode-zen -model deepseek-v4.1-flash "explique goroutines"
//	OPENCODE_API_KEY=sk-... go run ./examples/opencode-zen -models
//
// Docs: https://opencode.ai/docs/zen
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// Base endpoints exposed by OpenCode Console.
	openaiBase = "https://opencode.ai/inference/openai/v1"
	modelsURL  = "https://opencode.ai/inference/v1/models"
)

// Client is a minimal OpenAI-compatible client for the OpenCode gateway.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		baseURL: openaiBase,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 0}, // streaming: no global timeout
	}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message      message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) newRequest(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// Chat sends a single non-streaming prompt and returns the full reply.
func (c *Client) Chat(ctx context.Context, model, prompt string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:    model,
		Messages: []message{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}

	req, err := c.newRequest(ctx, http.MethodPost, c.baseURL+"/chat/completions", body)
	if err != nil {
		return "", err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("opencode http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if out.Error != nil {
		return "", errors.New("opencode: " + out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", errors.New("opencode: empty choices")
	}
	return out.Choices[0].Message.Content, nil
}

// StreamChat streams a reply, invoking onDelta for each text chunk.
func (c *Client) StreamChat(ctx context.Context, model, prompt string, onDelta func(string)) error {
	body, err := json.Marshal(chatRequest{
		Model:    model,
		Messages: []message{{Role: "user", Content: prompt}},
		Stream:   true,
	})
	if err != nil {
		return err
	}

	req, err := c.newRequest(ctx, http.MethodPost, c.baseURL+"/chat/completions", body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("opencode http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			onDelta(chunk.Choices[0].Delta.Content)
		}
	}
	return scanner.Err()
}

// ListModels fetches the model catalog.
func (c *Client) ListModels(ctx context.Context) ([]byte, error) {
	req, err := c.newRequest(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func main() {
	model := flag.String("model", "deepseek-v4.1-flash", "OpenCode model id (e.g. claude-sonnet-4-5, gpt-5.5, glm-5.3)")
	stream := flag.Bool("stream", false, "stream the reply token by token")
	list := flag.Bool("models", false, "print the available models and exit")
	timeout := flag.Duration("timeout", 120*time.Second, "request timeout")
	flag.Parse()

	apiKey := os.Getenv("OPENCODE_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "defina OPENCODE_API_KEY (crie uma em https://opencode.ai/console)")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client := NewClient(apiKey)

	if *list {
		out, err := client.ListModels(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "erro:", err)
			os.Exit(1)
		}
		fmt.Println(string(out))
		return
	}

	prompt := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if prompt == "" {
		fmt.Fprintln(os.Stderr, "informe um prompt, ex: -model gpt-5.5 \"olá\"")
		os.Exit(1)
	}

	if *stream {
		if err := client.StreamChat(ctx, *model, prompt, func(s string) { fmt.Print(s) }); err != nil {
			fmt.Fprintln(os.Stderr, "\nerro:", err)
			os.Exit(1)
		}
		fmt.Println()
		return
	}

	reply, err := client.Chat(ctx, *model, prompt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
	fmt.Println(reply)
}
