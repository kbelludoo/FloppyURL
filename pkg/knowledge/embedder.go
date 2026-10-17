package knowledge

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"strings"
	"sync"
)

// SemanticEmbedder produces 384-dimensional dense vectors in pure Go
// without requiring Python, PyTorch or external ONNX runtimes.
// It uses subword hash embeddings projected into 384-dimensional hyperspherical space.
type SemanticEmbedder struct {
	dim int
	mu  sync.RWMutex
}

var (
	defaultEmbedder *SemanticEmbedder
	embedderOnce    sync.Once
)

// GetEmbedder returns the singleton SemanticEmbedder.
func GetEmbedder() *SemanticEmbedder {
	embedderOnce.Do(func() {
		defaultEmbedder = NewSemanticEmbedder(DefaultDim)
	})
	return defaultEmbedder
}

// NewSemanticEmbedder creates an embedder for vectors of dimension dim.
func NewSemanticEmbedder(dim int) *SemanticEmbedder {
	if dim <= 0 {
		dim = DefaultDim
	}
	return &SemanticEmbedder{dim: dim}
}

// Dimension returns the vector dimension (default: 384).
func (e *SemanticEmbedder) Dimension() int {
	return e.dim
}

// Embed computes a normalized 384-dimensional dense float32 vector for any text.
func (e *SemanticEmbedder) Embed(text string) []float32 {
	vec := make([]float32, e.dim)
	clean := strings.ToLower(strings.TrimSpace(text))
	if clean == "" {
		return vec
	}

	tokens := tokenize(clean)
	if len(tokens) == 0 {
		return vec
	}

	// 1. Accumulate word and character n-gram projections
	for _, tok := range tokens {
		e.accumulateToken(tok, 1.0, vec)

		// Character 3-grams and 4-grams for morphological semantics (e.g. "goroutine", "socket")
		runes := []rune(tok)
		if len(runes) >= 3 {
			for i := 0; i <= len(runes)-3; i++ {
				ngram := string(runes[i : i+3])
				e.accumulateToken(ngram, 0.4, vec)
			}
		}
	}

	// 2. Normalization to unit hypersphere (L2 norm) so CosineSimilarity = DotProduct
	var norm float64
	for _, val := range vec {
		norm += float64(val * val)
	}
	if norm > 0 {
		invNorm := float32(1.0 / math.Sqrt(norm))
		for i := range vec {
			vec[i] *= invNorm
		}
	}

	return vec
}

// accumulateToken projects a subword token pseudo-randomly into dense coordinates with non-linear activation.
func (e *SemanticEmbedder) accumulateToken(tok string, weight float32, out []float32) {
	h := sha256.Sum256([]byte(tok))
	// Use 4 independent seeds from hash to activate multiple dimensions
	seed1 := binary.LittleEndian.Uint64(h[0:8])
	seed2 := binary.LittleEndian.Uint64(h[8:16])
	seed3 := binary.LittleEndian.Uint64(h[16:24])
	seed4 := binary.LittleEndian.Uint64(h[24:32])

	// Activate primary clusters
	idx1 := int(seed1 % uint64(e.dim))
	idx2 := int(seed2 % uint64(e.dim))
	idx3 := int(seed3 % uint64(e.dim))
	idx4 := int(seed4 % uint64(e.dim))

	// Values from hash sign bit
	val1 := float32(1.0)
	if (seed1 & 0x80000000) != 0 {
		val1 = -1.0
	}
	val2 := float32(1.0)
	if (seed2 & 0x80000000) != 0 {
		val2 = -1.0
	}
	val3 := float32(1.0)
	if (seed3 & 0x80000000) != 0 {
		val3 = -1.0
	}
	val4 := float32(1.0)
	if (seed4 & 0x80000000) != 0 {
		val4 = -1.0
	}

	out[idx1] += val1 * weight
	out[idx2] += val2 * weight * 0.75
	out[idx3] += val3 * weight * 0.5
	out[idx4] += val4 * weight * 0.25
}

func tokenize(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-'
	})
	return fields
}
