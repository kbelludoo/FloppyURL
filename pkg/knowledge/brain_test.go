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
