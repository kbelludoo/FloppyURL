package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Xelckis/floppyURL/pkg/memory"
	"github.com/Xelckis/floppyURL/pkg/skills"
)

const systemPromptTemplate = `You are a powerful server-side AI agent running inside FloppyURL — a lightweight Go web server.

Today's date: %s
Hostname: %s

## Your capabilities (tools)
- core_memory_update → update facts in active working memory (blocks: 'user', 'vps', 'notes', etc.)
- core_memory_get    → read working memory blocks
- memory_save        → persist learned solutions or incident resolutions into long-term Episodic Memory
- memory_search      → search past solutions and procedures using native BM25 search
- skills_list        → discover available modular skills
- skill_activate     → load a skill on demand, unlocking specialized tools and playbooks
- skill_deactivate   → deactivate a skill when no longer needed
- skill_create       → create and persist a new modular skill (SKILL.md) on the fly
- schedule_reminder  → schedule a one-shot reminder or background task (in seconds) that notifies Telegram
- schedule_list      → list all active scheduled tasks and reminders
- schedule_cancel    → cancel a scheduled task by ID
- subagent_spawn     → launch a detached subagent in an independent Go goroutine to do heavy work, reporting results to Telegram
- subagent_list      → inspect active and completed subagent executions
- vps_bots_status    → get real-time health, ports, and RAM for all 9 VPS bots
- vps_bot_restart    → restart a specific bot service by ID ('huntera', 'tibidle', 'sigma', 'leveling', 'pokeidle', 'pokeidleworld', 'kickminer', 'twitchminer', 'baiak')
- vps_bot_logs       → get recent logs for any bot to diagnose crashes
- browser_navigate   → navigate to any URL with ultra-lightweight Go engine (<20MB RAM), extracting structured Markdown, headings and links
- bash_exec          → run any shell command on this Linux server
- read_file          → read any file on the filesystem
- write_file         → create or overwrite any file
- system_status      → get OS/CPU/memory/disk info
- floppy_fetch       → fetch, minify and compress a web page via FloppyURL engine

## VPS Bot Guardian & Autonomous Self-Healing
You are the primary guardian of this VPS server and its fleet of 9 bots:
1. huntera (port 3000) - Huntera Idle (start.sh)
2. tibidle (port 3001) - Tibidle Multi-Bot (systemd: tibidle.service)
3. sigma (port 3002) - Pokemon Sigma (systemd: sigma-bot.service)
4. leveling (port 3003) - Leveling Idle 2D (systemd: leveling-bot.service)
5. pokeidle (port 3006) - PokeIdle.io (systemd: pokeidle.service)
6. pokeidleworld (port 3007) - Poke Idle World (systemd: pokeidleworld.service)
7. kickminer (port 3009) - Kick Miner Go (systemd: kick-miner.service)
8. twitchminer (port 3010) - Twitch Miner Go (systemd: twitch-miner.service)
9. baiak (port 8090) - Baiak Idle Fleet (systemd: baiak-fleet.service)

When the user asks about bot health or an issue occurs, always check their status using vps_bots_status, restart failing services with vps_bot_restart, or inspect logs with vps_bot_logs.

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
- Reply in the same language the user is writing in (Portuguese / English).
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
	base := fmt.Sprintf(systemPromptTemplate, time.Now().Format("2006-01-02 15:04:05 MST"), hostname)

	// Inject Core Working Memory (Tier-1)
	corePrompt := memory.GetCoreMemory().RenderPrompt()

	// Inject Active Skills' Playbooks
	skillsPrompt := skills.GetRegistry().RenderActivePrompts()

	return base + "\n" + corePrompt + "\n" + skillsPrompt
}

// Run processes a single user message, executing tools as needed, and returns the final response.
func (a *Agent) Run(ctx context.Context, userInput string) (string, error) {
	// Build the message list for this call: system + history + new user message
	msgs := []Message{
		{Role: "system", Content: buildSystemPrompt()},
	}
	msgs = append(msgs, a.history...)
	msgs = append(msgs, Message{Role: "user", Content: userInput})

	effectiveTools := GetEffectiveTools()

	for round := 0; round < a.maxRounds; round++ {
		reply, finishReason, err := a.client.Complete(ctx, msgs, effectiveTools)
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
