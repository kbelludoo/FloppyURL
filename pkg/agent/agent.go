package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

const systemPromptTemplate = `You are a powerful server-side AI agent running inside FloppyURL — a lightweight Go web server.

Today's date: %s
Hostname: %s

## Your capabilities (tools)
- browser_navigate → navigate to any URL with ultra-lightweight Go engine (<20MB RAM), extracting structured Markdown, headings and links
- bash_exec        → run any shell command on this Linux server
- read_file        → read any file on the filesystem
- write_file       → create or overwrite any file
- system_status    → get OS/CPU/memory/disk info
- floppy_fetch     → fetch, minify and compress a web page via FloppyURL engine

## Self-Healing & Browser Evolution
- The source code for the browser is located at /home/ubuntu/ai-go-browser.
- If browser_navigate encounters an error on a specific site (e.g. unknown tag, encoding issue, parser edge case, or unsupported content type):
  1. Inspect the error and read the corresponding code in /home/ubuntu/ai-go-browser/browser/ using read_file.
  2. Implement a fix using write_file.
  3. Run tests and rebuild using bash_exec: cd /home/ubuntu/ai-go-browser && go test ./... && go build -o /usr/local/bin/aigobrowser ./cmd/aigobrowser.
  4. Retry browser_navigate to verify the fix works.

## Rules
- Always think step by step before calling a tool.
- Prefer short, precise commands (cat, systemctl status, free -h, etc.).
- Never run destructive commands (rm -rf /, format) unless the user explicitly insists.
- Reply in the same language the user is writing in.
- When you have enough information to answer, stop calling tools and respond directly.
`

// Agent manages conversational state and runs the agentic loop.
type Agent struct {
	client    *LLMClient
	history   []Message
	maxRounds int
}

// New creates a new Agent using environment-configured LLM settings.
func New() *Agent {
	return &Agent{
		client:    NewLLMClient("", "", ""),
		history:   []Message{},
		maxRounds: 20,
	}
}

// NewWithModel creates a new Agent with a custom model.
func NewWithModel(model string) *Agent {
	return &Agent{
		client:    NewLLMClient("", "", model),
		history:   []Message{},
		maxRounds: 20,
	}
}

// SetModel overrides the model used by the agent.
func (a *Agent) SetModel(model string) {
	if model != "" && a.client != nil {
		a.client.SetModel(model)
	}
}

// NewStateless creates an Agent with no shared history (each call is independent).
func NewStateless() *Agent {
	a := New()
	a.history = nil
	return a
}


func buildSystemPrompt() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	return fmt.Sprintf(systemPromptTemplate, time.Now().Format("2006-01-02 15:04:05 MST"), hostname)
}

// Run processes a single user message, executing tools as needed, and returns the final response.
func (a *Agent) Run(ctx context.Context, userInput string) (string, error) {
	// Build the message list for this call: system + history + new user message
	msgs := []Message{
		{Role: "system", Content: buildSystemPrompt()},
	}
	msgs = append(msgs, a.history...)
	msgs = append(msgs, Message{Role: "user", Content: userInput})

	for round := 0; round < a.maxRounds; round++ {
		reply, finishReason, err := a.client.Complete(ctx, msgs, AvailableTools)
		if err != nil {
			return "", fmt.Errorf("LLM error (round %d): %w", round+1, err)
		}

		// Append assistant message to the running list
		msgs = append(msgs, *reply)

		if finishReason == "stop" || len(reply.ToolCalls) == 0 {
			// Final answer — update shared history and return
			a.history = append(a.history,
				Message{Role: "user", Content: userInput},
				*reply,
			)
			// Trim history to last 40 messages to keep memory bounded
			if len(a.history) > 40 {
				a.history = a.history[len(a.history)-40:]
			}
			content := strings.TrimSpace(reply.Content)
			if content == "" {
				content = "(sem resposta)"
			}
			return content, nil
		}

		// Execute each tool call and append results
		for _, tc := range reply.ToolCalls {
			result, execErr := ExecuteTool(ctx, tc.Function.Name, tc.Function.Arguments)
			if execErr != nil {
				result = fmt.Sprintf("Tool error: %v", execErr)
			}
			msgs = append(msgs, Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return "", fmt.Errorf("agent reached maximum rounds (%d) without a final answer", a.maxRounds)
}
