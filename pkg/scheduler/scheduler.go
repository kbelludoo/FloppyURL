package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Xelckis/floppyURL/pkg/watchdog"
)

// ScheduledTask defines a pending or recurring scheduled intention.
type ScheduledTask struct {
	ID        string        `json:"id"`
	CreatedAt time.Time     `json:"created_at"`
	FireAt    time.Time     `json:"fire_at"`
	Interval  time.Duration `json:"interval,omitempty"`
	Prompt    string        `json:"prompt"`
	ChatID    string        `json:"chat_id,omitempty"`
	OneShot   bool          `json:"one_shot"`
	Active    bool          `json:"active"`
}

type AgentRunner func(ctx context.Context, prompt string) (string, error)

type Manager struct {
	mu       sync.RWMutex
	filePath string
	tasks    map[string]*ScheduledTask
	runner   AgentRunner
	ctx      context.Context
	cancel   context.CancelFunc
}

var (
	defaultManager *Manager
	managerOnce    sync.Once
)

func GetManager() *Manager {
	managerOnce.Do(func() {
		home, _ := os.UserHomeDir()
		dir := "data"
		if home != "" {
			dir = filepath.Join(home, ".agent-go")
		}
		_ = os.MkdirAll(dir, 0755)
		defaultManager = NewManager(filepath.Join(dir, "schedules.json"))
	})
	return defaultManager
}

func NewManager(filePath string) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		filePath: filePath,
		tasks:    make(map[string]*ScheduledTask),
		ctx:      ctx,
		cancel:   cancel,
	}
	m.load()
	go m.loop()
	return m
}

func (m *Manager) SetRunner(r AgentRunner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runner = r
}

func (m *Manager) ScheduleReminder(duration time.Duration, prompt, chatID string) *ScheduledTask {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := &ScheduledTask{
		ID:        fmt.Sprintf("remind_%d", time.Now().UnixNano()),
		CreatedAt: time.Now(),
		FireAt:    time.Now().Add(duration),
		Prompt:    prompt,
		ChatID:    chatID,
		OneShot:   true,
		Active:    true,
	}
	m.tasks[task.ID] = task
	_ = m.save()
	log.Printf("[scheduler] ⏰ Lembrete agendado (ID: %s, em: %v): %q", task.ID, duration, prompt)
	return task
}

func (m *Manager) ScheduleRecurring(interval time.Duration, prompt, chatID string) *ScheduledTask {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := &ScheduledTask{
		ID:        fmt.Sprintf("cron_%d", time.Now().UnixNano()),
		CreatedAt: time.Now(),
		FireAt:    time.Now().Add(interval),
		Interval:  interval,
		Prompt:    prompt,
		ChatID:    chatID,
		OneShot:   false,
		Active:    true,
	}
	m.tasks[task.ID] = task
	_ = m.save()
	log.Printf("[scheduler] 🔄 Tarefa recorrente agendada (ID: %s, intervalo: %v): %q", task.ID, interval, prompt)
	return task
}

func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tasks[id]; ok {
		t.Active = false
		delete(m.tasks, id)
		_ = m.save()
		return true
	}
	return false
}

func (m *Manager) List() []ScheduledTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]ScheduledTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		if t.Active {
			res = append(res, *t)
		}
	}
	return res
}

func (m *Manager) loop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkTasks()
		}
	}
}

func (m *Manager) checkTasks() {
	m.mu.Lock()
	now := time.Now()
	var toFire []*ScheduledTask

	for _, t := range m.tasks {
		if t.Active && now.After(t.FireAt) {
			toFire = append(toFire, t)
			if t.OneShot {
				t.Active = false
				delete(m.tasks, t.ID)
			} else {
				t.FireAt = now.Add(t.Interval)
			}
		}
	}
	if len(toFire) > 0 {
		_ = m.save()
	}
	runner := m.runner
	m.mu.Unlock()

	for _, t := range toFire {
		go func(task *ScheduledTask) {
			log.Printf("[scheduler] 🚀 Disparando tarefa agendada: %s (%q)", task.ID, task.Prompt)
			res := ""
			if runner != nil {
				out, err := runner(context.Background(), task.Prompt)
				if err != nil {
					res = fmt.Sprintf("⚠️ Falha ao executar tarefa agendada: %v", err)
				} else {
					res = out
				}
			} else {
				res = fmt.Sprintf("🔔 Lembrete agendado: %s", task.Prompt)
			}

			// Send to Telegram
			tgMsg := fmt.Sprintf("⏰ *LEMBRETE AGENDADO DO AGENT GO*\n\n%s", res)
			watchdog.SendTelegram(tgMsg)
		}(t)
	}
}

func (m *Manager) load() {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return
	}
	var list []ScheduledTask
	if err := json.Unmarshal(data, &list); err == nil {
		for _, t := range list {
			if t.Active {
				cp := t
				m.tasks[t.ID] = &cp
			}
		}
	}
}

func (m *Manager) save() error {
	list := make([]ScheduledTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		if t.Active {
			list = append(list, *t)
		}
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filePath, data, 0644)
}
