// Package mempipe — pipeline FloppyURL 100% em memoria.
//
// Diferente da versao original (main.go), que escreve disk_*.txt + manifests
// no disco, aqui tudo e []byte -> []byte: nenhuma escrita, nenhum diretorio.
// Compoe as irmas sem copiar logica: lint.Compile -> linp.Compress ->
// linz.Assemble + linz.BuildManifest. HTML minifica em memoria com o mesmo
// motor tdewolff da versao original (config espelhada de LoadMinifiers).
package mempipe

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/Xelckis/floppyURL/lint"
	"github.com/Xelckis/floppyURL/linp"
	"github.com/Xelckis/floppyURL/linz"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	jsonMinify "github.com/tdewolff/minify/v2/json"
	"github.com/tdewolff/minify/v2/svg"
	"github.com/tdewolff/minify/v2/xml"
)

// Bundle e o resultado completo em memoria: do fonte aos discos + provas.
type Bundle struct {
	Type         string   `json:"type"`
	Filename     string   `json:"filename"`
	Algo         string   `json:"algo"`
	BytecodeB64  string   `json:"bytecode_b64,omitempty"`
	WrapperBytes int      `json:"wrapper_bytes"`
	CompBytes    int      `json:"comp_bytes"`
	Disks        []string `json:"disks"`
	Root         string   `json:"root_sha256"`
	ManifestJSON []byte   `json:"-"`
	RuleL        []byte   `json:"-"`
	Receipt      []byte   `json:"-"`
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// loadMinifiers espelha main.go:LoadMinifiers (mesmo motor, mesma config).
func loadMinifiers(m *minify.M) {
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("image/svg+xml", svg.Minify)
	m.AddFuncRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), js.Minify)
	m.AddFuncRegexp(regexp.MustCompile("[/+]json$"), jsonMinify.Minify)
	m.AddFuncRegexp(regexp.MustCompile("[/+]xml$"), xml.Minify)
}

// finish amarra compressao + envelope + receipt, tudo em memoria.
func finish(dtype, filename string, payload, bytecode, source []byte, algo string, chunk int) (*Bundle, error) {
	if chunk < 64 || chunk > 1800000 {
		return nil, fmt.Errorf("CHUNK fora de [64,1800000]")
	}
	comp, err := linp.Compress(payload, algo)
	if err != nil {
		return nil, err
	}
	enc := base64.RawURLEncoding.EncodeToString(comp)
	zplan := &linz.Plan{Payload: filename, Algo: algo, Chunk: chunk}
	disks, root, err := linz.Assemble(zplan, enc)
	if err != nil {
		return nil, err
	}
	m := linz.BuildManifest(zplan, disks, root)
	receipt, _ := json.MarshalIndent(map[string]string{
		"pipe": "mempipe-P0", "type": dtype, "filename": filename, "algo": algo,
		"source_sha256": sha256Hex(source), "bytecode_sha256": sha256Hex(bytecode),
		"root_sha256": root,
	}, "", "  ")
	b := &Bundle{Type: dtype, Filename: filename, Algo: algo,
		WrapperBytes: len(payload), CompBytes: len(comp),
		Disks: disks, Root: root, ManifestJSON: m.JSON, RuleL: m.RuleL, Receipt: receipt}
	if len(bytecode) > 0 {
		b.BytecodeB64 = base64.RawURLEncoding.EncodeToString(bytecode)
	}
	return b, nil
}

// PipeLAY: .lay -> .laybc -> envelope v2. Identico ao path .lay do main.go.
func PipeLAY(filename string, laySrc []byte, algo string, chunk int) (*Bundle, error) {
	laybc, err := lint.Compile(laySrc)
	if err != nil {
		return nil, err
	}
	wrapper, _ := json.Marshal(map[string]interface{}{
		"type": "lay", "filename": filename,
		"bytecode": base64.RawURLEncoding.EncodeToString(laybc),
	})
	return finish("lay", filename, wrapper, laybc, laySrc, algo, chunk)
}

// PipeLIN: .lin -> envelope v2. Identico ao path .lin do main.go.
func PipeLIN(filename string, linSrc []byte, algo string, chunk int) (*Bundle, error) {
	wrapper, _ := json.Marshal(map[string]interface{}{
		"type": "lin", "filename": filename, "source": string(linSrc),
	})
	return finish("lin", filename, wrapper, linSrc, linSrc, algo, chunk)
}

// PipeHTML: .html -> minify em memoria -> envelope v2. Identico ao path html do main.go.
func PipeHTML(filename string, htmlSrc []byte, algo string, chunk int) (*Bundle, error) {
	m := minify.New()
	loadMinifiers(m)
	var buf bytes.Buffer
	if err := m.Minify("text/html", &buf, bytes.NewReader(htmlSrc)); err != nil {
		buf.Reset()
		buf.Write(htmlSrc)
	}
	return finish("html", filename, buf.Bytes(), buf.Bytes(), htmlSrc, algo, chunk)
}

// PipeRAW: bytes crus -> envelope v2 (semantica linp, sem minify).
func PipeRAW(filename string, raw []byte, algo string, chunk int) (*Bundle, error) {
	return finish("raw", filename, raw, raw, raw, algo, chunk)
}
