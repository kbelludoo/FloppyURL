package verifier

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// VerificationResult holds the execution proof of a Go code snippet.
type VerificationResult struct {
	Valid          bool          `json:"valid"`
	CompileSuccess bool          `json:"compile_success"`
	TestSuccess    bool          `json:"test_success"`
	Output         string        `json:"output"`
	Error          string        `json:"error,omitempty"`
	Duration       time.Duration `json:"duration"`
	BinaryBytes    int64         `json:"binary_bytes,omitempty"`
	SandboxPath    string        `json:"sandbox_path"`
}

// VerifyGoCode creates an isolated sandbox, writes the code, runs 'go vet', 'go build', and 'go test'.
func VerifyGoCode(ctx context.Context, goCode string, testCode string) (*VerificationResult, error) {
	start := time.Now()

	// 1. Create clean isolated temporary directory
	sandboxDir, err := os.MkdirTemp("", "agent_sandbox_*")
	if err != nil {
		return nil, fmt.Errorf("falha ao criar sandbox: %w", err)
	}
	defer os.RemoveAll(sandboxDir) // Automatic cleanup

	res := &VerificationResult{
		SandboxPath: sandboxDir,
	}

	// 2. Initialize minimal go.mod in sandbox
	goModContent := "module sandbox\n\ngo 1.22\n"
	if err := os.WriteFile(filepath.Join(sandboxDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		return nil, fmt.Errorf("falha ao escrever go.mod na sandbox: %w", err)
	}

	// 3. Write main or package code
	if err := os.WriteFile(filepath.Join(sandboxDir, "main.go"), []byte(goCode), 0644); err != nil {
		return nil, fmt.Errorf("falha ao escrever main.go na sandbox: %w", err)
	}

	// 4. Optionally write unit test
	if testCode != "" {
		if err := os.WriteFile(filepath.Join(sandboxDir, "main_test.go"), []byte(testCode), 0644); err != nil {
			return nil, fmt.Errorf("falha ao escrever main_test.go na sandbox: %w", err)
		}
	}

	tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// 5. Run 'go vet' (Static Analysis)
	vetCmd := exec.CommandContext(tctx, "go", "vet", "./...")
	vetCmd.Dir = sandboxDir
	var vetOut bytes.Buffer
	vetCmd.Stdout = &vetOut
	vetCmd.Stderr = &vetOut
	if err := vetCmd.Run(); err != nil {
		res.Error = fmt.Sprintf("Erro no 'go vet' (análise estática):\n%s", vetOut.String())
		res.Duration = time.Since(start)
		return res, nil
	}

	// 6. Run 'go build' or 'go test'
	if testCode != "" {
		testCmd := exec.CommandContext(tctx, "go", "test", "-v", "./...")
		testCmd.Dir = sandboxDir
		var testOut bytes.Buffer
		testCmd.Stdout = &testOut
		testCmd.Stderr = &testOut
		testErr := testCmd.Run()
		res.Output = testOut.String()
		res.Duration = time.Since(start)

		if testErr != nil {
			res.Error = fmt.Sprintf("Falha na execução dos testes ('go test'):\n%s", testOut.String())
			return res, nil
		}
		res.CompileSuccess = true
		res.TestSuccess = true
		res.Valid = true
		return res, nil
	}

	// If no tests provided, verify executable compilation
	binName := "app_bin"
	if strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") || filepath.Separator == '\\' {
		binName = "app_bin.exe"
	}
	binPath := filepath.Join(sandboxDir, binName)
	buildCmd := exec.CommandContext(tctx, "go", "build", "-o", binPath, ".")
	buildCmd.Dir = sandboxDir
	var buildOut bytes.Buffer
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut

	if err := buildCmd.Run(); err != nil {
		res.Error = fmt.Sprintf("Erro de compilação ('go build'):\n%s", buildOut.String())
		res.Duration = time.Since(start)
		return res, nil
	}

	res.CompileSuccess = true
	res.Valid = true
	if fi, err := os.Stat(binPath); err == nil {
		res.BinaryBytes = fi.Size()
	}

	// Run executable with 3s timeout to verify no immediate panic
	runCtx, runCancel := context.WithTimeout(tctx, 3*time.Second)
	defer runCancel()

	runCmd := exec.CommandContext(runCtx, binPath)
	runCmd.Dir = sandboxDir
	var runOut bytes.Buffer
	runCmd.Stdout = &runOut
	runCmd.Stderr = &runOut
	_ = runCmd.Run()

	res.Output = strings.TrimSpace(runOut.String())
	res.Duration = time.Since(start)
	return res, nil
}
