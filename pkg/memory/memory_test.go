package memory

import (
	"path/filepath"
	"testing"
)

func TestCoreMemory(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_core.json")

	cm := NewCoreMemory(path)
	if err := cm.Set("objective", "Garantir 100% de uptime dos bots"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val := cm.Get("objective")
	if val != "Garantir 100% de uptime dos bots" {
		t.Fatalf("unexpected val: %q", val)
	}

	if err := cm.Append("objective", "Manter RAM abaixo de 500MB"); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	valAfter := cm.Get("objective")
	if valAfter != "Garantir 100% de uptime dos bots\nManter RAM abaixo de 500MB" {
		t.Fatalf("unexpected append val: %q", valAfter)
	}

	prompt := cm.RenderPrompt()
	if prompt == "" || !testing.Short() {
		t.Logf("Rendered prompt:\n%s", prompt)
	}

	// Verify persistence by reloading
	cmReloaded := NewCoreMemory(path)
	if cmReloaded.Get("objective") != valAfter {
		t.Fatalf("persisted value mismatch on reload")
	}
}

func TestEpisodicMemorySearch(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_episodic.json")

	em := NewEpisodicMemory(path)

	_, err := em.Save("Falha no bot Huntera na porta 3000", "Ao cair o huntera, rodar bash /home/ubuntu/huntera-go/start.sh", []string{"huntera", "port3000", "crash"})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	_, err = em.Save("Ajuste de swap na Oracle VPS", "Swap configurado em 1GB com swappiness 10", []string{"swap", "memory", "vps"})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Search for huntera
	results := em.Search("huntera caiu na porta 3000", 3)
	if len(results) == 0 {
		t.Fatalf("expected search results for huntera")
	}
	if results[0].Record.Title != "Falha no bot Huntera na porta 3000" {
		t.Fatalf("expected top result to be huntera, got: %q (score: %f)", results[0].Record.Title, results[0].Score)
	}

	// Search for swap
	swapResults := em.Search("memoria swap", 3)
	if len(swapResults) == 0 {
		t.Fatalf("expected search results for swap")
	}
	if swapResults[0].Record.Title != "Ajuste de swap na Oracle VPS" {
		t.Fatalf("expected top result to be swap, got: %q", swapResults[0].Record.Title)
	}

	// List recent
	recent := em.ListRecent(5)
	if len(recent) < 2 {
		t.Fatalf("expected at least 2 recent entries, got %d", len(recent))
	}
}
