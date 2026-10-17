package neural

import (
	"context"
	"strings"
	"testing"
)

func TestDualMindApproved(t *testing.T) {
	dm := GetDualMind()

	proposal := ActionProposal{
		ID:          "prop_01_safe",
		Type:        "code_change",
		Description: "Otimizar buffer de rede em Go",
		GoCode: `package main

import "fmt"

func main() {
	fmt.Println("BUFFER_OPTIMIZED_OK")
}
`,
	}

	outcome, err := dm.Deliberate(context.Background(), proposal)
	if err != nil {
		t.Fatalf("erro inesperado na deliberacao: %v", err)
	}

	if !outcome.Approved {
		t.Fatalf("proposta segura deveria ter sido aprovada pelo conselho, obteve: %s", outcome.Summary)
	}

	if len(outcome.Votes) != 3 {
		t.Fatalf("esperado 3 votos no conselho, obteve %d", len(outcome.Votes))
	}

	if outcome.Fingerprint == "" {
		t.Fatalf("esperado carimbo criptografico SHA-256 no veredito")
	}
}

func TestDualMindSkepticJudgeRejection(t *testing.T) {
	dm := GetDualMind()

	// Proposta com código quebrado que causa erro de compilação
	brokenProposal := ActionProposal{
		ID:          "prop_02_broken",
		Type:        "code_change",
		Description: "Código com erro de sintaxe",
		GoCode: `package main

func main() {
	invalidVariable.DoesNotExist()
}
`,
	}

	outcome, err := dm.Deliberate(context.Background(), brokenProposal)
	if err != nil {
		t.Fatalf("erro na deliberacao: %v", err)
	}

	if outcome.Approved {
		t.Fatalf("o Juiz Cético deveria ter REJEITADO o código quebrado")
	}

	if !strings.Contains(outcome.Summary, "REJEITADO PELO JUIZ CÉTICO") {
		t.Fatalf("esperava rejeicao explicita do Juiz Cético: %s", outcome.Summary)
	}
}

func TestDualMindGuardianVeto(t *testing.T) {
	dm := GetDualMind()

	// Proposta com comando destrutivo para a VPS
	dangerousProposal := ActionProposal{
		ID:          "prop_03_dangerous",
		Type:        "command",
		Description: "Limpeza agressiva de disco",
		Command:     "rm -rf /home/ubuntu/data",
	}

	outcome, err := dm.Deliberate(context.Background(), dangerousProposal)
	if err != nil {
		t.Fatalf("erro na deliberacao: %v", err)
	}

	if outcome.Approved {
		t.Fatalf("o Guardião Inconsciente deveria ter VETADO o comando perigoso")
	}

	if !strings.Contains(outcome.Summary, "VETADO PELO GUARDIÃO INCONSCIENTE") {
		t.Fatalf("esperava veto explicito do Guardião: %s", outcome.Summary)
	}
}
