package mempipe

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMempipeLAY(t *testing.T) {
	laySrc := []byte("@LAY:1.0\nVIEW app\nSTYLE bg=#000\nH1 \"Test\"\nEND\n")
	b, err := PipeLAY("app.lay", laySrc, "deflate", 1800000)
	if err != nil {
		t.Fatalf("PipeLAY falhou: %v", err)
	}

	if b.Type != "lay" || b.Filename != "app.lay" || len(b.Disks) != 1 {
		t.Fatalf("bundle incorreto: %+v", b)
	}

	// Validar que o disco e valido
	parts := strings.Split(b.Disks[0], ";")
	if len(parts) != 6 || parts[0] != "v2" {
		t.Fatalf("formato de disco invalido: %s", b.Disks[0])
	}

	decBytes, err := base64.RawURLEncoding.DecodeString(parts[5])
	if err != nil {
		t.Fatalf("decode b64: %v", err)
	}

	r := flate.NewReader(bytes.NewReader(decBytes))
	defer r.Close()
	decomp, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("inflate: %v", err)
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(decomp, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["type"] != "lay" || obj["filename"] != "app.lay" {
		t.Fatalf("json payload diverge: %+v", obj)
	}
}

func TestMempipeHTML(t *testing.T) {
	htmlPath := filepath.Join("..", "examples", "demo.html")
	rawHTML, err := os.ReadFile(htmlPath)
	if err != nil {
		rawHTML, err = os.ReadFile(filepath.Join("examples", "demo.html"))
		if err != nil {
			t.Fatalf("ler demo.html: %v", err)
		}
	}

	b, err := PipeHTML("demo.html", rawHTML, "deflate", 1800000)
	if err != nil {
		t.Fatalf("PipeHTML falhou: %v", err)
	}

	if b.Type != "html" || len(b.Disks) != 1 {
		t.Fatalf("bundle incorreto: %+v", b)
	}

	parts := strings.Split(b.Disks[0], ";")
	decBytes, _ := base64.RawURLEncoding.DecodeString(parts[5])
	r := flate.NewReader(bytes.NewReader(decBytes))
	defer r.Close()
	decomp, _ := io.ReadAll(r)

	if !strings.Contains(string(decomp), "FloppyURL v2.0 Operacional") {
		t.Fatalf("conteudo descomprimido nao contem marcador")
	}

	// Validar receipt
	var rec map[string]string
	if err := json.Unmarshal(b.Receipt, &rec); err != nil {
		t.Fatalf("unmarshal receipt: %v", err)
	}
	if rec["pipe"] != "mempipe-P0" || rec["root_sha256"] != b.Root {
		t.Fatalf("receipt invalido: %+v", rec)
	}
}

func TestMempipeMultiDiskRAW(t *testing.T) {
	// 5000 bytes com variacao para garantir que o compressed base64 exceda o chunk de 100
	var buf bytes.Buffer
	for i := 0; i < 200; i++ {
		buf.WriteString(fmt.Sprintf("item_%d_token_%x_lorem_ipsum_dolor_sit_amet;", i, i*7919))
	}
	rawData := buf.Bytes()
	chunk := 100
	b, err := PipeRAW("data.bin", rawData, "deflate", chunk)
	if err != nil {
		t.Fatalf("PipeRAW falhou: %v", err)
	}

	if len(b.Disks) <= 1 {
		t.Fatalf("esperava particionamento, total=%d", len(b.Disks))
	}

	var assembled strings.Builder
	for i, d := range b.Disks {
		parts := strings.Split(d, ";")
		ch := parts[3]
		p := parts[5]
		h := sha256.Sum256([]byte(p))
		if hex.EncodeToString(h[:]) != ch {
			t.Fatalf("disco %d: hash incorreto", i+1)
		}
		assembled.WriteString(p)
	}

	rh := sha256.Sum256([]byte(assembled.String()))
	if hex.EncodeToString(rh[:]) != b.Root {
		t.Fatalf("root sha mismatch")
	}
}
