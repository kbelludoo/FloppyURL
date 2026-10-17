package subagent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Xelckis/floppyURL/pkg/memory"
	"github.com/Xelckis/floppyURL/pkg/watchdog"
)

// SubagentRecord tracks the lifecycle and output of a detached subagent goroutine.
type SubagentRecord struct {
	ID         string    `json:"id"`
	Role       string    `json:"role"`
	Prompt     string    `json:"prompt"`
	Status     string    `json:"status"` // running, completed, failed
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	DurationMs int64     `json:"duration_ms,omitempty"`
	Result     string    `json:"result,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type SubagentRunner func(ctx context.Context, prompt string) (string, error)

type Manager struct {
	mu        sync.RWMutex
	subagents map[string]*SubagentRecord
}

var (
	defaultManager *Manager
	managerOnce    sync.Once
)

func GetManager() *Manager {
	managerOnce.Do(func() {
		defaultManager = &Manager{
			subagents: make(map[string]*SubagentRecord),
		}
	})
	return defaultManager
}

// Spawn launches a detached subagent in its own goroutine without blocking the parent conversation.
func (m *Manager) Spawn(role, prompt string, runner SubagentRunner, notifyTelegram bool) *SubagentRecord {
	m.mu.Lock()
	defer m.mu.Unlock()

	rec := &SubagentRecord{
		ID:        fmt.Sprintf("sub_%d", time.Now().UnixNano()),
		Role:      role,
		Prompt:    prompt,
		Status:    "running",
		StartedAt: time.Now(),
	}
	m.subagents[rec.ID] = rec

	log.Printf("[subagent] 🚀 Subagente disparado (ID: %s, Papel: %q): %q", rec.ID, role, prompt)

	go func(r *SubagentRecord) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		var out string
		var err error

		if runner != nil {
			out, err = runner(ctx, r.Prompt)
		} else {
			err = fmt.Errorf("runner executor não configurado")
		}

		m.mu.Lock()
		r.FinishedAt = time.Now()
		r.DurationMs = r.FinishedAt.Sub(r.StartedAt).Milliseconds()
		if err != nil {
			r.Status = "failed"
			r.Error = err.Error()
		} else {
			r.Status = "completed"
			r.Result = out
		}
		m.mu.Unlock()

		log.Printf("[subagent] ✅ Subagente finalizado (ID: %s, Status: %s, Duração: %dms)", r.ID, r.Status, r.DurationMs)

		// Persist result into episodic memory
		em := memory.GetEpisodicMemory()
		_, _ = em.Save(
			fmt.Sprintf("Relatório do Subagente: %s", r.Role),
			fmt.Sprintf("Prompt: %s\n\nResultado:\n%s", r.Prompt, r.Result),
			[]string{"subagent", r.Role, r.Status},
		)

		if notifyTelegram {
			msgStatus := "✅ Concluído"
			body := r.Result
			if r.Status == "failed" {
				msgStatus = "❌ Falhou"
				body = r.Error
			}
			tgMsg := fmt.Sprintf(
				"🤖 *RELATÓRIO DO SUBAGENTE (%s)*\n\n• *Papel:* `%s`\n• *ID:* `%s`\n• *Duração:* `%.1fs`\n• *Status:* %s\n\n📝 *Resultado:*\n%s",
				msgStatus, r.Role, r.ID, float64(r.DurationMs)/1000.0, msgStatus, body,
			)
			watchdog.SendTelegram(tgMsg)
		}
	}(rec)

	return rec
}

// List returns a snapshot of all active and recent subagents.
func (m *Manager) List() []SubagentRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]SubagentRecord, 0, len(m.subagents))
	for _, s := range m.subagents {
		res = append(res, *s)
	}
	return res
}
