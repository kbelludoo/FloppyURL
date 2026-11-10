package neural

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TestCase defines a regression test with ground truth expectations.
type TestCase struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Inputs      []float64 `json:"inputs"`
	Expected    []float64 `json:"expected"`
	IsCritical  bool      `json:"is_critical"` // If true, ANY divergence > maxLoss immediately disqualifies candidate
	MaxLoss     float64   `json:"max_loss"`    // Maximum acceptable loss for this test
}

// EvolutionRecord logs an evolution event.
type EvolutionRecord struct {
	Timestamp   time.Time `json:"timestamp"`
	Generation  int       `json:"generation"`
	Promoted    bool      `json:"promoted"`
	OldFitness  float64   `json:"old_fitness"`
	NewFitness  float64   `json:"new_fitness"`
	Fingerprint string    `json:"fingerprint"`
	Reason      string    `json:"reason"`
}

// Checkpoint stores a verified champion state for atomic rollbacks.
type Checkpoint struct {
	Generation  int            `json:"generation"`
	Fitness     float64        `json:"fitness"`
	Fingerprint string         `json:"fingerprint"`
	Network     *NeuralNetwork `json:"-"`
	Timestamp   time.Time      `json:"timestamp"`
}

// EvolutionEngine manages continuous safe neuroevolution.
type EvolutionEngine struct {
	mu           sync.RWMutex
	Champion     *NeuralNetwork    `json:"-"`
	Generation   int               `json:"generation"`
	BestFitness  float64           `json:"best_fitness"`
	MutationRate float64           `json:"mutation_rate"` // e.g. 0.08 (8% of weights mutated)
	Magnitude    float64           `json:"magnitude"`     // e.g. 0.15 (mutation step size)
	Suite        []TestCase        `json:"suite"`
	Checkpoints  []Checkpoint      `json:"checkpoints"`
	History      []EvolutionRecord `json:"history"`
	storageDir   string
}

// NewEvolutionEngine creates a new safe evolution manager.
func NewEvolutionEngine(initialNet *NeuralNetwork, suite []TestCase, storageDir string) *EvolutionEngine {
	if storageDir == "" {
		home, _ := os.UserHomeDir()
		storageDir = filepath.Join(home, ".agent-go", "neural")
	}
	_ = os.MkdirAll(storageDir, 0755)

	eng := &EvolutionEngine{
		Champion:     initialNet,
		Generation:   1,
		MutationRate: 0.08,
		Magnitude:    0.15,
		Suite:        suite,
		Checkpoints:  make([]Checkpoint, 0, 10),
		History:      make([]EvolutionRecord, 0, 50),
		storageDir:   storageDir,
	}

	// Calculate initial champion fitness
	fit, _ := eng.Evaluate(initialNet)
	eng.BestFitness = fit
	eng.saveCurrentCheckpoint("Gênesis inicial")
	return eng
}

// Evaluate computes fitness across test cases and checks critical safety invariants.
func (e *EvolutionEngine) Evaluate(net *NeuralNetwork) (fitness float64, passedCritical bool) {
	if len(e.Suite) == 0 {
		return 1.0, true
	}

	totalLoss := 0.0
	passedCritical = true

	for _, tc := range e.Suite {
		preds := net.Predict(tc.Inputs)
		caseLoss := 0.0

		for i := range preds {
			exp := 0.0
			if i < len(tc.Expected) {
				exp = tc.Expected[i]
			}
			diff := preds[i] - exp
			caseLoss += diff * diff
		}
		caseLoss = caseLoss / float64(len(preds))
		totalLoss += caseLoss

		maxAllowed := tc.MaxLoss
		if maxAllowed <= 0 {
			maxAllowed = 0.25 // Default strict limit for critical tests
		}

		if tc.IsCritical && caseLoss > maxAllowed {
			passedCritical = false
		}
	}

	mse := totalLoss / float64(len(e.Suite))
	fitness = 1.0 / (1.0 + mse) // Fitness between 0.0 and 1.0 (higher is better)
	return fitness, passedCritical
}

// Step performs one safe evolutionary trial: clone -> mutate -> evaluate -> promote or discard.
func (e *EvolutionEngine) Step() (promoted bool, oldFit, newFit float64, reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	oldFit = e.BestFitness

	// 1. Clone Champion into Challenger
	challenger := e.Champion.Clone()

	// 2. Apply bounded stochastic mutation
	challenger.Mutate(e.MutationRate, e.Magnitude)

	// 3. Strictly evaluate Challenger against Golden Test Suite
	fit, passedCritical := e.Evaluate(challenger)
	newFit = fit

	// 4. Zero-Regression Gate:
	// A. MUST pass all critical safety tests (zero failure tolerance).
	if !passedCritical {
		reason = "Descartada: violou teste crítico de regressão (segurança falhou)"
		e.record(false, oldFit, newFit, challenger.Fingerprint(), reason)
		return false, oldFit, newFit, reason
	}

	// B. MUST strictly outperform Champion (fitness superior)
	if fit <= oldFit {
		reason = fmt.Sprintf("Descartada: aptidão inferior ou igual (%.4f <= %.4f)", fit, oldFit)
		e.record(false, oldFit, newFit, challenger.Fingerprint(), reason)
		return false, oldFit, newFit, reason
	}

	// 5. Promotion: Candidate won fair and square!
	e.Generation++
	e.BestFitness = fit
	e.Champion = challenger

	reason = fmt.Sprintf("PROMOVIDA! Nova Campeã (Geração %d, Aptidão: %.4f -> %.4f)", e.Generation, oldFit, newFit)
	e.record(true, oldFit, newFit, challenger.Fingerprint(), reason)
	e.saveCurrentCheckpoint(reason)

	return true, oldFit, newFit, reason
}

// Rollback restores the previous stable champion from checkpoint stack.
func (e *EvolutionEngine) Rollback() (bool, string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.Checkpoints) <= 1 {
		return false, "Nenhum checkpoint anterior disponível para rollback"
	}

	// Pop current and restore previous
	prev := e.Checkpoints[len(e.Checkpoints)-2]
	e.Checkpoints = e.Checkpoints[:len(e.Checkpoints)-1]

	e.Champion = prev.Network
	e.BestFitness = prev.Fitness
	e.Generation = prev.Generation

	msg := fmt.Sprintf("Rollback concluído! Restaurado para Geração %d (SHA-256: %s)", prev.Generation, prev.Fingerprint[:8])
	return true, msg
}

func (e *EvolutionEngine) saveCurrentCheckpoint(reason string) {
	cp := Checkpoint{
		Generation:  e.Generation,
		Fitness:     e.BestFitness,
		Fingerprint: e.Champion.Fingerprint(),
		Network:     e.Champion.Clone(),
		Timestamp:   time.Now(),
	}
	e.Checkpoints = append(e.Checkpoints, cp)
	if len(e.Checkpoints) > 10 {
		e.Checkpoints = e.Checkpoints[len(e.Checkpoints)-10:]
	}

	// Persist champion weights
	if e.storageDir != "" {
		filePath := filepath.Join(e.storageDir, "champion_weights.json")
		_ = e.Champion.Save(filePath)
	}
}

func (e *EvolutionEngine) record(promoted bool, oldFit, newFit float64, fp, reason string) {
	rec := EvolutionRecord{
		Timestamp:   time.Now(),
		Generation:  e.Generation,
		Promoted:    promoted,
		OldFitness:  oldFit,
		NewFitness:  newFit,
		Fingerprint: fp,
		Reason:      reason,
	}
	e.History = append(e.History, rec)
	if len(e.History) > 100 {
		e.History = e.History[len(e.History)-100:]
	}
}

// GetStatus returns the live operational metrics of the evolution engine.
func (e *EvolutionEngine) GetStatus() map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return map[string]any{
		"generation":        e.Generation,
		"best_fitness":      math.Round(e.BestFitness*10000) / 10000,
		"mutation_rate":     e.MutationRate,
		"checkpoints_count": len(e.Checkpoints),
		"fingerprint":       e.Champion.Fingerprint(),
		"total_history":     len(e.History),
	}
}
