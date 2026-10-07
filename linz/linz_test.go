package linz

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinzCompileAndAssemble(t *testing.T) {
	src := []byte(`@LINZ:1.0
ENVELOPE v2
PAYLOAD test_payload
ALGO deflate
CHUNK 500
ASSEMBLE
END`)

	bc1, plan1, err := Compile(src)
	if err != nil {
		t.Fatalf("Compile falhou: %v", err)
	}

	bc2, _, err := Compile(src)
	if err != nil {
		t.Fatalf("Compile 2 falhou: %v", err)
	}

	if !bytes.Equal(bc1, bc2) {
		t.Fatalf("compilacao linzbc nao deterministica")
	}

	if string(bc1[:4]) != Magic {
		t.Fatalf("magic invalido: %s", string(bc1[:4]))
	}

	decoded, err := Decode(bc1)
	if err != nil {
		t.Fatalf("Decode falhou: %v", err)
	}

	if decoded.Payload != plan1.Payload || decoded.Algo != plan1.Algo || decoded.Chunk != plan1.Chunk {
		t.Fatalf("Decode diverge: %+v vs %+v", decoded, plan1)
	}

	// Montagem multi-disco (payload com 1200 chars -> 3 discos com chunk 500)
	payloadMock := strings.Repeat("abcdefghij", 120) // 1200 chars
	disks, root, err := Assemble(plan1, payloadMock)
	if err != nil {
		t.Fatalf("Assemble falhou: %v", err)
	}

	if len(disks) != 3 {
		t.Fatalf("esperava 3 discos, obteve %d", len(disks))
	}

	rootHash := sha256.Sum256([]byte(payloadMock))
	if root != hex.EncodeToString(rootHash[:]) {
		t.Fatalf("root sha incorreto")
	}

	// Validar que cada disco contem o hash de chunk e o root corretos
	var reassembled strings.Builder
	for i, d := range disks {
		parts := strings.Split(d, ";")
		if len(parts) != 6 {
			t.Fatalf("disco %d com formato invalido: %s", i+1, d)
		}
		ch, r, p := parts[3], parts[4], parts[5]
		if r != root {
			t.Fatalf("disco %d: root diverge", i+1)
		}
		h := sha256.Sum256([]byte(p))
		if hex.EncodeToString(h[:]) != ch {
			t.Fatalf("disco %d: chunk sha invalido", i+1)
		}
		reassembled.WriteString(p)
	}

	if reassembled.String() != payloadMock {
		t.Fatalf("remontagem diverge do original")
	}

	// Testar gravacao em disco e manifestos
	outDir := t.TempDir()
	err = WriteOut(plan1, bc1, src, disks, root, outDir)
	if err != nil {
		t.Fatalf("WriteOut falhou: %v", err)
	}

	rulelBytes, err := os.ReadFile(filepath.Join(outDir, "manifest.rulel"))
	if err != nil {
		t.Fatalf("ler manifest.rulel: %v", err)
	}
	if !strings.Contains(string(rulelBytes), "@RULEL:FLOPPY_MANIFEST:2.0.0") {
		t.Fatalf("manifest.rulel sem cabecalho")
	}
	if !strings.Contains(string(rulelBytes), root) {
		t.Fatalf("manifest.rulel sem root_sha")
	}
}

func TestLinzFailClosed(t *testing.T) {
	badInputs := []struct {
		name string
		src  string
	}{
		{"envelope v1 rejeitado no P0", "@LINZ:1.0\nENVELOPE v1\nPAYLOAD p\nALGO deflate\nCHUNK 500\nASSEMBLE\nEND"},
		{"algo invalido", "@LINZ:1.0\nENVELOPE v2\nPAYLOAD p\nALGO lzma\nCHUNK 500\nASSEMBLE\nEND"},
		{"chunk fora de limites", "@LINZ:1.0\nENVELOPE v2\nPAYLOAD p\nALGO deflate\nCHUNK 10\nASSEMBLE\nEND"},
		{"payload com path traversal", "@LINZ:1.0\nENVELOPE v2\nPAYLOAD ../secret\nALGO deflate\nCHUNK 500\nASSEMBLE\nEND"},
		{"assemble duplicado", "@LINZ:1.0\nENVELOPE v2\nPAYLOAD p\nALGO deflate\nCHUNK 500\nASSEMBLE\nASSEMBLE\nEND"},
	}

	for _, tc := range badInputs {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := Compile([]byte(tc.src))
			if err == nil {
				t.Fatalf("esperava erro em %s, mas compilou", tc.name)
			}
		})
	}

	_, err := Decode([]byte("LNZ1\x01"))
	if err == nil {
		t.Fatalf("esperava erro ao decodificar truncado")
	}
}
