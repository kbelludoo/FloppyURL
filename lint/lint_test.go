package lint

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestCompileLAYDeterminismAndStructure(t *testing.T) {
	layPath := filepath.Join("..", "examples", "floppyurl_lay.lay")
	src, err := os.ReadFile(layPath)
	if err != nil {
		// Fallback se chamado de dentro de lint/
		src, err = os.ReadFile(filepath.Join("examples", "floppyurl_lay.lay"))
		if err != nil {
			t.Fatalf("ler floppyurl_lay.lay: %v", err)
		}
	}

	bc1, err := Compile(src)
	if err != nil {
		t.Fatalf("Compile 1 failed: %v", err)
	}

	bc2, err := Compile(src)
	if err != nil {
		t.Fatalf("Compile 2 failed: %v", err)
	}

	// 1. Determinismo byte-identico
	if !bytes.Equal(bc1, bc2) {
		t.Fatalf("compilacao nao e deterministica (bc1 != bc2)")
	}

	// 2. Cabecalho
	if len(bc1) < 11 {
		t.Fatalf("bytecode muito curto: %d bytes", len(bc1))
	}
	if string(bc1[:4]) != Magic {
		t.Fatalf("magic incorreto: %s", string(bc1[:4]))
	}
	if bc1[4] != Version {
		t.Fatalf("versao incorreta: %d", bc1[4])
	}

	// 3. Stats & Estrutura
	root, nnodes, nstr, err := Stats(bc1)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if root != 0 {
		t.Fatalf("root deveria ser 0, veio %d", root)
	}
	if nnodes < 3 {
		t.Fatalf("deveria ter pelo menos 3 nos, veio %d", nnodes)
	}
	if nstr < 3 {
		t.Fatalf("deveria ter pelo menos 3 strings, veio %d", nstr)
	}

	// 4. Verificacao manual de decodificacao de nos
	off := 11
	strs := make([]string, 0, nstr)
	for i := 0; i < nstr; i++ {
		ln := int(binary.LittleEndian.Uint16(bc1[off:]))
		off += 2
		strs = append(strs, string(bc1[off:off+ln]))
		off += ln
	}

	for i := 0; i < nnodes; i++ {
		if off+14 > len(bc1) {
			t.Fatalf("offset ultrapassou tamanho do bytecode")
		}
		parent := binary.LittleEndian.Uint16(bc1[off : off+2])
		tag := bc1[off+2]
		flags := bc1[off+3]
		ti := binary.LittleEndian.Uint16(bc1[off+4 : off+6])
		si := binary.LittleEndian.Uint16(bc1[off+6 : off+8])
		ai := binary.LittleEndian.Uint16(bc1[off+8 : off+10])
		aci := binary.LittleEndian.Uint16(bc1[off+10 : off+12])

		if parent != 0xFFFF && int(parent) >= nnodes {
			t.Fatalf("parent fora de range: %d (max: %d)", parent, nnodes)
		}
		if tag > 15 {
			t.Fatalf("tag ID invalida: %d", tag)
		}

		for _, item := range []struct {
			v uint16
			b uint8
		}{
			{ti, 0}, {si, 1}, {ai, 2}, {aci, 3},
		} {
			has := (flags & (1 << item.b)) != 0
			if has && int(item.v) >= len(strs) {
				t.Fatalf("string idx fora de range: %d", item.v)
			}
			if !has && item.v != 0xFFFF {
				t.Fatalf("sem flag mas com string idx definida")
			}
		}

		off += 14
	}
	if off != len(bc1) {
		t.Fatalf("bytes residuais no bytecode: esperado %d, obtido %d", len(bc1), off)
	}
}

func TestCompileLAYFailClosed(t *testing.T) {
	badSrc := []byte("@LAY:1.0\nVIEW app\nFOOBAR \"x\"\nEND\n")
	_, err := Compile(badSrc)
	if err == nil {
		t.Fatalf("esperado erro ao compilar tag desconhecida FOOBAR, mas passou")
	}

	// Stats com bytecode truncado
	_, _, _, err = Stats([]byte("LAY1\x01\x00\x00"))
	if err == nil {
		t.Fatalf("Stats deveria rejeitar bytecode truncado")
	}

	// Stats com magic invalido
	_, _, _, err = Stats([]byte("XXXX\x01\x00\x00\x00\x00\x00\x00"))
	if err == nil {
		t.Fatalf("Stats deveria rejeitar magic invalido")
	}
}
