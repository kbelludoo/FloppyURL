package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kbelludoo/FloppyURL/pkg/knowledge"
	"github.com/kbelludoo/FloppyURL/pkg/memory"
	"github.com/kbelludoo/FloppyURL/pkg/neural"
	"github.com/kbelludoo/FloppyURL/pkg/scheduler"
	"github.com/kbelludoo/FloppyURL/pkg/server"
	"github.com/kbelludoo/FloppyURL/pkg/skills"
	"github.com/kbelludoo/FloppyURL/pkg/subagent"
	"github.com/kbelludoo/FloppyURL/pkg/verifier"
	"github.com/kbelludoo/FloppyURL/pkg/watchdog"
)

// ToolDef defines a callable function for the LLM.
type ToolDef struct {
	Type     string       `json:"type"`
	Function FunctionDef  `json:"function"`
}

type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall represents a tool execution requested by the LLM.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Built-in tool definitions formatted for OpenAI-compatible tool calling.
var AvailableTools = []ToolDef{
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "bash_exec",
			Description: "Executes a shell command on the host server and returns the stdout and stderr.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command": {
						"type": "string",
						"description": "The command line string to execute"
					}
				},
				"required": ["command"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "read_file",
			Description: "Reads the content of a file on the local filesystem.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {
						"type": "string",
						"description": "Absolute or relative file path to read"
					}
				},
				"required": ["path"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "write_file",
			Description: "Writes content to a file on the local filesystem.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {
						"type": "string",
						"description": "Path to the file to create or overwrite"
					},
					"content": {
						"type": "string",
						"description": "Text content to write into the file"
					}
				},
				"required": ["path", "content"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "system_status",
			Description: "Returns current server resources: OS, CPU, memory usage, uptime and disk space.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "floppy_fetch",
			Description: "Fetches a URL using FloppyURL's engine, minifying and calculating compression metrics.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {
						"type": "string",
						"description": "The web URL to fetch and compress"
					}
				},
				"required": ["url"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "browser_navigate",
			Description: "Navigates to any web page using the lightweight AIGoBrowser engine (<20MB RAM) and returns structured Markdown, headings, links and actions.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {
						"type": "string",
						"description": "The destination URL to visit (e.g. https://news.ycombinator.com)"
					},
					"render_js": {
						"type": "boolean",
						"description": "Set to true if the page requires JavaScript/SPA rendering"
					}
				},
				"required": ["url"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "list_available_models",
			Description: "Lists recommended free NVIDIA NIM models and active router aliases.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "vps_bots_status",
			Description: "Returns real-time status of all 9 gaming and automation bots on this VPS (huntera, tibidle, sigma, leveling, pokeidle, pokeidleworld, kickminer, twitchminer, baiak), showing online/offline status, ports, latency, and RAM.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "vps_bot_restart",
			Description: "Safely restarts a specific bot service on the VPS by ID ('huntera', 'tibidle', 'sigma', 'leveling', 'pokeidle', 'pokeidleworld', 'kickminer', 'twitchminer', 'baiak') and verifies if it came back online.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"bot_id": {
						"type": "string",
						"description": "The ID of the bot to restart (e.g. 'sigma', 'baiak', 'huntera')"
					}
				},
				"required": ["bot_id"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "vps_bot_logs",
			Description: "Returns recent journalctl or process logs for a specific bot by ID to diagnose errors or crashes.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"bot_id": {
						"type": "string",
						"description": "The ID of the bot to inspect logs for (e.g. 'sigma', 'baiak', 'huntera')"
					},
					"lines": {
						"type": "integer",
						"description": "Number of log lines to retrieve (default: 30)"
					}
				},
				"required": ["bot_id"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "core_memory_update",
			Description: "Updates or appends knowledge in Core Working Memory (persistent prompt scratchpad: 'user', 'vps', 'notes', etc.).",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"block": {
						"type": "string",
						"description": "The memory block name (e.g. 'user', 'vps', 'notes', 'custom')"
					},
					"content": {
						"type": "string",
						"description": "Text content to save or update"
					},
					"append": {
						"type": "boolean",
						"description": "If true, appends to the existing block instead of overwriting"
					}
				},
				"required": ["block", "content"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "core_memory_get",
			Description: "Retrieves all blocks or a specific block from Core Working Memory.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"block": {
						"type": "string",
						"description": "Optional block name to retrieve. If empty, returns all blocks."
					}
				}
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "memory_save",
			Description: "Saves a learned solution, incident resolution, or fact to long-term Episodic Memory.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"title": {
						"type": "string",
						"description": "Concise title for the memory entry"
					},
					"content": {
						"type": "string",
						"description": "Detailed explanation of the solution, fact, or procedure"
					},
					"tags": {
						"type": "array",
						"items": {"type": "string"},
						"description": "Keywords or tags for fast retrieval (e.g. ['pokeidle', 'port3006', 'systemd'])"
					}
				},
				"required": ["title", "content"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "memory_search",
			Description: "Searches past knowledge, incident resolutions, and learned facts via pure Go BM25 relevance matching.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "Search terms or question to match against past memories"
					},
					"limit": {
						"type": "integer",
						"description": "Maximum number of results to return (default: 5)"
					}
				},
				"required": ["query"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "knowledge_search",
			Description: "Searches the specialized Go Bot Knowledge Brain (vector & semantic procedures for Go concurrency, memory leak prevention, systemd, bot architectures, and game automation).",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "Question or technical problem to search in the Go bot knowledge base (e.g. 'websocket timeout', 'memory leak http transport', 'systemd restart', 'gogc')"
					},
					"top_k": {
						"type": "integer",
						"description": "Number of top results to return (default: 3)"
					}
				},
				"required": ["query"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "code_verify_go",
			Description: "Execution-grounded code verifier: Compiles and tests Go source code in an isolated sandbox with 'go vet', 'go build', and 'go test', returning exact compiler errors or execution output to eliminate probabilistic bugs.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"go_code": {
						"type": "string",
						"description": "The complete Go source code snippet to verify (e.g. package main...)"
					},
					"test_code": {
						"type": "string",
						"description": "Optional unit test code (package main... TestXxx...)"
					}
				},
				"required": ["go_code"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "neural_evolve_step",
			Description: "Executes a safe neuroevolution mutation cycle on the local neural network: clones champion, mutates weights, rigorously tests against the Golden Test Suite, and promotes the candidate ONLY if zero regressions occur.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "dual_mind_deliberate",
			Description: "Submits an action, command, or code change to the 4-persona Dual-Mind Council (Mutant Engineer, Skeptic Judge, Subconscious Guardian, and Mediator), obtaining unanimous proof of execution or veto before any production change.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"type": {
						"type": "string",
						"description": "Action type: 'code_change', 'command', 'bot_restart', or 'optimization'"
					},
					"description": {
						"type": "string",
						"description": "Clear description of the proposed action"
					},
					"go_code": {
						"type": "string",
						"description": "Go source code if proposing a code change"
					},
					"command": {
						"type": "string",
						"description": "Shell command if proposing a command"
					}
				},
				"required": ["type", "description"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "skills_list",
			Description: "Lists all available modular Hermes-style skills, their descriptions, provided tools, and active status.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "skill_activate",
			Description: "Activates a skill modularly (e.g. 'vps-guardian', 'browser-pilot'), dynamically granting specialized playbook instructions and tools.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"skill_id": {
						"type": "string",
						"description": "The ID of the skill to activate (e.g. 'vps-guardian', 'browser-pilot')"
					}
				},
				"required": ["skill_id"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "skill_deactivate",
			Description: "Deactivates a skill to keep the tool context minimal and focused.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"skill_id": {
						"type": "string",
						"description": "The ID of the skill to deactivate"
					}
				},
				"required": ["skill_id"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "skill_create",
			Description: "Creates and saves a brand new persistent skill (SKILL.md) authored by the AI, immediately activating it.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"skill_id": {
						"type": "string",
						"description": "Unique identifier (lowercase with hyphens, e.g. 'baiak-tuner', 'git-sync')"
					},
					"name": {
						"type": "string",
						"description": "Display name of the skill"
					},
					"description": {
						"type": "string",
						"description": "Short description of what the skill does"
					},
					"instructions": {
						"type": "string",
						"description": "Detailed guidelines, rules, or playbooks injected into the prompt when active"
					}
				},
				"required": ["skill_id", "name", "description", "instructions"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "schedule_reminder",
			Description: "Schedules a one-shot reminder or task to be executed after a duration in seconds, notifying Telegram.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"duration_seconds": {
						"type": "integer",
						"description": "Number of seconds from now to execute the task"
					},
					"prompt": {
						"type": "string",
						"description": "The task prompt or reminder message to execute and deliver"
					}
				},
				"required": ["duration_seconds", "prompt"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "schedule_list",
			Description: "Lists all currently pending and active scheduled tasks and reminders.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "schedule_cancel",
			Description: "Cancels a scheduled task or reminder by ID.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"task_id": {
						"type": "string",
						"description": "The ID of the task to cancel"
					}
				},
				"required": ["task_id"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "subagent_spawn",
			Description: "Spawns a detached background subagent in an independent Go goroutine for long-running analyses, delivering results to Telegram when done.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"role": {
						"type": "string",
						"description": "Role or title of the subagent (e.g. 'Log Investigator', 'Security Auditor')"
					},
					"prompt": {
						"type": "string",
						"description": "The task prompt or instruction for the subagent to execute"
					},
					"notify_telegram": {
						"type": "boolean",
						"description": "Whether to send the completion report to Telegram (default: true)"
					}
				},
				"required": ["role", "prompt"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDef{
			Name:        "subagent_list",
			Description: "Lists running, completed, and recent subagent background executions.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
	},
}

// GetEffectiveTools returns the active set of tools: static core tools + tools from active skills.
func GetEffectiveTools() []ToolDef {
	tools := make([]ToolDef, len(AvailableTools))
	copy(tools, AvailableTools)

	reg := skills.GetRegistry()
	for _, def := range reg.GetActiveToolDefs() {
		fn, ok := def["function"].(map[string]any)
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		var paramBytes []byte
		if p, ok := fn["parameters"].(json.RawMessage); ok {
			paramBytes = p
		} else {
			paramBytes, _ = json.Marshal(fn["parameters"])
		}

		// Avoid duplicates
		exists := false
		for _, t := range tools {
			if t.Function.Name == name {
				exists = true
				break
			}
		}
		if !exists {
			tools = append(tools, ToolDef{
				Type: "function",
				Function: FunctionDef{
					Name:        name,
					Description: desc,
					Parameters:  paramBytes,
				},
			})
		}
	}
	return tools
}

var (
	defaultEngine *neural.EvolutionEngine
	evolutionOnce sync.Once
)

func getEvolutionEngine() *neural.EvolutionEngine {
	evolutionOnce.Do(func() {
		net, _ := neural.NewNetwork([]int{4, 8, 2}, []string{"relu", "sigmoid"})
		suite := []neural.TestCase{
			{
				ID:         "bot_health_safety",
				Name:       "Bot health prediction safety",
				Inputs:     []float64{0.1, 0.2, 0.05, 0.9},
				Expected:   []float64{0.95, 0.05},
				IsCritical: true,
				MaxLoss:    0.20,
			},
			{
				ID:         "memory_pressure_invariant",
				Name:       "High swap indicates danger",
				Inputs:     []float64{0.9, 0.8, 0.8, 0.1},
				Expected:   []float64{0.1, 0.9},
				IsCritical: true,
				MaxLoss:    0.25,
			},
		}
		defaultEngine = neural.NewEvolutionEngine(net, suite, "")
	})
	return defaultEngine
}

// ExecuteTool runs the requested tool by name with JSON arguments.
func ExecuteTool(ctx context.Context, name, arguments string) (string, error) {

	// 1. Check if tool is handled by an active modular skill
	if res, handled, err := skills.GetRegistry().ExecuteActiveTool(ctx, name, arguments); handled {
		return res, err
	}

	switch name {
	case "core_memory_update":
		var args struct {
			Block   string `json:"block"`
			Content string `json:"content"`
			Append  bool   `json:"append"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for core_memory_update: %w", err)
		}
		cm := memory.GetCoreMemory()
		var err error
		if args.Append {
			err = cm.Append(args.Block, args.Content)
		} else {
			err = cm.Set(args.Block, args.Content)
		}
		if err != nil {
			return fmt.Sprintf("Erro ao atualizar Core Memory: %v", err), nil
		}
		return fmt.Sprintf("Bloco de memória [%s] atualizado com sucesso no Core Memory.", args.Block), nil

	case "core_memory_get":
		var args struct {
			Block string `json:"block"`
		}
		_ = json.Unmarshal([]byte(arguments), &args)
		cm := memory.GetCoreMemory()
		if args.Block != "" {
			val := cm.Get(args.Block)
			if val == "" {
				return fmt.Sprintf("Bloco [%s] não encontrado no Core Memory.", args.Block), nil
			}
			return fmt.Sprintf("[%s]: %s", args.Block, val), nil
		}
		data, _ := json.MarshalIndent(cm.ListBlocks(), "", "  ")
		return string(data), nil

	case "memory_save":
		var args struct {
			Title   string   `json:"title"`
			Content string   `json:"content"`
			Tags    []string `json:"tags"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for memory_save: %w", err)
		}
		em := memory.GetEpisodicMemory()
		rec, err := em.Save(args.Title, args.Content, args.Tags)
		if err != nil {
			return fmt.Sprintf("Erro ao salvar memória episódica: %v", err), nil
		}
		return fmt.Sprintf("Memória persistida com sucesso (ID: %s, Título: %q).", rec.ID, rec.Title), nil

	case "memory_search":
		var args struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for memory_search: %w", err)
		}
		if args.Limit <= 0 {
			args.Limit = 5
		}
		em := memory.GetEpisodicMemory()
		results := em.Search(args.Query, args.Limit)
		if len(results) == 0 {
			return "Nenhuma memória encontrada com esses termos de busca.", nil
		}
		data, _ := json.MarshalIndent(results, "", "  ")
		return string(data), nil

	case "knowledge_search":
		var args struct {
			Query string `json:"query"`
			TopK  int    `json:"top_k"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for knowledge_search: %w", err)
		}
		if args.TopK <= 0 {
			args.TopK = 3
		}
		brain := knowledge.GetBrain()
		results := brain.SearchKeyword(args.Query, args.TopK)
		if len(results) == 0 {
			return "Nenhum procedimento encontrado no Cérebro de Conhecimento para essa consulta.", nil
		}
		data, _ := json.MarshalIndent(results, "", "  ")
		return string(data), nil

	case "code_verify_go":
		var args struct {
			GoCode   string `json:"go_code"`
			TestCode string `json:"test_code"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for code_verify_go: %w", err)
		}
		res, err := verifier.VerifyGoCode(ctx, args.GoCode, args.TestCode)
		if err != nil {
			return fmt.Sprintf("Erro ao instanciar sandbox de execução: %v", err), nil
		}
		data, _ := json.MarshalIndent(res, "", "  ")
		return string(data), nil

	case "neural_evolve_step":
		eng := getEvolutionEngine()
		promoted, oldFit, newFit, reason := eng.Step()
		status := eng.GetStatus()
		data, _ := json.MarshalIndent(map[string]any{
			"promoted":     promoted,
			"old_fitness":  oldFit,
			"new_fitness":  newFit,
			"verdict":      reason,
			"neural_state": status,
		}, "", "  ")
		return string(data), nil

	case "dual_mind_deliberate":
		var args struct {
			Type        string `json:"type"`
			Description string `json:"description"`
			GoCode      string `json:"go_code"`
			TestCode    string `json:"test_code"`
			Command     string `json:"command"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for dual_mind_deliberate: %w", err)
		}
		proposal := neural.ActionProposal{
			ID:          fmt.Sprintf("prop_%d", time.Now().UnixNano()),
			Type:        args.Type,
			Description: args.Description,
			GoCode:      args.GoCode,
			TestCode:    args.TestCode,
			Command:     args.Command,
			Timestamp:   time.Now(),
		}
		dm := neural.GetDualMind()
		outcome, err := dm.Deliberate(ctx, proposal)
		if err != nil {
			return fmt.Sprintf("Erro no conselho da Mente Dual: %v", err), nil
		}
		data, _ := json.MarshalIndent(outcome, "", "  ")
		return string(data), nil

	case "skills_list":
		reg := skills.GetRegistry()
		list := reg.List()
		data, _ := json.MarshalIndent(list, "", "  ")
		return string(data), nil

	case "skill_activate":
		var args struct {
			SkillID string `json:"skill_id"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for skill_activate: %w", err)
		}
		reg := skills.GetRegistry()
		s, err := reg.Activate(args.SkillID)
		if err != nil {
			return fmt.Sprintf("Falha ao ativar skill: %v", err), nil
		}
		return fmt.Sprintf("Skill '%s' (%s) ativada com sucesso! Instruções e ferramentas especializadas agora estão disponíveis.", s.Name, s.ID), nil

	case "skill_deactivate":
		var args struct {
			SkillID string `json:"skill_id"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for skill_deactivate: %w", err)
		}
		reg := skills.GetRegistry()
		reg.Deactivate(args.SkillID)
		return fmt.Sprintf("Skill '%s' desativada.", args.SkillID), nil

	case "skill_create":
		var args struct {
			SkillID      string `json:"skill_id"`
			Name         string `json:"name"`
			Description  string `json:"description"`
			Instructions string `json:"instructions"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for skill_create: %w", err)
		}
		reg := skills.GetRegistry()
		s, err := reg.CreateSkill(args.SkillID, args.Name, args.Description, args.Instructions)
		if err != nil {
			return fmt.Sprintf("Erro ao criar skill: %v", err), nil
		}
		return fmt.Sprintf("Skill '%s' (%s) criada e ativada com sucesso! Arquivo SKILL.md salvo e pronto para uso.", s.Name, s.ID), nil

	case "schedule_reminder":
		var args struct {
			DurationSeconds int    `json:"duration_seconds"`
			Prompt          string `json:"prompt"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for schedule_reminder: %w", err)
		}
		if args.DurationSeconds <= 0 {
			args.DurationSeconds = 60
		}
		mgr := scheduler.GetManager()
		task := mgr.ScheduleReminder(time.Duration(args.DurationSeconds)*time.Second, args.Prompt, "")
		return fmt.Sprintf("Lembrete agendado com sucesso (ID: %s)! Executará em %d segundos e notificará o Telegram.", task.ID, args.DurationSeconds), nil

	case "schedule_list":
		mgr := scheduler.GetManager()
		list := mgr.List()
		if len(list) == 0 {
			return "Nenhuma tarefa agendada no momento.", nil
		}
		data, _ := json.MarshalIndent(list, "", "  ")
		return string(data), nil

	case "schedule_cancel":
		var args struct {
			TaskID string `json:"task_id"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for schedule_cancel: %w", err)
		}
		mgr := scheduler.GetManager()
		if mgr.Cancel(args.TaskID) {
			return fmt.Sprintf("Tarefa agendada '%s' cancelada com sucesso.", args.TaskID), nil
		}
		return fmt.Sprintf("Tarefa agendada '%s' não encontrada ou já expirada.", args.TaskID), nil

	case "subagent_spawn":
		var args struct {
			Role           string `json:"role"`
			Prompt         string `json:"prompt"`
			NotifyTelegram *bool  `json:"notify_telegram"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for subagent_spawn: %w", err)
		}
		notify := true
		if args.NotifyTelegram != nil {
			notify = *args.NotifyTelegram
		}
		subMgr := subagent.GetManager()
		runner := func(subCtx context.Context, subPrompt string) (string, error) {
			return NewStateless().Run(subCtx, subPrompt)
		}
		rec := subMgr.Spawn(args.Role, args.Prompt, runner, notify)
		return fmt.Sprintf("Subagente disparado com sucesso em goroutine Go independente!\n• ID: %s\n• Papel: %s\n• Prompt: %s\nO resultado será arquivado na memória e enviado ao Telegram ao terminar.", rec.ID, rec.Role, rec.Prompt), nil

	case "subagent_list":
		subMgr := subagent.GetManager()
		list := subMgr.List()
		if len(list) == 0 {
			return "Nenhum subagente registrado ou em execução.", nil
		}
		data, _ := json.MarshalIndent(list, "", "  ")
		return string(data), nil

	case "bash_exec":
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for bash_exec: %w", err)
		}
		return RunBash(ctx, args.Command)

	case "read_file":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for read_file: %w", err)
		}
		data, err := os.ReadFile(args.Path)
		if err != nil {
			return fmt.Sprintf("Error reading file: %v", err), nil
		}
		return string(data), nil

	case "write_file":
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for write_file: %w", err)
		}
		err := os.WriteFile(args.Path, []byte(args.Content), 0644)
		if err != nil {
			return fmt.Sprintf("Error writing file: %v", err), nil
		}
		return fmt.Sprintf("File %s written successfully (%d bytes).", args.Path, len(args.Content)), nil

	case "system_status":
		return GetSystemStatus(), nil

	case "floppy_fetch":
		var args struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for floppy_fetch: %w", err)
		}
		pack, err := server.PackURL(args.URL, "deflate")
		if err != nil {
			return fmt.Sprintf("Error fetching via FloppyURL: %v", err), nil
		}
		jsonBytes, _ := json.MarshalIndent(pack, "", "  ")
		return string(jsonBytes), nil

	case "browser_navigate":
		var args struct {
			URL      string `json:"url"`
			RenderJS bool   `json:"render_js"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for browser_navigate: %w", err)
		}
		cmdArgs := "aigobrowser -format markdown "
		if args.RenderJS {
			cmdArgs = "aigobrowser -render-js -format markdown "
		}
		cmdArgs += fmt.Sprintf("%q", args.URL)
		return RunBash(ctx, cmdArgs)

	case "list_available_models":
		return `Modelos recomendados no NVIDIA NIM (gratuitos):
1. moonshotai/kimi-k3 (Kimi K3 - raciocínio avançado, multimodal) [ATUAL PADRÃO]
2. meta/llama-3.3-70b-instruct (Llama 3.3 70B - muito inteligente)
3. nvidia/llama-3.1-nemotron-70b-instruct (Nemotron 70B - excelente para código e raciocínio)
4. meta/llama-3.1-8b-instruct (Llama 8B - ultra-rápido e leve)
5. microsoft/phi-3-mini-128k-instruct (Phi-3 Mini - leve com contexto longo)

Para trocar, envie o campo "model" na requisição JSON para /agent ou acesse /v1/chat/completions`, nil

	case "vps_bots_status":
		report := watchdog.GetFleetStatus()
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Sprintf("Error serializing fleet status: %v", err), nil
		}
		return string(data), nil

	case "vps_bot_restart":
		var args struct {
			BotID string `json:"bot_id"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for vps_bot_restart: %w", err)
		}
		ok, msg := watchdog.RestartBot(args.BotID)
		if !ok {
			return fmt.Sprintf("Falha: %s", msg), nil
		}
		return msg, nil

	case "vps_bot_logs":
		var args struct {
			BotID string `json:"bot_id"`
			Lines int    `json:"lines"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for vps_bot_logs: %w", err)
		}
		if args.Lines <= 0 {
			args.Lines = 30
		}
		return watchdog.GetBotLogs(args.BotID, args.Lines), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}


// RunBash executes a shell command with a timeout.
func RunBash(ctx context.Context, cmdStr string) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(tctx, "powershell", "-NoProfile", "-Command", cmdStr)
	} else {
		cmd = exec.CommandContext(tctx, "bash", "-c", cmdStr)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}
	if err != nil {
		output += fmt.Sprintf("\nCommand exit error: %v", err)
	}
	if len(output) > 16000 {
		output = output[:16000] + "\n...[truncated output]"
	}
	return strings.TrimSpace(output), nil
}

// GetSystemStatus gathers system metrics.
func GetSystemStatus() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("OS: %s/%s\n", runtime.GOOS, runtime.GOARCH))
	sb.WriteString(fmt.Sprintf("CPUs: %d\n", runtime.NumCPU()))

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	sb.WriteString(fmt.Sprintf("Go Alloc: %.2f MB (Sys: %.2f MB)\n", float64(m.Alloc)/(1024*1024), float64(m.Sys)/(1024*1024)))

	if runtime.GOOS == "linux" {
		out, err := exec.Command("free", "-h").Output()
		if err == nil {
			sb.WriteString("\nMemory (free -h):\n" + string(out))
		}
		out, err = exec.Command("df", "-h", "/").Output()
		if err == nil {
			sb.WriteString("\nDisk Space (df -h):\n" + string(out))
		}
		out, err = exec.Command("uptime").Output()
		if err == nil {
			sb.WriteString("\nUptime: " + strings.TrimSpace(string(out)) + "\n")
		}
	}
	return sb.String()
}
