package scheduler

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerReminder(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(filepath.Join(tmpDir, "test_schedules.json"))
	defer mgr.cancel()

	var executed atomic.Int32
	mgr.SetRunner(func(ctx context.Context, prompt string) (string, error) {
		executed.Add(1)
		return "ok: " + prompt, nil
	})

	task := mgr.ScheduleReminder(10*time.Millisecond, "Teste Lembrete Rápido", "")
	if task == nil || task.ID == "" {
		t.Fatalf("esperado task válido, obteve nil")
	}

	list := mgr.List()
	if len(list) != 1 {
		t.Fatalf("esperado 1 tarefa na lista, obteve %d", len(list))
	}

	// Força o avanço no tempo
	time.Sleep(30 * time.Millisecond)
	mgr.checkTasks()

	// Aguarda goroutine de execução
	time.Sleep(50 * time.Millisecond)

	if executed.Load() < 1 {
		t.Fatalf("esperava que o lembrete tivesse executado")
	}

	// One-shot deve ter sido desativado após disparo
	listAfter := mgr.List()
	if len(listAfter) != 0 {
		t.Fatalf("esperado 0 tarefas ativas após one-shot, obteve %d", len(listAfter))
	}
}

func TestSchedulerCancel(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(filepath.Join(tmpDir, "test_schedules_cancel.json"))
	defer mgr.cancel()

	task := mgr.ScheduleReminder(10*time.Minute, "Lembrete Longo", "")
	if len(mgr.List()) != 1 {
		t.Fatalf("esperado 1 tarefa ativa")
	}

	if !mgr.Cancel(task.ID) {
		t.Fatalf("falha ao cancelar tarefa existente")
	}

	if len(mgr.List()) != 0 {
		t.Fatalf("tarefa não foi removida após cancel")
	}
}
