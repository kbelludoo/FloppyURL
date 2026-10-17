package subagent

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSubagentSpawn(t *testing.T) {
	mgr := &Manager{
		subagents: make(map[string]*SubagentRecord),
	}

	runner := func(ctx context.Context, prompt string) (string, error) {
		return "Diagnóstico concluído para: " + prompt, nil
	}

	rec := mgr.Spawn("Investigador de Logs", "Analise portas 3000-3010", runner, false)
	if rec == nil || rec.ID == "" {
		t.Fatalf("esperado subagente criado, obteve nil")
	}

	if rec.Status != "running" {
		t.Fatalf("status inicial deve ser 'running', obteve %s", rec.Status)
	}

	// Aguarda goroutine finalizar
	time.Sleep(100 * time.Millisecond)

	list := mgr.List()
	if len(list) != 1 {
		t.Fatalf("esperado 1 subagente na lista, obteve %d", len(list))
	}

	finished := list[0]
	if finished.Status != "completed" {
		t.Fatalf("esperado status 'completed', obteve %s", finished.Status)
	}

	if !strings.Contains(finished.Result, "Diagnóstico concluído") {
		t.Fatalf("resultado inesperado: %s", finished.Result)
	}
}
