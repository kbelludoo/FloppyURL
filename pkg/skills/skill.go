package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Xelckis/floppyURL/pkg/watchdog"
)

// SkillTool defines a tool provided by a skill.
type SkillTool struct {
	Name        string                                                 `json:"name"`
	Description string                                                 `json:"description"`
	Parameters  json.RawMessage                                        `json:"parameters"`
	Execute     func(ctx context.Context, args string) (string, error) `json:"-"`
}

// Skill represents a modular Hermes-style capability pack.
type Skill struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Prompt      string      `json:"prompt"`
	Tools       []SkillTool `json:"tools"`
}

// SkillRegistry manages available and active skills.
type SkillRegistry struct {
	mu     sync.RWMutex
	skills map[string]*Skill
	active map[string]bool
}

var (
	defaultRegistry *SkillRegistry
	registryOnce    sync.Once
)

func GetRegistry() *SkillRegistry {
	registryOnce.Do(func() {
		defaultRegistry = NewRegistry()
		defaultRegistry.registerBuiltins()
		defaultRegistry.loadSkillsFromDisk()
	})
	return defaultRegistry
}

func NewRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills: make(map[string]*Skill),
		active: make(map[string]bool),
	}
}

func (r *SkillRegistry) Register(s *Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[s.ID] = s
}

func (r *SkillRegistry) Activate(id string) (*Skill, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.skills[id]
	if !ok {
		return nil, fmt.Errorf("skill '%s' não encontrada no catálogo", id)
	}
	r.active[id] = true
	return s, nil
}

// CreateSkill creates a new persistent SKILL.md and registers it immediately.
func (r *SkillRegistry) CreateSkill(id, name, description, prompt string) (*Skill, error) {
	cleanID := strings.ToLower(strings.TrimSpace(id))
	cleanID = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(cleanID, "-")
	if cleanID == "" {
		return nil, fmt.Errorf("ID de skill inválido")
	}

	baseDir := "skills"
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		baseDir = filepath.Join(home, ".agent-go", "skills")
	}
	skillFolder := filepath.Join(baseDir, cleanID)
	if err := os.MkdirAll(skillFolder, 0755); err != nil {
		return nil, fmt.Errorf("erro ao criar diretório da skill: %w", err)
	}

	content := fmt.Sprintf(`---
name: %q
description: %q
id: %q
---

%s
`, name, description, cleanID, strings.TrimSpace(prompt))

	filePath := filepath.Join(skillFolder, "SKILL.md")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("erro ao salvar SKILL.md: %w", err)
	}

	skill := &Skill{
		ID:          cleanID,
		Name:        name,
		Description: description,
		Prompt:      strings.TrimSpace(prompt),
		Tools:       []SkillTool{},
	}

	r.Register(skill)
	_, _ = r.Activate(cleanID)
	return skill, nil
}

func (r *SkillRegistry) Deactivate(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.active, id)
}

func (r *SkillRegistry) IsActive(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active[id]
}

func (r *SkillRegistry) List() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]map[string]any, 0, len(r.skills))
	for id, s := range r.skills {
		toolNames := make([]string, 0, len(s.Tools))
		for _, t := range s.Tools {
			toolNames = append(toolNames, t.Name)
		}
		res = append(res, map[string]any{
			"id":          id,
			"name":        s.Name,
			"description": s.Description,
			"tools":       toolNames,
			"active":      r.active[id],
		})
	}
	return res
}

// RenderActivePrompts renders instructions for all currently active skills into the system prompt.
func (r *SkillRegistry) RenderActivePrompts() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder
	for id, active := range r.active {
		if active {
			if s, ok := r.skills[id]; ok && s.Prompt != "" {
				sb.WriteString(fmt.Sprintf("\n### [Active Skill: %s (%s)]\n%s\n", s.Name, s.ID, s.Prompt))
			}
		}
	}
	return sb.String()
}

// GetActiveToolDefs returns OpenAI-compatible tool definitions for all active skills.
func (r *SkillRegistry) GetActiveToolDefs() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var defs []map[string]any
	for id, active := range r.active {
		if !active {
			continue
		}
		s, ok := r.skills[id]
		if !ok {
			continue
		}
		for _, t := range s.Tools {
			defs = append(defs, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			})
		}
	}
	return defs
}

// ExecuteActiveTool executes a tool from any active skill. Returns true if the tool was found and executed.
func (r *SkillRegistry) ExecuteActiveTool(ctx context.Context, name, arguments string) (string, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for id, active := range r.active {
		if !active {
			continue
		}
		s, ok := r.skills[id]
		if !ok {
			continue
		}
		for _, t := range s.Tools {
			if t.Name == name {
				if t.Execute != nil {
					res, err := t.Execute(ctx, arguments)
					return res, true, err
				}
				return "", true, fmt.Errorf("tool '%s' tem executor nulo", name)
			}
		}
	}
	return "", false, nil
}

// registerBuiltins mounts the core skills: vps-guardian, browser-pilot, and system-admin.
func (r *SkillRegistry) registerBuiltins() {
	// 1. VPS Guardian Skill
	r.Register(&Skill{
		ID:          "vps-guardian",
		Name:        "VPS Bot Guardian & Fleet Auto-Heal",
		Description: "Monitoramento em tempo real, auto-cura e diagnósticos dos 9 bots da VPS (huntera, tibidle, sigma, leveling, pokeidle, pokeidleworld, kickminer, twitchminer, baiak)",
		Prompt: `Você tem autoridade total para monitorar, reiniciar e diagnosticar a frota de 9 bots da VPS:
- Portas 3000-3010 e 8090.
- Se o usuário relatar falha em algum bot, use 'vps_bots_status' para verificar, 'vps_bot_logs' para ver logs de erro e 'vps_bot_restart' para reiniciar.
- O restart no systemd requer sudo (já configurado sem senha para o usuário ubuntu).`,
		Tools: []SkillTool{
			{
				Name:        "vps_bots_status",
				Description: "Retorna a saúde em tempo real dos 9 bots da VPS (portas, latências, estado online/offline e memória RAM).",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
				Execute: func(ctx context.Context, args string) (string, error) {
					rep := watchdog.GetFleetStatus()
					b, err := json.MarshalIndent(rep, "", "  ")
					if err != nil {
						return fmt.Sprintf("Erro ao gerar status da frota: %v", err), nil
					}
					return string(b), nil
				},
			},
			{
				Name:        "vps_bot_restart",
				Description: "Reinicia com segurança um bot específico da VPS pelo ID ('huntera', 'tibidle', 'sigma', 'leveling', 'pokeidle', 'pokeidleworld', 'kickminer', 'twitchminer', 'baiak') e testa se voltou a responder.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"bot_id":{"type":"string","description":"O ID do bot a reiniciar"}},"required":["bot_id"]}`),
				Execute: func(ctx context.Context, args string) (string, error) {
					var p struct {
						BotID string `json:"bot_id"`
					}
					if err := json.Unmarshal([]byte(args), &p); err != nil {
						return "", err
					}
					ok, msg := watchdog.RestartBot(p.BotID)
					if !ok {
						return fmt.Sprintf("Falha: %s", msg), nil
					}
					return msg, nil
				},
			},
			{
				Name:        "vps_bot_logs",
				Description: "Obtém os últimos logs do systemd ou processo para um bot específico a fim de diagnosticar erros.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"bot_id":{"type":"string","description":"O ID do bot"},"lines":{"type":"integer","description":"Número de linhas (default: 30)"}},"required":["bot_id"]}`),
				Execute: func(ctx context.Context, args string) (string, error) {
					var p struct {
						BotID string `json:"bot_id"`
						Lines int    `json:"lines"`
					}
					if err := json.Unmarshal([]byte(args), &p); err != nil {
						return "", err
					}
					if p.Lines <= 0 {
						p.Lines = 30
					}
					return watchdog.GetBotLogs(p.BotID, p.Lines), nil
				},
			},
		},
	})

	// Active by default on VPS
	r.active["vps-guardian"] = true

	// 2. Browser Pilot Skill
	r.Register(&Skill{
		ID:          "browser-pilot",
		Name:        "AIGoBrowser Web Navigation Engine",
		Description: "Navegação na web e extração de Markdown/links utilizando o motor ultra-leve Go AIGoBrowser (<20MB RAM).",
		Prompt: `Você possui acesso ao AIGoBrowser ultra-leve em Go puro para navegar na web, extrair conteúdo estruturado em Markdown e links limpos.
Use 'browser_navigate' sempre que precisar inspecionar sites, ler documentações ou consultar páginas externas.`,
		Tools: []SkillTool{
			{
				Name:        "browser_navigate",
				Description: "Navega para qualquer página web utilizando o motor ultra-leve Go AIGoBrowser (<20MB RAM) e retorna Markdown estruturado e links.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"A URL de destino a visitar (ex: https://news.ycombinator.com)"},"render_js":{"type":"boolean","description":"Se verdadeiro, renderiza via worker JS"}},"required":["url"]}`),
				Execute: func(ctx context.Context, args string) (string, error) {
					var p struct {
						URL      string `json:"url"`
						RenderJS bool   `json:"render_js"`
					}
					if err := json.Unmarshal([]byte(args), &p); err != nil {
						return "", err
					}

					bin := "aigobrowser"
					if runtime.GOOS == "windows" {
						if _, err := os.Stat("aigobrowser.exe"); err == nil {
							bin = ".\\aigobrowser.exe"
						} else if _, err := os.Stat("bin\\aigobrowser.exe"); err == nil {
							bin = ".\\bin\\aigobrowser.exe"
						} else if _, err := os.Stat("C:\\Users\\kbell\\Downloads\\ai-go-browser-colecao-completa\\current-project\\ai-go-browser-action-resolver\\bin\\aigobrowser.exe"); err == nil {
							bin = "C:\\Users\\kbell\\Downloads\\ai-go-browser-colecao-completa\\current-project\\ai-go-browser-action-resolver\\bin\\aigobrowser.exe"
						}
					} else {
						if _, err := os.Stat("/usr/local/bin/aigobrowser"); err == nil {
							bin = "/usr/local/bin/aigobrowser"
						} else if _, err := os.Stat("/home/ubuntu/ai-go-browser/bin/aigobrowser"); err == nil {
							bin = "/home/ubuntu/ai-go-browser/bin/aigobrowser"
						}
					}

					cmdArgs := fmt.Sprintf("%s -format markdown ", bin)
					if p.RenderJS {
						cmdArgs = fmt.Sprintf("%s -render-js -format markdown ", bin)
					}
					cmdArgs += fmt.Sprintf("%q", p.URL)

					tctx, cancel := context.WithTimeout(ctx, 45*time.Second)
					defer cancel()

					var cmd *exec.Cmd
					if runtime.GOOS == "windows" {
						cmd = exec.CommandContext(tctx, "powershell", "-NoProfile", "-Command", cmdArgs)
					} else {
						cmd = exec.CommandContext(tctx, "bash", "-c", cmdArgs)
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
					return strings.TrimSpace(output), nil
				},
			},
		},
	})

	// Both core skills active by default
	r.active["browser-pilot"] = true
}

// loadSkillsFromDisk loads any custom SKILL.md files from ./skills or ~/.agent-go/skills
func (r *SkillRegistry) loadSkillsFromDisk() {
	searchDirs := []string{
		"skills",
		filepath.Join(os.Getenv("HOME"), ".agent-go", "skills"),
		"/home/ubuntu/.agent-go/skills",
	}

	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				skillMD := filepath.Join(dir, e.Name(), "SKILL.md")
				if data, err := os.ReadFile(skillMD); err == nil {
					s := parseSkillMarkdown(e.Name(), string(data))
					if s != nil {
						r.Register(s)
					}
				}
			}
		}
	}
}

// parseSkillMarkdown parses a SKILL.md file with basic YAML frontmatter.
func parseSkillMarkdown(folderName, content string) *Skill {
	s := &Skill{
		ID:          folderName,
		Name:        folderName,
		Description: "Skill carregada de " + folderName + "/SKILL.md",
	}

	lines := strings.Split(content, "\n")
	if len(lines) > 2 && strings.TrimSpace(lines[0]) == "---" {
		bodyStart := 1
		for i := 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "---" {
				bodyStart = i + 1
				break
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				k := strings.ToLower(strings.TrimSpace(parts[0]))
				v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
				switch k {
				case "name":
					s.Name = v
				case "description":
					s.Description = v
				case "id":
					s.ID = v
				}
			}
		}
		if bodyStart < len(lines) {
			s.Prompt = strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
		}
	} else {
		s.Prompt = strings.TrimSpace(content)
	}

	return s
}
