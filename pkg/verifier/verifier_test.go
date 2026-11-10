package verifier

import (
	"context"
	"strings"
	"testing"
)

func TestVerifyValidGoCode(t *testing.T) {
	code := `package main

import "fmt"

func main() {
	fmt.Println("Proof of Execution: SUCCESS")
}
`
	res, err := VerifyGoCode(context.Background(), code, "")
	if err != nil {
		t.Fatalf("erro inesperado na sandbox: %v", err)
	}

	if !res.Valid || !res.CompileSuccess {
		t.Fatalf("esperado código válido, obteve erro: %s", res.Error)
	}

	if !strings.Contains(res.Output, "Proof of Execution: SUCCESS") {
		t.Fatalf("saída inesperada: %s", res.Output)
	}
}

func TestVerifyInvalidGoCode(t *testing.T) {
	brokenCode := `package main

func main() {
	undefinedVariable.Call() // Erro proposital de compilação
}
`
	res, err := VerifyGoCode(context.Background(), brokenCode, "")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if res.Valid {
		t.Fatalf("código quebrado não pode passar como válido")
	}

	if !strings.Contains(res.Error, "undefinedVariable") {
		t.Fatalf("esperava menção ao identificador indefinido no erro: %s", res.Error)
	}
}
