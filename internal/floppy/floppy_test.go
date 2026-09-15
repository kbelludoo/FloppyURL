package floppy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const demoHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Demo</title>
  <style>body { color: red;   }</style>
</head>
<body>
  <h1>OLÁ MUNDO FLOPPY</h1>
  <script>console.log("oi");</script>
</body>
</html>`

func packForTest(t *testing.T, opts Options) *Result {
	t.Helper()
	res, err := Pack([]byte(demoHTML), t.TempDir(), opts)
	if err != nil {
		t.Fatalf("Pack falhou: %v", err)
	}
	return res
}

func joinPayloads(res *Result) string {
	var sb strings.Builder
	for _, d := range res.Disks {
		sb.WriteString(d.Payload)
	}
	return sb.String()
}

func TestRoundTripAllAlgorithms(t *testing.T) {
	for _, algo := range []string{AlgoBrotli, AlgoDeflate, AlgoGzip} {
		t.Run(algo, func(t *testing.T) {
			res := packForTest(t, Options{Algo: algo, ChunkSize: 100})

			if res.Manifest.TotalDisks != len(res.Disks) {
				t.Fatalf("manifest/total disks divergentes: %d != %d", res.Manifest.TotalDisks, len(res.Disks))
			}
			joined := joinPayloads(res)
			if SHA256Hex([]byte(joined)) != res.Manifest.RootSHA256 {
				t.Fatalf("hash raiz não confere com payload concatenado")
			}
			for i, d := range res.Disks {
				if SHA256Hex([]byte(d.Payload)) != d.SHA256 {
					t.Fatalf("disco %d: hash do chunk inválido", d.Number)
				}
				if d.SHA256 != res.Manifest.DiskHashes[i] {
					t.Fatalf("disco %d: hash fora do manifest", d.Number)
				}
			}

			raw, err := base64.RawURLEncoding.DecodeString(joined)
			if err != nil {
				t.Fatalf("base64 inválido: %v", err)
			}
			minified, err := Decompress(raw, algo)
			if err != nil {
				t.Fatalf("decompress falhou: %v", err)
			}
			want, err := MinifyHTML([]byte(demoHTML))
			if err != nil {
				t.Fatalf("minify de referência falhou: %v", err)
			}
			if !bytes.Equal(minified, want) {
				t.Fatalf("roundtrip divergiu:\n got: %q\nwant: %q", minified, want)
			}
			if got, want := res.Manifest.MinifiedSHA256, SHA256Hex(want); got != want {
				t.Fatalf("manifest.minified_sha256 inválido: %s != %s", got, want)
			}
		})
	}
}

func TestChunkingHasNoEmptyLastDisk(t *testing.T) {
	for _, size := range []int{1, 2, 7, 100} {
		res := packForTest(t, Options{Algo: AlgoDeflate, ChunkSize: size})
		encodedLen := res.Manifest.EncodedChars
		want := (encodedLen + size - 1) / size
		if len(res.Disks) != want {
			t.Fatalf("chunk-size %d: esperado %d discos, obtido %d", size, want, len(res.Disks))
		}
		for _, d := range res.Disks {
			if d.Payload == "" {
				t.Fatalf("chunk-size %d: disco vazio gerado", size)
			}
		}
	}
}

func TestExactMultipleChunkSize(t *testing.T) {
	// payload cujo tamanho é múltiplo exato do chunk-size não pode gerar
	// um disco vazio extra (bug da v1).
	res := packForTest(t, Options{Algo: AlgoDeflate, ChunkSize: 4})
	if res.Manifest.EncodedChars%4 == 0 && len(res.Disks) != res.Manifest.EncodedChars/4 {
		t.Fatalf("múltiplo exato gerou %d discos para %d chars", len(res.Disks), res.Manifest.EncodedChars)
	}
}

func TestV2DiskFormat(t *testing.T) {
	res := packForTest(t, Options{Algo: AlgoDeflate, ChunkSize: 50})
	re := regexp.MustCompile(`^v2;deflate;\[1/(\d+)\];([0-9a-f]{64});([0-9a-f]{64});(.+)$`)
	m := re.FindStringSubmatch(res.Disks[0].Formatted)
	if m == nil {
		t.Fatalf("formato de disco v2 inesperado: %q", res.Disks[0].Formatted)
	}
	if m[1] != fmt.Sprintf("%d", res.Manifest.TotalDisks) {
		t.Fatalf("total no cabeçalho diverge do manifest: %s != %d", m[1], res.Manifest.TotalDisks)
	}
	if m[3] != res.Manifest.RootSHA256 {
		t.Fatalf("hash raiz no cabeçalho diverge do manifest")
	}
	if res.Manifest.FormatVersion != FormatV2 || res.Manifest.Encrypted {
		t.Fatalf("manifest deveria ser v2 não criptografado: %+v", res.Manifest)
	}
}

func TestV1CompatFormat(t *testing.T) {
	res := packForTest(t, Options{Algo: AlgoBrotli, ChunkSize: 50, V1Compat: true})
	if res.Manifest.FormatVersion != FormatV1 {
		t.Fatalf("esperado v1, obtido %s", res.Manifest.FormatVersion)
	}
	re := regexp.MustCompile(`^v1;\[1/\d+\]`)
	if !re.MatchString(res.Disks[0].Formatted) {
		t.Fatalf("disco v1 sem assinatura: %q", res.Disks[0].Formatted)
	}

	single := packForTest(t, Options{Algo: AlgoBrotli, V1Compat: true})
	if strings.HasPrefix(single.FirstDisk, "v1;") {
		t.Fatalf("disco único v1 não deveria ter prefixo: %q", single.FirstDisk)
	}
}

func TestEncryptedRoundTrip(t *testing.T) {
	res := packForTest(t, Options{Algo: AlgoDeflate, ChunkSize: 64, Passphrase: "senhaforte123"})
	if res.Manifest.FormatVersion != FormatV2E || !res.Manifest.Encrypted {
		t.Fatalf("esperado v2e criptografado: %+v", res.Manifest)
	}
	if res.Manifest.PBKDF2Iterations != PBKDF2Iterations {
		t.Fatalf("iterações PBKDF2 não registradas no manifest")
	}

	joined := joinPayloads(res)
	if SHA256Hex([]byte(joined)) != res.Manifest.RootSHA256 {
		t.Fatalf("hash raiz (do payload criptografado) não confere")
	}

	raw, err := base64.RawURLEncoding.DecodeString(joined)
	if err != nil {
		t.Fatalf("base64 inválido: %v", err)
	}
	if _, err := decrypt(raw, "senha-errada", PBKDF2Iterations); err == nil {
		t.Fatalf("senha errada foi aceita pelo AES-GCM")
	}
	compressed, err := decrypt(raw, "senhaforte123", PBKDF2Iterations)
	if err != nil {
		t.Fatalf("senha correta rejeitada: %v", err)
	}
	minified, err := Decompress(compressed, AlgoDeflate)
	if err != nil {
		t.Fatalf("decompress pós-decript falhou: %v", err)
	}
	if !strings.Contains(string(minified), "OLÁ MUNDO FLOPPY") {
		t.Fatalf("conteúdo inesperado pós roundtrip criptografado: %q", minified)
	}
}

func TestV1WithPassphraseRejected(t *testing.T) {
	if _, err := Pack([]byte(demoHTML), t.TempDir(), Options{V1Compat: true, Passphrase: "x"}); err == nil {
		t.Fatalf("v1 + senha deveria ser rejeitado")
	}
}

func TestPBKDF2RFC7914Vector(t *testing.T) {
	// Vetor oficial PBKDF2-HMAC-SHA256 (draft-josefsson-scrypt-kdf / RFC 7914 §11):
	// P="password", S="salt", c=1, dkLen=32
	got := PBKDF2Key([]byte("password"), []byte("salt"), 1, 32)
	want := "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"
	if gotHex := strings.ToLower(sha2hex(got)); gotHex != want {
		t.Fatalf("PBKDF2 divergiu do vetor RFC: %s", gotHex)
	}
	// c=4096 deve ser determinístico (autoteste de consistência interna)
	a := PBKDF2Key([]byte("password"), []byte("salt"), 4096, 32)
	b := PBKDF2Key([]byte("password"), []byte("salt"), 4096, 32)
	if sha2hex(a) != sha2hex(b) {
		t.Fatalf("PBKDF2 não determinístico")
	}
	if len(PBKDF2Key([]byte("p"), []byte("s"), 1, 64)) != 64 {
		t.Fatalf("dkLen respeitado")
	}
}

func sha2hex(b []byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, hexDigits[c>>4], hexDigits[c&0x0f])
	}
	return string(out)
}

func TestInlineAssets(t *testing.T) {
	dir := t.TempDir()
	png1x1 := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
		0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
	files := map[string]string{
		"style.css": "body{color:#0f0}",
		"app.js":    "console.log('inline!');",
		"logo.png":  string(png1x1),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	page := `<html><head><link rel="stylesheet" href="style.css"></head>
<body><img src="logo.png"><script src="app.js"></script>
<script type="module" src="ext-module.js"></script>
<script src="https://cdn.example.com/x.js"></script></body></html>`

	got, notes := InlineAssets([]byte(page), dir)
	s := string(got)
	if !strings.Contains(s, "<style>body{color:#0f0}</style>") {
		t.Fatalf("css não inlinado: %s", s)
	}
	if !strings.Contains(s, "<script>console.log('inline!');</script>") {
		t.Fatalf("js não inlinado: %s", s)
	}
	if !strings.Contains(s, "data:image/png;base64,") {
		t.Fatalf("imagem não inlinada: %s", s)
	}
	if !strings.Contains(s, `type="module" src="ext-module.js"`) {
		t.Fatalf("module script deveria ser preservado: %s", s)
	}
	if !strings.Contains(s, "https://cdn.example.com/x.js") {
		t.Fatalf("script remoto deveria ser preservado: %s", s)
	}
	if len(notes) != 3 {
		t.Fatalf("esperava 3 notas de inlining, obtive %v", notes)
	}
}

func TestMinifyReduces(t *testing.T) {
	out, err := MinifyHTML([]byte(demoHTML))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) >= len(demoHTML) {
		t.Fatalf("minify não reduziu: %d >= %d", len(out), len(demoHTML))
	}
}

func TestManifestJSONShape(t *testing.T) {
	res := packForTest(t, Options{Algo: AlgoGzip})
	data, err := json.Marshal(res.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"source_file", "algorithm", "minified_sha256", "root_sha256", "format_version", "total_disks", "disk_hashes"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("manifest sem chave %s: %s", key, data)
		}
	}
}
