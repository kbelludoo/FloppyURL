package linp

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinpCompileAndExecute(t *testing.T) {
	src := []byte(`@LINP:1.0
JOB demo
FILE examples/demo.html
ALGO deflate
CHUNK 1800000
PACK
END`)

	bc1, plan1, err := Compile(src)
	if err != nil {
		t.Fatalf("Compile falhou: %v", err)
	}

	bc2, _, err := Compile(src)
	if err != nil {
		t.Fatalf("Compile 2 falhou: %v", err)
	}

	// 1. Determinismo
	if !bytes.Equal(bc1, bc2) {
		t.Fatalf("bytecode LINP nao e deterministico")
	}

	// 2. Cabecalho
	if len(bc1) < 13 {
		t.Fatalf("bytecode muito curto: %d bytes", len(bc1))
	}
	if string(bc1[:4]) != Magic {
		t.Fatalf("magic invalido: %s", string(bc1[:4]))
	}
	if bc1[4] != Version {
		t.Fatalf("versao invalida: %d", bc1[4])
	}
	if plan1.Job != "demo" || plan1.File != "examples/demo.html" || plan1.Algo != "deflate" {
		t.Fatalf("plano incorreto: %+v", plan1)
	}

	// 3. Execucao
	baseRepo := ".."
	if _, err := os.Stat(filepath.Join(baseRepo, "examples", "demo.html")); err != nil {
		baseRepo = "."
	}

	outDir := t.TempDir()
	_, err = Run(plan1, bc1, src, baseRepo, outDir)
	if err != nil {
		t.Fatalf("Run falhou: %v", err)
	}

	// 4. Validacao do disco gerado
	diskPath := filepath.Join(outDir, "disk_01.txt")
	diskBytes, err := os.ReadFile(diskPath)
	if err != nil {
		t.Fatalf("ler disk_01.txt: %v", err)
	}

	parts := strings.SplitN(strings.TrimSpace(string(diskBytes)), ";", 6)
	if len(parts) != 6 {
		t.Fatalf("formato v2 invalido: %s", string(diskBytes))
	}
	h, algo, frac, chunkSha, rootSha, payload := parts[0], parts[1], parts[2], parts[3], parts[4], parts[5]

	if h != "v2" || algo != "deflate" || frac != "[1/1]" {
		t.Fatalf("cabecalho invalido: %v %v %v", h, algo, frac)
	}

	cpHash := sha256.Sum256([]byte(payload))
	if hex.EncodeToString(cpHash[:]) != chunkSha {
		t.Fatalf("chunk SHA mismatch")
	}
	if chunkSha != rootSha {
		t.Fatalf("root SHA mismatch")
	}

	// 5. Roundtrip de bytes crus
	rawOrig, err := os.ReadFile(filepath.Join(baseRepo, "examples", "demo.html"))
	if err != nil {
		t.Fatalf("ler demo.html: %v", err)
	}

	decBytes, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("b64 decode: %v", err)
	}

	flateReader := flate.NewReader(bytes.NewReader(decBytes))
	defer flateReader.Close()
	decompressed, err := io.ReadAll(flateReader)
	if err != nil {
		t.Fatalf("inflate falhou: %v", err)
	}

	if !bytes.Equal(decompressed, rawOrig) {
		t.Fatalf("roundtrip LINP: descomprimido (%d B) != original (%d B)", len(decompressed), len(rawOrig))
	}

	// 6. Validacao do receipt
	receiptBytes, err := os.ReadFile(filepath.Join(outDir, "receipt.json"))
	if err != nil {
		t.Fatalf("ler receipt.json: %v", err)
	}
	var receipt map[string]interface{}
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatalf("unmarshal receipt: %v", err)
	}

	srcSha := sha256.Sum256(src)
	if receipt["source_sha256"] != hex.EncodeToString(srcSha[:]) {
		t.Fatalf("receipt source_sha256 mismatch")
	}
	bcSha := sha256.Sum256(bc1)
	if receipt["bytecode_sha256"] != hex.EncodeToString(bcSha[:]) {
		t.Fatalf("receipt bytecode_sha256 mismatch")
	}
	if receipt["root_sha256"] != rootSha {
		t.Fatalf("receipt root_sha256 mismatch")
	}
}

func TestLinpFailClosed(t *testing.T) {
	badInputs := []struct {
		name string
		src  string
	}{
		{"algo desconhecido", "@LINP:1.0\nJOB x\nFILE a\nALGO lzma\nCHUNK 100\nPACK\nEND"},
		{"chunk muito pequeno", "@LINP:1.0\nJOB x\nFILE a\nALGO deflate\nCHUNK 10\nPACK\nEND"},
		{"chunk muito grande", "@LINP:1.0\nJOB x\nFILE a\nALGO deflate\nCHUNK 99999999\nPACK\nEND"},
		{"path fora do repo", "@LINP:1.0\nJOB x\nFILE ../secret\nALGO deflate\nCHUNK 1000\nPACK\nEND"},
		{"path absoluto", "@LINP:1.0\nJOB x\nFILE /etc/passwd\nALGO deflate\nCHUNK 1000\nPACK\nEND"},
		{"job duplicado", "@LINP:1.0\nJOB a\nJOB b\nFILE f\nALGO deflate\nCHUNK 1000\nPACK\nEND"},
		{"sem pack", "@LINP:1.0\nJOB a\nFILE f\nALGO deflate\nCHUNK 1000\nEND"},
	}

	for _, tc := range badInputs {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := Compile([]byte(tc.src))
			if err == nil {
				t.Fatalf("esperado erro para %q, mas compilou", tc.name)
			}
		})
	}
}
