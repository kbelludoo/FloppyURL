package linj

import (
	"bytes"
	"testing"
)

func TestLinjCompileAndDecodeRoundtrip(t *testing.T) {
	src := []byte(`@LINJ:1.0
BOOT test_app
EXPECT deflate
EXPECT html
STEP VERIFY
STEP INFLATE
STEP DISPATCH
END`)

	bc1, p1, err := Compile(src)
	if err != nil {
		t.Fatalf("Compile falhou: %v", err)
	}

	bc2, _, err := Compile(src)
	if err != nil {
		t.Fatalf("Compile 2 falhou: %v", err)
	}

	if !bytes.Equal(bc1, bc2) {
		t.Fatalf("bytecode nao deterministico")
	}

	if string(bc1[:4]) != Magic {
		t.Fatalf("magic invalido: %s", string(bc1[:4]))
	}
	if bc1[4] != Version {
		t.Fatalf("versao invalida: %d", bc1[4])
	}

	if p1.Boot != "test_app" || p1.Algo != "deflate" || p1.Dtype != "html" {
		t.Fatalf("plano compilado incorreto: %+v", p1)
	}

	decoded, err := Decode(bc1)
	if err != nil {
		t.Fatalf("Decode falhou: %v", err)
	}

	if decoded.Boot != p1.Boot || decoded.Algo != p1.Algo || decoded.Dtype != p1.Dtype {
		t.Fatalf("plano decodificado diverge: %+v vs %+v", decoded, p1)
	}
}

func TestLinjFailClosed(t *testing.T) {
	badInputs := []struct {
		name string
		src  string
	}{
		{"sem boot", "@LINJ:1.0\nEXPECT deflate\nEXPECT html\nSTEP VERIFY\nSTEP INFLATE\nSTEP DISPATCH\nEND"},
		{"algo brotli fora do P0", "@LINJ:1.0\nBOOT app\nEXPECT brotli\nEXPECT html\nSTEP VERIFY\nSTEP INFLATE\nSTEP DISPATCH\nEND"},
		{"dtype desconhecido", "@LINJ:1.0\nBOOT app\nEXPECT deflate\nEXPECT unknown\nSTEP VERIFY\nSTEP INFLATE\nSTEP DISPATCH\nEND"},
		{"steps fora de ordem", "@LINJ:1.0\nBOOT app\nEXPECT deflate\nEXPECT html\nSTEP INFLATE\nSTEP VERIFY\nSTEP DISPATCH\nEND"},
		{"step faltando", "@LINJ:1.0\nBOOT app\nEXPECT deflate\nEXPECT html\nSTEP VERIFY\nSTEP INFLATE\nEND"},
	}

	for _, tc := range badInputs {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := Compile([]byte(tc.src))
			if err == nil {
				t.Fatalf("esperava erro em %s, mas compilou", tc.name)
			}
		})
	}

	// Bytecode corrompido ou truncado
	_, err := Decode([]byte("LNJ1\x01\x00"))
	if err == nil {
		t.Fatalf("esperava erro ao decodificar bytecode truncado")
	}

	_, err = Decode([]byte("XXXX\x01\x00\x00\x00\x00\x00\x00\x00\x00"))
	if err == nil {
		t.Fatalf("esperava erro para magic invalido")
	}
}
