package neural

import (
	"testing"
)

func TestNeuralNetworkBasics(t *testing.T) {
	net, err := NewNetwork([]int{4, 8, 2}, []string{"relu", "sigmoid"})
	if err != nil {
		t.Fatalf("erro ao criar rede: %v", err)
	}

	inputs := []float64{0.5, -0.2, 0.8, 0.1}
	outputs := net.Predict(inputs)

	if len(outputs) != 2 {
		t.Fatalf("esperado 2 saidas, obteve %d", len(outputs))
	}

	for i, o := range outputs {
		if o < 0.0 || o > 1.0 {
			t.Fatalf("saida sigmoid deve estar em [0, 1], obteve %f na posicao %d", o, i)
		}
	}

	// Test deep clone
	cloned := net.Clone()
	cloned.Mutate(1.0, 0.5) // muta todos os pesos
	if net.Fingerprint() == cloned.Fingerprint() {
		t.Fatalf("clone mutado nao pode ter o mesmo fingerprint da original")
	}
}

func TestEvolutionSafetyGates(t *testing.T) {
	net, _ := NewNetwork([]int{2, 4, 1}, []string{"relu", "sigmoid"})

	// Golden Test Suite com teste critico
	suite := []TestCase{
		{
			ID:         "safety_invariant_1",
			Name:       "Zero inputs must not explode",
			Inputs:     []float64{0.0, 0.0},
			Expected:   []float64{0.0},
			IsCritical: true,
			MaxLoss:    0.15,
		},
		{
			ID:         "target_match_1",
			Name:       "High input gives high output",
			Inputs:     []float64{1.0, 1.0},
			Expected:   []float64{1.0},
			IsCritical: false,
		},
	}

	tmpDir := t.TempDir()
	eng := NewEvolutionEngine(net, suite, tmpDir)

	initialGen := eng.Generation
	initialFit := eng.BestFitness

	// Executa multiplos passos evolutivos
	promotions := 0
	for step := 0; step < 50; step++ {
		promoted, _, newFit, reason := eng.Step()
		if promoted {
			promotions++
			if newFit <= initialFit {
				t.Fatalf("candidata promovida com fitness inferior: %s", reason)
			}
		}
	}

	t.Logf("Evolucao segura: %d promocoes em 50 passos. Geracao atual: %d, Fitness: %.4f",
		promotions, eng.Generation, eng.BestFitness)

	if eng.Generation < initialGen {
		t.Fatalf("geracao nunca pode regredir")
	}

	// Testa Rollback
	if len(eng.Checkpoints) > 1 {
		lastGen := eng.Generation
		ok, msg := eng.Rollback()
		if !ok {
			t.Fatalf("rollback falhou: %s", msg)
		}
		if eng.Generation >= lastGen {
			t.Fatalf("rollback deve retornar para geracao anterior, obteve %d (antes: %d)", eng.Generation, lastGen)
		}
	}
}
