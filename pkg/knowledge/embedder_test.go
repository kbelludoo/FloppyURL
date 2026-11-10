package knowledge

import (
	"testing"
)

func TestSemanticEmbedder(t *testing.T) {
	emb := NewSemanticEmbedder(384)

	// Test 1: Dimension
	if emb.Dimension() != 384 {
		t.Fatalf("expected dim 384, got %d", emb.Dimension())
	}

	// Test 2: Normalization
	v1 := emb.Embed("tibia bot tcp socket keepalive")
	if len(v1) != 384 {
		t.Fatalf("expected vector len 384, got %d", len(v1))
	}

	norm := 0.0
	for _, val := range v1 {
		norm += float64(val * val)
	}
	// L2 norm of non-empty vector should be approximately 1.0
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("expected unit norm ~1.0, got %f", norm)
	}

	// Test 3: Semantic similarity between related texts
	v2 := emb.Embed("tibia ot tcp connection latency")
	v3 := emb.Embed("bolo de cenoura com chocolate receita")

	simRelated := CosineSimilarity(v1, v2)
	simUnrelated := CosineSimilarity(v1, v3)

	if simRelated <= simUnrelated {
		t.Errorf("expected related similarity (%f) > unrelated similarity (%f)", simRelated, simUnrelated)
	}

	// Test 4: Empty string handling
	vEmpty := emb.Embed("")
	if len(vEmpty) != 384 {
		t.Errorf("expected 384 elements for empty input, got %d", len(vEmpty))
	}
}
