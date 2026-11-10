package neural

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"
)

// Layer represents a dense fully-connected neural network layer.
type Layer struct {
	Weights    [][]float64 `json:"weights"`    // [outputs][inputs]
	Biases     []float64   `json:"biases"`     // [outputs]
	Activation string      `json:"activation"` // relu, sigmoid, tanh, linear
}

// NeuralNetwork represents a multi-layer feed-forward network in pure Go.
type NeuralNetwork struct {
	mu        sync.RWMutex
	Layers    []Layer `json:"layers"`
	InputDim  int     `json:"input_dim"`
	OutputDim int     `json:"output_dim"`
}

// NewNetwork initializes a network with specified layer sizes (e.g. [16, 32, 16, 4]).
func NewNetwork(layerSizes []int, activations []string) (*NeuralNetwork, error) {
	if len(layerSizes) < 2 {
		return nil, fmt.Errorf("a rede precisa de no mínimo 2 camadas (entrada e saída)")
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	layers := make([]Layer, len(layerSizes)-1)

	for i := 0; i < len(layerSizes)-1; i++ {
		inDim := layerSizes[i]
		outDim := layerSizes[i+1]

		act := "relu"
		if i < len(activations) && activations[i] != "" {
			act = activations[i]
		} else if i == len(layerSizes)-2 {
			act = "sigmoid" // default output activation
		}

		// Xavier/He initialization
		limit := math.Sqrt(6.0 / float64(inDim+outDim))
		weights := make([][]float64, outDim)
		biases := make([]float64, outDim)

		for o := 0; o < outDim; o++ {
			weights[o] = make([]float64, inDim)
			for in := 0; in < inDim; in++ {
				weights[o][in] = (rng.Float64()*2 - 1) * limit
			}
			biases[o] = 0.0
		}

		layers[i] = Layer{
			Weights:    weights,
			Biases:     biases,
			Activation: act,
		}
	}

	return &NeuralNetwork{
		Layers:    layers,
		InputDim:  layerSizes[0],
		OutputDim: layerSizes[len(layerSizes)-1],
	}, nil
}

// Predict performs ultra-fast forward propagation (runs in < 20 microseconds).
func (n *NeuralNetwork) Predict(inputs []float64) []float64 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	current := make([]float64, len(inputs))
	copy(current, inputs)

	for _, l := range n.Layers {
		outDim := len(l.Biases)
		next := make([]float64, outDim)

		for o := 0; o < outDim; o++ {
			sum := l.Biases[o]
			wRow := l.Weights[o]
			for in := range current {
				sum += wRow[in] * current[in]
			}
			next[o] = activate(sum, l.Activation)
		}
		current = next
	}

	return current
}

func activate(x float64, kind string) float64 {
	switch kind {
	case "relu":
		if x < 0 {
			return 0
		}
		return x
	case "sigmoid":
		if x > 40 {
			return 1.0
		}
		if x < -40 {
			return 0.0
		}
		return 1.0 / (1.0 + math.Exp(-x))
	case "tanh":
		return math.Tanh(x)
	default:
		return x
	}
}

// Clone creates an exact deep copy of the neural network in memory.
func (n *NeuralNetwork) Clone() *NeuralNetwork {
	n.mu.RLock()
	defer n.mu.RUnlock()

	clonedLayers := make([]Layer, len(n.Layers))
	for i, l := range n.Layers {
		outDim := len(l.Biases)
		inDim := len(l.Weights[0])

		wCopy := make([][]float64, outDim)
		bCopy := make([]float64, outDim)
		copy(bCopy, l.Biases)

		for o := 0; o < outDim; o++ {
			wCopy[o] = make([]float64, inDim)
			copy(wCopy[o], l.Weights[o])
		}

		clonedLayers[i] = Layer{
			Weights:    wCopy,
			Biases:     bCopy,
			Activation: l.Activation,
		}
	}

	return &NeuralNetwork{
		Layers:    clonedLayers,
		InputDim:  n.InputDim,
		OutputDim: n.OutputDim,
	}
}

// Mutate applies bounded random Gaussian/Uniform mutations to weights and biases.
func (n *NeuralNetwork) Mutate(rate, magnitude float64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for lIdx := range n.Layers {
		l := &n.Layers[lIdx]
		for o := range l.Biases {
			if rng.Float64() < rate {
				delta := (rng.Float64()*2 - 1) * magnitude
				l.Biases[o] = clip(l.Biases[o]+delta, -3.0, 3.0)
			}
			for in := range l.Weights[o] {
				if rng.Float64() < rate {
					delta := (rng.Float64()*2 - 1) * magnitude
					l.Weights[o][in] = clip(l.Weights[o][in]+delta, -3.0, 3.0)
				}
			}
		}
	}
}

// Clip ensures all weights stay within strict mathematical safety bounds [-3.0, 3.0].
func clip(val, min, max float64) float64 {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0.0
	}
	if val > max {
		return max
	}
	if val < min {
		return min
	}
	return val
}

// Fingerprint returns the cryptographic SHA-256 hash of all network weights.
func (n *NeuralNetwork) Fingerprint() string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	data, _ := json.Marshal(n.Layers)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Save writes the network weights to a JSON file.
func (n *NeuralNetwork) Save(filePath string) error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	data, err := json.MarshalIndent(n, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// LoadNetwork reads a network from a JSON file.
func LoadNetwork(filePath string) (*NeuralNetwork, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var net NeuralNetwork
	if err := json.Unmarshal(data, &net); err != nil {
		return nil, err
	}
	return &net, nil
}
