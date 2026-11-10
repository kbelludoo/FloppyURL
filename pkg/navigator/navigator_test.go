package navigator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSnapshotAndSession(t *testing.T) {
	// Create mock server
	htmlContent := `<!DOCTYPE html>
<html>
<head><title>Página de Teste</title></head>
<body>
	<h1>Bem-vindo ao Teste</h1>
	<p>Este é um teste de navegação web ultraleve em Go.</p>
	<form id="searchForm" action="/search" method="GET">
		<input type="text" name="q" placeholder="Buscar no site..." id="searchInput">
		<button type="submit" id="btnSearch">Pesquisar</button>
	</form>
	<div class="links">
		<a href="/artigo-golang" id="link1">Aprenda Golang</a>
		<a href="/sobre" id="link2">Sobre Nós</a>
	</div>
</body>
</html>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			q := r.URL.Query().Get("q")
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!DOCTYPE html><html><head><title>Resultados de Busca</title></head><body><h2>Resultados para: ` + q + `</h2><a href="/detalhe">Item Encontrado</a></body></html>`))
			return
		}
		if r.URL.Path == "/artigo-golang" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!DOCTYPE html><html><head><title>Artigo Golang</title></head><body><h2>Golang é incrível</h2><p>Concorrência nativa com goroutines.</p></body></html>`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(htmlContent))
	}))
	defer ts.Close()

	sess := NewSession()
	ctx := context.Background()

	// 1. Navigate to test server
	snap, err := sess.Navigate(ctx, ts.URL)
	if err != nil {
		t.Fatalf("navigate failed: %v", err)
	}

	if snap.Title != "Página de Teste" {
		t.Errorf("expected title 'Página de Teste', got %q", snap.Title)
	}

	if len(snap.Elements) < 4 {
		t.Fatalf("expected at least 4 interactive elements, got %d", len(snap.Elements))
	}

	t.Logf("Snapshot rendered:\n%s", snap.RenderLLMSnapshot(20))

	// Find the input field index
	var inputIdx, submitIdx, linkIdx int
	for _, el := range snap.Elements {
		if el.Tag == "input" && el.Name == "q" {
			inputIdx = el.Index
		}
		if el.Tag == "button" && el.Type == "submit" {
			submitIdx = el.Index
		}
		if el.Tag == "a" && strings.Contains(el.Text, "Aprenda Golang") {
			linkIdx = el.Index
		}
	}

	if inputIdx == 0 || submitIdx == 0 || linkIdx == 0 {
		t.Fatalf("could not find all required elements: input=%d, submit=%d, link=%d", inputIdx, submitIdx, linkIdx)
	}

	// 2. Type into input
	typeMsg, err := sess.Type(inputIdx, "Go Concorrência")
	if err != nil {
		t.Fatalf("type failed: %v", err)
	}
	t.Logf("Type result: %s", typeMsg)

	// 3. Click submit
	newSnap, clickMsg, err := sess.Click(ctx, submitIdx)
	if err != nil {
		t.Fatalf("click submit failed: %v", err)
	}
	t.Logf("Click submit result: %s, new title: %s", clickMsg, newSnap.Title)

	if newSnap.Title != "Resultados de Busca" {
		t.Errorf("expected title 'Resultados de Busca', got %q", newSnap.Title)
	}

	// 4. Test Autonomous Goal Execution
	navigator := NewAutonomousNavigator()
	stepCount := 0

	mockLLM := func(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
		stepCount++
		if stepCount == 1 {
			return "PENSAMENTO: Estou na página inicial, vejo o link de Golang no elemento #3.\nAÇÃO: CLICK 3", nil
		}
		return "PENSAMENTO: Cheguei no artigo sobre Golang e encontrei o texto sobre concorrência nativa.\nAÇÃO: COMPLETE O artigo afirma que Golang tem concorrência nativa com goroutines.", nil
	}

	res, err := navigator.Run(ctx, ts.URL, "Descobrir o que o artigo de Golang diz", mockLLM)
	if err != nil {
		t.Fatalf("navigator.Run failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected navigation success, got false")
	}

	t.Logf("Autonomous Navigation Completed!\nSteps: %d\nFinal Answer: %s\nTrajectory:\n%s",
		res.StepsTaken, res.FinalAnswer, strings.Join(res.Trajectory, "\n"))
}
