package navigator

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LLMCompleter is an abstraction over the agent's LLM completion function.
type LLMCompleter func(ctx context.Context, systemPrompt, userPrompt string) (string, error)

// NavigationResult contains the outcome of an autonomous navigation mission.
type NavigationResult struct {
	Success     bool     `json:"success"`
	Goal        string   `json:"goal"`
	FinalAnswer string   `json:"final_answer"`
	StepsTaken  int      `json:"steps_taken"`
	FinalURL    string   `json:"final_url"`
	Trajectory  []string `json:"trajectory"`
	DurationMs  int64    `json:"duration_ms"`
}

// AutonomousNavigator drives the browser session towards achieving a goal using an LLM.
type AutonomousNavigator struct {
	Session  *Session
	MaxSteps int
}

// NewAutonomousNavigator creates a navigator agent instance.
func NewAutonomousNavigator() *AutonomousNavigator {
	return &AutonomousNavigator{
		Session:  NewSession(),
		MaxSteps: 12,
	}
}

const navigatorSystemPrompt = `Você é o AGENTE ESPECIALISTA EM NAVEGAÇÃO WEB do AI Go Browser.
Sua missão é atingir o objetivo solicitado pelo usuário navegando autonomamente por páginas web.

Você tem à sua disposição uma visualização semântica da página com elementos interativos numerados:
- Links: [#ID] LINK: "Texto" -> URL
- Botões: [#ID] BOTÃO (type): "Texto"
- Campos: [#ID] CAMPO (tag: name) (placeholder) [valor atual]

REGRAS DE AÇÃO:
Para agir, você DEVE responder EXATAMENTE no seguinte formato:

PENSAMENTO: <Seu raciocínio sobre o que vê na página e qual o próximo passo lógico>
AÇÃO: <COMANDO>

Comandos permitidos:
- AÇÃO: NAVIGATE <url>         (abre uma nova URL)
- AÇÃO: CLICK <id_numerico>     (ex: AÇÃO: CLICK 3  -- clica no link ou botão #3)
- AÇÃO: TYPE <id_numerico> <texto> (ex: AÇÃO: TYPE 2 Golang concorrência  -- digita no campo #2)
- AÇÃO: EXTRACT <termo>        (busca trecho ou dados específicos na página)
- AÇÃO: COMPLETE <resposta>    (quando atingir o objetivo com sucesso, traga a resposta final)
- AÇÃO: FAILED <motivo>        (se for comprovadamente impossível cumprir o objetivo)

DICAS DE NAVEGADOR ESPECIALISTA:
1. Para pesquisar em um formulário: primeiro use TYPE no campo de pesquisa, depois use CLICK no botão de envio.
2. Seja cirúrgico: use os IDs numéricos exatos fornecidos na lista de elementos.
3. Se a página contiver a resposta desejada, use AÇÃO: COMPLETE com as informações detalhadas.`

var (
	reAction = regexp.MustCompile(`(?i)AÇÃO:\s*([A-Z_]+)(?:\s+(.*))?`)
)

// Run executes the autonomous navigation loop until the goal is achieved or steps exhaust.
func (n *AutonomousNavigator) Run(ctx context.Context, startURL, goal string, completeFn LLMCompleter) (*NavigationResult, error) {
	start := time.Now()
	res := &NavigationResult{
		Goal:       goal,
		Trajectory: []string{},
	}

	// 1. Initial navigation if starting URL provided
	if startURL != "" {
		res.Trajectory = append(res.Trajectory, fmt.Sprintf("Iniciando em: %s", startURL))
		_, err := n.Session.Navigate(ctx, startURL)
		if err != nil {
			res.Trajectory = append(res.Trajectory, fmt.Sprintf("Erro ao abrir URL inicial: %v", err))
		}
	}

	for step := 1; step <= n.MaxSteps; step++ {
		res.StepsTaken = step

		// Check if a page is currently loaded
		var snapshotStr string
		if n.Session.Current != nil {
			res.FinalURL = n.Session.Current.URL
			snapshotStr = n.Session.Current.RenderLLMSnapshot(35)
		} else {
			snapshotStr = "(Nenhuma página carregada ainda. A primeira ação deve ser NAVIGATE para uma URL)."
		}

		userPrompt := fmt.Sprintf(`OBJETIVO: %s

HISTÓRICO RECENTE:
%s

ESTADO DA PÁGINA (Passo %d/%d):
%s

Decida o próximo passo:`, goal, formatRecentHistory(res.Trajectory), step, n.MaxSteps, snapshotStr)

		llmResp, err := completeFn(ctx, navigatorSystemPrompt, userPrompt)
		if err != nil {
			return nil, fmt.Errorf("falha ao consultar LLM no passo %d: %w", step, err)
		}

		llmResp = strings.TrimSpace(llmResp)
		res.Trajectory = append(res.Trajectory, fmt.Sprintf("[Passo %d]\n%s", step, llmResp))

		// Parse action
		matches := reAction.FindStringSubmatch(llmResp)
		if len(matches) < 2 {
			res.Trajectory = append(res.Trajectory, "Aviso: Não foi detectado comando no formato 'AÇÃO: <COMANDO>'. Tentando reorientar.")
			continue
		}

		actionType := strings.ToUpper(strings.TrimSpace(matches[1]))
		actionArg := ""
		if len(matches) > 2 {
			actionArg = strings.TrimSpace(matches[2])
		}

		switch actionType {
		case "NAVIGATE":
			dest := actionArg
			_, err := n.Session.Navigate(ctx, dest)
			if err != nil {
				res.Trajectory = append(res.Trajectory, fmt.Sprintf("Erro ao navegar: %v", err))
			} else {
				res.Trajectory = append(res.Trajectory, fmt.Sprintf("Observação: Navegou com sucesso para %s", dest))
			}

		case "CLICK":
			idx, err := strconv.Atoi(actionArg)
			if err != nil {
				// Strip '#' if present
				actionArgClean := strings.TrimPrefix(actionArg, "#")
				idx, err = strconv.Atoi(actionArgClean)
			}
			if err != nil {
				res.Trajectory = append(res.Trajectory, fmt.Sprintf("Erro: ID inválido para CLICK: %q", actionArg))
				continue
			}

			_, msg, err := n.Session.Click(ctx, idx)
			if err != nil {
				res.Trajectory = append(res.Trajectory, fmt.Sprintf("Erro ao clicar #%d: %v", idx, err))
			} else {
				res.Trajectory = append(res.Trajectory, fmt.Sprintf("Observação: %s", msg))
			}

		case "TYPE":
			parts := strings.SplitN(actionArg, " ", 2)
			if len(parts) < 2 {
				res.Trajectory = append(res.Trajectory, "Erro: TYPE exige <id> <texto>")
				continue
			}
			idxClean := strings.TrimPrefix(parts[0], "#")
			idx, err := strconv.Atoi(idxClean)
			if err != nil {
				res.Trajectory = append(res.Trajectory, fmt.Sprintf("Erro: ID inválido para TYPE: %q", parts[0]))
				continue
			}
			text := parts[1]
			msg, err := n.Session.Type(idx, text)
			if err != nil {
				res.Trajectory = append(res.Trajectory, fmt.Sprintf("Erro ao digitar em #%d: %v", idx, err))
			} else {
				res.Trajectory = append(res.Trajectory, fmt.Sprintf("Observação: %s", msg))
			}

		case "EXTRACT":
			data := n.Session.Extract(actionArg)
			res.Trajectory = append(res.Trajectory, fmt.Sprintf("Observação da extração (%s):\n%s", actionArg, data))

		case "COMPLETE":
			res.Success = true
			res.FinalAnswer = actionArg
			res.DurationMs = time.Since(start).Milliseconds()
			return res, nil

		case "FAILED":
			res.Success = false
			res.FinalAnswer = actionArg
			res.DurationMs = time.Since(start).Milliseconds()
			return res, nil

		default:
			res.Trajectory = append(res.Trajectory, fmt.Sprintf("Comando desconhecido: %s", actionType))
		}
	}

	res.Success = false
	res.FinalAnswer = "Limite máximo de passos atingido antes de concluir o objetivo."
	res.DurationMs = time.Since(start).Milliseconds()
	return res, nil
}

func formatRecentHistory(traj []string) string {
	if len(traj) == 0 {
		return "(Início da navegação)"
	}
	start := 0
	if len(traj) > 6 {
		start = len(traj) - 6
	}
	return strings.Join(traj[start:], "\n---\n")
}
