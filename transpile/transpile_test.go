package transpile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Xelckis/floppyURL/lint"
)

func TestHTMLToLayTranspile(t *testing.T) {
	goodHTML := `<div><h1>Titulo</h1><p style="color: #00ff66">Ola</p><div class="col"><input id="a" placeholder="x" action="mount"><button action="mount">MONTAR</button></div></div>`

	lay1, err := HTMLToLay(goodHTML)
	if err != nil {
		t.Fatalf("HTMLToLay falhou: %v", err)
	}

	lay2, err := HTMLToLay(goodHTML)
	if err != nil {
		t.Fatalf("HTMLToLay 2 falhou: %v", err)
	}

	// 1. Determinismo
	if lay1 != lay2 {
		t.Fatalf("HTMLToLay nao deterministico")
	}

	if !strings.HasPrefix(lay1, "@LAY:1.0\nVIEW app\n") {
		t.Fatalf("cabecalho inesperado: %s", lay1)
	}

	// 2. O compilador lint nativo aceita a saida do transpilador
	_, err = lint.Compile([]byte(lay1))
	if err != nil {
		t.Fatalf("lint.Compile rejeitou saida do transpilador: %v\nSaida:\n%s", err, lay1)
	}

	// 3. Fail-closed: rejeitar tags/styles proibidos
	badInputs := []struct {
		name string
		html string
	}{
		{"script tag", "<script>alert(1)</script>"},
		{"style block", "<style>.a{color:red}</style>"},
		{"css arbitrario", `<div style="position: absolute">x</div>`},
		{"action desconhecida", `<button action="nuke">x</button>`},
		{"table tag", "<table><tr><td>x</td></tr></table>"},
		{"iframe tag", "<iframe></iframe>"},
	}

	for _, tc := range badInputs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := HTMLToLay(tc.html)
			if err == nil {
				t.Fatalf("esperava erro para %q, mas aceitou", tc.name)
			}
		})
	}

	// Fail-closed para demo.html (nao e subset simples, tem scripts e tags completas)
	demoPath := filepath.Join("..", "examples", "demo.html")
	demoBytes, err := os.ReadFile(demoPath)
	if err == nil {
		_, err := HTMLToLay(string(demoBytes))
		if err == nil {
			t.Fatalf("HTMLToLay deveria rejeitar demo.html pois contem tags fora do subset")
		}
	}
}

func TestPyToLinpTranspile(t *testing.T) {
	goodPy := "job(\"demo\")\npack(\"examples/demo.html\", algo=\"deflate\", chunk=1800000)\n"

	linp1, err := PyToLinp(goodPy)
	if err != nil {
		t.Fatalf("PyToLinp falhou: %v", err)
	}

	linp2, err := PyToLinp(goodPy)
	if err != nil {
		t.Fatalf("PyToLinp 2 falhou: %v", err)
	}

	if linp1 != linp2 {
		t.Fatalf("PyToLinp nao deterministico")
	}

	if !strings.HasPrefix(linp1, "@LINP:1.0\nJOB demo\nFILE examples/demo.html\nALGO deflate\nCHUNK 1800000\nPACK\nEND") {
		t.Fatalf("saida inesperada: %s", linp1)
	}

	// Fail-closed
	badPy := []struct {
		name string
		src  string
	}{
		{"import ilegal", "import os\njob(\"x\")\npack(\"a\")\n"},
		{"for loop ilegal", "job(\"x\")\nfor i in r:\n pack(\"a\")\n"},
		{"path fora do repo", "job(\"x\")\npack(\"/etc/passwd\")\n"},
		{"algo desconhecido", "job(\"x\")\npack(\"a\", algo=\"lzma\")\n"},
		{"job ausente", "pack(\"a\")\n"},
		{"multi pack", "job(\"x\")\npack(\"a\")\npack(\"b\")\n"},
	}

	for _, tc := range badPy {
		t.Run(tc.name, func(t *testing.T) {
			_, err := PyToLinp(tc.src)
			if err == nil {
				t.Fatalf("esperava erro para %q, mas aceitou", tc.name)
			}
		})
	}
}

func TestSpecToLinzTranspile(t *testing.T) {
	goodSpec := "envelope(\"v2\")\npayload(\"payload_data\")\nalgo(\"deflate\")\nchunk(1800000)\n"

	linz1, err := SpecToLinz(goodSpec)
	if err != nil {
		t.Fatalf("SpecToLinz falhou: %v", err)
	}

	linz2, err := SpecToLinz(goodSpec)
	if err != nil {
		t.Fatalf("SpecToLinz 2 falhou: %v", err)
	}

	if linz1 != linz2 {
		t.Fatalf("SpecToLinz nao deterministico")
	}

	if !strings.HasPrefix(linz1, "@LINZ:1.0\nENVELOPE v2\nPAYLOAD payload_data\nALGO deflate\nCHUNK 1800000\nASSEMBLE\nEND") {
		t.Fatalf("saida inesperada: %s", linz1)
	}

	// Fail-closed
	_, err = SpecToLinz("envelope(\"v1\")\npayload(\"x\")\nalgo(\"deflate\")\nchunk(100)\n")
	if err == nil {
		t.Fatalf("deveria rejeitar envelope v1")
	}
}

func TestJSToLinjTranspile(t *testing.T) {
	goodJS := "boot(\"app\");\nverify(\"sha256\");\ninflate(\"deflate\");\ndispatch(\"html\");\n"

	linj1, err := JSToLinj(goodJS)
	if err != nil {
		t.Fatalf("JSToLinj falhou: %v", err)
	}

	linj2, err := JSToLinj(goodJS)
	if err != nil {
		t.Fatalf("JSToLinj 2 falhou: %v", err)
	}

	if linj1 != linj2 {
		t.Fatalf("JSToLinj nao deterministico")
	}

	if !strings.HasPrefix(linj1, "@LINJ:1.0\nBOOT app\nEXPECT deflate\nEXPECT html\nSTEP VERIFY\nSTEP INFLATE\nSTEP DISPATCH\nEND") {
		t.Fatalf("saida inesperada: %s", linj1)
	}

	// Fail-closed
	_, err = JSToLinj("boot(\"app\");\ninflate(\"brotli\");\ndispatch(\"html\");\n")
	if err == nil {
		t.Fatalf("deveria rejeitar algo brotli no linj")
	}
}
