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
	"time"

	"github.com/Xelckis/floppyURL/pkg/server"
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
}

// ExecuteTool runs the requested tool by name with JSON arguments.
func ExecuteTool(ctx context.Context, name, arguments string) (string, error) {

	switch name {
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
