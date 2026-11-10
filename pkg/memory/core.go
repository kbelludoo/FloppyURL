package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CoreMemory represents the active working memory / scratchpad injected into the system prompt.
// It holds key persistent facts about the user, the server environment, and active objectives.
type CoreMemory struct {
	mu       sync.RWMutex
	filePath string
	Blocks   map[string]string `json:"blocks"`
}

func getDefaultMemoryDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "data"
	}
	dir := filepath.Join(home, ".agent-go")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// DefaultCoreMemory returns a singleton or instance stored in the default location.
var (
	defaultCore *CoreMemory
	coreOnce    sync.Once
)

func GetCoreMemory() *CoreMemory {
	coreOnce.Do(func() {
		dir := getDefaultMemoryDir()
		defaultCore = NewCoreMemory(filepath.Join(dir, "core_memory.json"))
	})
	return defaultCore
}

// NewCoreMemory creates a new CoreMemory instance backed by filePath.
func NewCoreMemory(filePath string) *CoreMemory {
	cm := &CoreMemory{
		filePath: filePath,
		Blocks:   make(map[string]string),
	}
	cm.load()

	// Default baseline facts if empty
	if len(cm.Blocks) == 0 {
		cm.Blocks["user"] = "Comandante (KbElLuDoO LORD). Prefere respostas diretas, técnicas e em português."
		cm.Blocks["vps"] = "Oracle Cloud VPS (129.153.157.17), 1GB RAM total. Monitora 9 bots nas portas 3000-3010 e 8090. Watchdog nativo rodando a cada 45s."
		cm.Blocks["notes"] = "opencode serve foi desativado permanentemente para liberar ~300MB de RAM. Usar fallback Apinex/CodeCraft/TokenHarbor."
		_ = cm.save()
	}

	return cm
}

// RenderPrompt outputs the core memory as an XML-tagged markdown block for the system prompt.
func (cm *CoreMemory) RenderPrompt() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.Blocks) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n<core_memory>\n")
	sb.WriteString("# Working Memory (Hermes Tier-1 Active Facts):\n")
	for block, content := range cm.Blocks {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", block, content))
	}
	sb.WriteString("</core_memory>\n")
	return sb.String()
}

// Get retrieves the value of a specific memory block.
func (cm *CoreMemory) Get(block string) string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Blocks[block]
}

// Set updates or creates a memory block and saves to disk.
func (cm *CoreMemory) Set(block, content string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.Blocks[block] = strings.TrimSpace(content)
	return cm.save()
}

// Append appends text to an existing block or creates it if it doesn't exist.
func (cm *CoreMemory) Append(block, text string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	existing := cm.Blocks[block]
	if existing == "" {
		cm.Blocks[block] = strings.TrimSpace(text)
	} else {
		cm.Blocks[block] = existing + "\n" + strings.TrimSpace(text)
	}
	return cm.save()
}

// Delete removes a block.
func (cm *CoreMemory) Delete(block string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.Blocks, block)
	return cm.save()
}

// ListBlocks returns all existing block names and content.
func (cm *CoreMemory) ListBlocks() map[string]string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	res := make(map[string]string, len(cm.Blocks))
	for k, v := range cm.Blocks {
		res[k] = v
	}
	return res
}

func (cm *CoreMemory) load() {
	data, err := os.ReadFile(cm.filePath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &cm.Blocks)
}

func (cm *CoreMemory) save() error {
	if cm.filePath == "" {
		return nil
	}
	dir := filepath.Dir(cm.filePath)
	_ = os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(cm.Blocks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cm.filePath, data, 0644)
}
