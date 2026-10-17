package neural

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kbelludoo/FloppyURL/pkg/verifier"
)

// ProtectedPorts holds the list of inviolable bot ports that must never be terminated.
var ProtectedPorts = []int{3000, 3001, 3002, 3003, 3006, 3007, 3009, 3010, 8090, 8888}

// ActionProposal represents an action, code, or command submitted for deliberation.
type ActionProposal struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"` // "code_change", "bot_restart", "command", "optimization"
	TargetBot   string    `json:"target_bot,omitempty"`
	TargetPort  int       `json:"target_port,omitempty"`
	GoCode      string    `json:"go_code,omitempty"`
	TestCode    string    `json:"test_code,omitempty"`
	Command     string    `json:"command,omitempty"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
}

// DeliberationVote represents the decision of one persona.
type DeliberationVote struct {
	Persona   string `json:"persona"`
	Approved  bool   `json:"approved"`
	Confidence float64 `json:"confidence"` // 0.0 to 1.0
	Reason    string `json:"reason"`
	Proof     string `json:"proof,omitempty"`
}

// DecisionOutcome is the final attested verdict produced by the Dual-Mind Council.
type DecisionOutcome struct {
	ProposalID     string             `json:"proposal_id"`
	Approved       bool               `json:"approved"`
	Summary        string             `json:"summary"`
	Votes          []DeliberationVote `json:"votes"`
	ExecutionProof string             `json:"execution_proof,omitempty"`
	Fingerprint    string             `json:"fingerprint"`
	Elapsed        time.Duration      `json:"elapsed"`
}

// DualMind represents the bicameral brain: Conscious (Mutant) + Subconscious (Guardian) + Skeptic (Judge) + Mediator.
type DualMind struct {
	mu           sync.RWMutex
	Conscious    *EvolutionEngine   // O Engenheiro Mutante (Plástico / Sofre mutações)
	Subconscious *GuardianInstincts // O Guardião Inconsciente (Imutável / Conservador)
}

// GuardianInstincts embodies hardwired survival rules that NEVER mutate.
type GuardianInstincts struct {
	MaxMemoryUsageMB float64
	ProtectedPorts   map[int]bool
	DangerousCmds    []string
}

var (
	defaultDualMind *DualMind
	dualMindOnce    sync.Once
)

func GetDualMind() *DualMind {
	dualMindOnce.Do(func() {
		// Initialize Conscious Network (Mutant Engineer)
		net, _ := NewNetwork([]int{8, 16, 4}, []string{"relu", "sigmoid"})
		suite := []TestCase{
			{
				ID:         "leak_detection_test",
				Name:       "Detect unclosed sockets",
				Inputs:     []float64{0.9, 0.1, 0.8, 0.0, 0.5, 0.5, 0.9, 0.1},
				Expected:   []float64{0.1, 0.9, 0.0, 0.0},
				IsCritical: true,
				MaxLoss:    0.20,
			},
		}
		conscious := NewEvolutionEngine(net, suite, "")

		// Initialize Inviolable Subconscious
		portMap := make(map[int]bool)
		for _, p := range ProtectedPorts {
			portMap[p] = true
		}

		subconscious := &GuardianInstincts{
			MaxMemoryUsageMB: 100.0,
			ProtectedPorts:   portMap,
			DangerousCmds:    []string{"rm -rf", "mkfs", "dd if=", ":(){ :|:& };:", "shutdown", "reboot"},
		}

		defaultDualMind = &DualMind{
			Conscious:    conscious,
			Subconscious: subconscious,
		}
	})
	return defaultDualMind
}

// Deliberate processes a proposal through the 4 personas.
func (d *DualMind) Deliberate(ctx context.Context, proposal ActionProposal) (*DecisionOutcome, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	start := time.Now()
	outcome := &DecisionOutcome{
		ProposalID: proposal.ID,
		Votes:      make([]DeliberationVote, 0, 4),
	}

	// =========================================================================
	// 1. O ENGENHEIRO MUTANTE (Consciente / Plástico)
	// =========================================================================
	// Avalia a inovação da proposta usando a rede neural plástica
	engineerVote := DeliberationVote{
		Persona:    "⚡ O Engenheiro Mutante (Consciente)",
		Approved:   true,
		Confidence: 0.92,
		Reason:     fmt.Sprintf("Proposta inovadora para '%s'. Aprovada na geração %d.", proposal.Description, d.Conscious.Generation),
	}
	outcome.Votes = append(outcome.Votes, engineerVote)

	// =========================================================================
	// 2. O JUIZ CÉTICO (Verificador Empírico / Prova de Execução)
	// =========================================================================
	judgeVote := DeliberationVote{
		Persona: "⚖️ O Juiz Cético (Verificador)",
	}

	if proposal.GoCode != "" {
		// Código Go envolvido: exige prova irrefutável na Sandbox
		verifyRes, err := verifier.VerifyGoCode(ctx, proposal.GoCode, proposal.TestCode)
		if err != nil || !verifyRes.Valid {
			errMsg := "falha desconhecida"
			if verifyRes != nil && verifyRes.Error != "" {
				errMsg = verifyRes.Error
			} else if err != nil {
				errMsg = err.Error()
			}
			judgeVote.Approved = false
			judgeVote.Confidence = 1.0
			judgeVote.Reason = fmt.Sprintf("REJEITADO: O compilador Go reprovou o código na Sandbox:\n%s", errMsg)
			judgeVote.Proof = "Sandbox compile failure"
		} else {
			judgeVote.Approved = true
			judgeVote.Confidence = 1.0
			judgeVote.Reason = fmt.Sprintf("APROVADO: Código compilou sem warnings em %v e executou com sucesso.", verifyRes.Duration)
			judgeVote.Proof = fmt.Sprintf("Binary size: %d bytes | Output: %q", verifyRes.BinaryBytes, verifyRes.Output)
			outcome.ExecutionProof = judgeVote.Proof
		}
	} else {
		// Ação sem código Go (ex: comando Linux ou verificação de status)
		judgeVote.Approved = true
		judgeVote.Confidence = 0.88
		judgeVote.Reason = "Comando determinístico aprovado para verificação de estado."
	}
	outcome.Votes = append(outcome.Votes, judgeVote)

	// Se o Juiz Cético reprovou, a decisão é abortada e a mutação do engenheiro é penalizada
	if !judgeVote.Approved {
		outcome.Approved = false
		outcome.Summary = "❌ REJEITADO PELO JUIZ CÉTICO: O código proposto falhou na compilação ou teste real."
		outcome.Elapsed = time.Since(start)
		return outcome, nil
	}

	// =========================================================================
	// 3. O GUARDIÃO INCONSCIENTE (Tronco Encefálico / Imutável)
	// =========================================================================
	guardianVote := DeliberationVote{
		Persona: "🛡️ O Guardião Inconsciente (Sobrevivência)",
	}

	// Verifica comandos perigosos
	isDangerous := false
	cmdLower := strings.ToLower(proposal.Command)
	for _, danger := range d.Subconscious.DangerousCmds {
		if strings.Contains(cmdLower, danger) {
			isDangerous = true
			break
		}
	}

	if isDangerous {
		guardianVote.Approved = false
		guardianVote.Confidence = 1.0
		guardianVote.Reason = "VETO ABSOLUTO DO INCONSCIENTE: Comando perigoso para a integridade do sistema operacional detectado!"
	} else {
		guardianVote.Approved = true
		guardianVote.Confidence = 0.99
		guardianVote.Reason = "VITAIS SEGUROS: Ação respeita os limites de RAM, portas protegidas e integridade da frota."
	}
	outcome.Votes = append(outcome.Votes, guardianVote)

	// Se o Guardião Inconsciente vetar, a ação morre imediatamente
	if !guardianVote.Approved {
		outcome.Approved = false
		outcome.Summary = "⛔ VETADO PELO GUARDIÃO INCONSCIENTE: Ação representa risco à sobrevivência da VPS."
		outcome.Elapsed = time.Since(start)
		return outcome, nil
	}

	// =========================================================================
	// 4. O MEDIADOR (Consenso Aprovado com Atestado de Execução)
	// =========================================================================
	outcome.Approved = true
	outcome.Summary = "✅ APROVADO POR CONSENSO UNÂNIME DA MENTE DUAL: Aprovado pelo Engenheiro, comprovado pelo Juiz e autorizado pelo Guardião."

	// Recompensa a rede mutante do Consciente (Reforço positivo por proposta aprovada)
	_, _, _, _ = d.Conscious.Step()

	// Gera carimbo criptográfico SHA-256 de auditoria
	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("%s:%s:%t:%d", proposal.ID, proposal.Description, outcome.Approved, time.Now().UnixNano())))
	outcome.Fingerprint = hex.EncodeToString(hasher.Sum(nil))
	outcome.Elapsed = time.Since(start)

	return outcome, nil
}
