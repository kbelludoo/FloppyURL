package knowledge

import (
	"path/filepath"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	v1 := []float32{1.0, 0.0, 0.0}
	v2 := []float32{1.0, 0.0, 0.0}
	v3 := []float32{0.0, 1.0, 0.0}

	simIdentical := CosineSimilarity(v1, v2)
	if simIdentical < 0.999 {
		t.Fatalf("esperado ~1.0 para vetores identicos, obteve %f", simIdentical)
	}

	simOrthogonal := CosineSimilarity(v1, v3)
	if simOrthogonal > 0.001 {
		t.Fatalf("esperado ~0.0 para vetores ortogonais, obteve %f", simOrthogonal)
	}
}

func TestBinaryRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_brain.bin")

	b1 := NewBrain(3)
	b1.AddChunk(KnowledgeChunk{
		ID:       "chunk_1",
		Title:    "Bot Memory Tuning",
		Category: "performance",
		Tags:     []string{"ram", "tuning"},
		Content:  "Use GOMEMLIMIT=50MiB",
		Vector:   []float32{0.5, 0.5, 0.0},
	})
	b1.AddChunk(KnowledgeChunk{
		ID:       "chunk_2",
		Title:    "WebSocket Reconnect",
		Category: "networking",
		Tags:     []string{"socket", "reconnect"},
		Content:  "Implement exponential backoff",
		Vector:   []float32{0.0, 0.8, 0.6},
	})

	if err := b1.ExportBinary(filePath); err != nil {
		t.Fatalf("falha ao exportar binario: %v", err)
	}

	b2 := NewBrain(3)
	if err := b2.LoadBinary(filePath); err != nil {
		t.Fatalf("falha ao carregar binario: %v", err)
	}

	if b2.TotalChunks() != 2 {
		t.Fatalf("esperado 2 chunks carregados, obteve %d", b2.TotalChunks())
	}

	// Test vector search on loaded brain
	query := []float32{0.5, 0.5, 0.0}
	results := b2.SearchByVector(query, 1)
	if len(results) == 0 || results[0].Chunk.ID != "chunk_1" {
		t.Fatalf("esperado match com chunk_1, obteve %+v", results)
	}
	if results[0].Score < 0.99 {
		t.Fatalf("score esperado ~1.0, obteve %f", results[0].Score)
	}

	// Test keyword search
	kwResults := b2.SearchKeyword("reconnect websocket", 1)
	if len(kwResults) == 0 || kwResults[0].Chunk.ID != "chunk_2" {
		t.Fatalf("esperado match com chunk_2 por keyword, obteve %+v", kwResults)
	}
}

func TestSearchOnDemand(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "ondemand_brain.bin")

	b := NewBrain(3)
	b.AddChunk(KnowledgeChunk{
		ID:       "chunk_leak",
		Title:    "Memory Leak Prevention",
		Category: "performance",
		Tags:     []string{"leak", "memory", "transport"},
		Content:  "Always close resp.Body and io.Discard",
		Vector:   []float32{1.0, 0.0, 0.0},
	})
	b.AddChunk(KnowledgeChunk{
		ID:       "chunk_reconnect",
		Title:    "WebSocket Auto-Reconnect",
		Category: "networking",
		Tags:     []string{"websocket", "reconnect", "backoff"},
		Content:  "Use exponential backoff ticker",
		Vector:   []float32{0.0, 1.0, 0.0},
	})
	b.AddChunk(KnowledgeChunk{
		ID:       "chunk_supervisor",
		Title:    "Systemd Supervisor",
		Category: "devops",
		Tags:     []string{"systemd", "restart", "vps"},
		Content:  "Set Restart=always and MemoryHigh=80M",
		Vector:   []float32{0.0, 0.0, 1.0},
	})

	if err := b.ExportBinary(filePath); err != nil {
		t.Fatalf("falha ao exportar binario: %v", err)
	}

	// 1. Direct SearchOnDemand by vector (0 MB heap)
	targetVec := []float32{0.0, 1.0, 0.0}
	results, err := SearchOnDemand(filePath, targetVec, 1)
	if err != nil {
		t.Fatalf("SearchOnDemand retornou erro: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("SearchOnDemand nao retornou resultados")
	}
	if results[0].Chunk.ID != "chunk_reconnect" {
		t.Fatalf("esperado chunk_reconnect, obteve %s", results[0].Chunk.ID)
	}
	if results[0].Score < 0.999 {
		t.Fatalf("score esperado ~1.0, obteve %f", results[0].Score)
	}
	if results[0].Chunk.Title != "WebSocket Auto-Reconnect" {
		t.Fatalf("titulo incorreto lido do disco: %s", results[0].Chunk.Title)
	}
	if len(results[0].Chunk.Tags) != 3 || results[0].Chunk.Tags[0] != "websocket" {
		t.Fatalf("tags incorretas lidas do disco: %+v", results[0].Chunk.Tags)
	}

	// 2. Integration with Brain.SetOnDemandPath
	emptyBrain := NewBrain(3)
	emptyBrain.SetOnDemandPath(filePath)

	if emptyBrain.TotalChunks() != 3 {
		t.Fatalf("TotalChunks esperado 3, obteve %d", emptyBrain.TotalChunks())
	}

	resVec := emptyBrain.SearchByVector([]float32{1.0, 0.0, 0.0}, 1)
	if len(resVec) == 0 || resVec[0].Chunk.ID != "chunk_leak" {
		t.Fatalf("esperado chunk_leak via brain on-demand, obteve %+v", resVec)
	}
}

