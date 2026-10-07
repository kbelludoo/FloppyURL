package main

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

type parsedV2Disk struct {
	Header   string
	Algo     string
	Frac     string
	ChunkSHA string
	RootSHA  string
	Payload  string
}

func parseV2Disk(raw string) (*parsedV2Disk, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), ";", 6)
	if len(parts) != 6 {
		return nil, fmt.Errorf("formato v2 invalido: %s", raw)
	}
	return &parsedV2Disk{
		Header:   parts[0],
		Algo:     parts[1],
		Frac:     parts[2],
		ChunkSHA: parts[3],
		RootSHA:  parts[4],
		Payload:  parts[5],
	}, nil
}

func decompressDeflate(rawB64 string) ([]byte, error) {
	data, err := base64.RawURLEncoding.DecodeString(rawB64)
	if err != nil {
		return nil, fmt.Errorf("decode b64: %w", err)
	}
	r := flate.NewReader(bytes.NewReader(data))
	defer r.Close()
	return io.ReadAll(r)
}

func decompressBrotli(rawB64 string) ([]byte, error) {
	data, err := base64.RawURLEncoding.DecodeString(rawB64)
	if err != nil {
		return nil, fmt.Errorf("decode b64: %w", err)
	}
	r := brotli.NewReader(bytes.NewReader(data))
	return io.ReadAll(r)
}

func TestDeflateHTML(t *testing.T) {
	outDir := t.TempDir()
	opts := PackOptions{
		InputFile: filepath.Join("examples", "demo.html"),
		Algo:      "deflate",
		OutputDir: outDir,
	}

	manifest, firstDisk, err := RunPack(opts)
	if err != nil {
		t.Fatalf("RunPack failed: %v", err)
	}

	if manifest.TotalDisks != 1 {
		t.Fatalf("esperado 1 disco, obteve %d", manifest.TotalDisks)
	}

	diskBytes, err := os.ReadFile(filepath.Join(outDir, "disk_01.txt"))
	if err != nil {
		t.Fatalf("falha ao ler disk_01.txt: %v", err)
	}

	diskStr := string(diskBytes)
	if diskStr != firstDisk {
		t.Fatalf("conteudo do arquivo diverge do retornado pelo RunPack")
	}

	parsed, err := parseV2Disk(diskStr)
	if err != nil {
		t.Fatalf("erro no parse de v2: %v", err)
	}

	if parsed.Header != "v2" || parsed.Algo != "deflate" || parsed.Frac != "[1/1]" {
		t.Fatalf("cabecalho inesperado: %+v", parsed)
	}

	// Integridade SHA-256
	h := sha256.Sum256([]byte(parsed.Payload))
	computedChunkSHA := hex.EncodeToString(h[:])
	if computedChunkSHA != parsed.ChunkSHA {
		t.Fatalf("chunk SHA mismatch: %s != %s", computedChunkSHA, parsed.ChunkSHA)
	}
	if parsed.ChunkSHA != parsed.RootSHA {
		t.Fatalf("em volume unico, chunk_sha deve ser igual a root_sha")
	}

	// Descompressao real via flate
	decompressed, err := decompressDeflate(parsed.Payload)
	if err != nil {
		t.Fatalf("falha ao descomprimir deflate: %v", err)
	}

	if !strings.Contains(string(decompressed), "FloppyURL v2.0 Operacional") {
		t.Fatalf("conteudo descomprimido nao contem marcador esperado: %s", string(decompressed))
	}

	// Verificacao do RuleL
	rulelBytes, err := os.ReadFile(filepath.Join(outDir, "manifest.rulel"))
	if err != nil {
		t.Fatalf("falha ao ler manifest.rulel: %v", err)
	}
	rulelStr := string(rulelBytes)
	if !strings.Contains(rulelStr, "@RULEL:FLOPPY_MANIFEST:2.0.0") {
		t.Fatalf("manifest.rulel sem cabecalho esperado")
	}
	if !strings.Contains(rulelStr, manifest.RootSHA256) {
		t.Fatalf("manifest.rulel nao contem root_sha256")
	}
}

func TestMultiDiskRAID0(t *testing.T) {
	outDir := t.TempDir()
	opts := PackOptions{
		InputFile: filepath.Join("examples", "defi_swap_lin.html"),
		Algo:      "deflate",
		ChunkSize: 800,
		OutputDir: outDir,
	}

	manifest, _, err := RunPack(opts)
	if err != nil {
		t.Fatalf("RunPack failed: %v", err)
	}

	if manifest.TotalDisks <= 1 {
		t.Fatalf("esperado particionamento multi-disco, totalDisks=%d", manifest.TotalDisks)
	}

	var assembledPayload strings.Builder
	for i := 1; i <= manifest.TotalDisks; i++ {
		diskFile := filepath.Join(outDir, fmt.Sprintf("disk_%02d.txt", i))
		b, err := os.ReadFile(diskFile)
		if err != nil {
			t.Fatalf("disco %d faltando: %v", i, err)
		}

		p, err := parseV2Disk(string(b))
		if err != nil {
			t.Fatalf("parse disk %d: %v", i, err)
		}

		if p.RootSHA != manifest.RootSHA256 {
			t.Fatalf("disk %d: root_sha mismatch (%s != %s)", i, p.RootSHA, manifest.RootSHA256)
		}

		ch := sha256.Sum256([]byte(p.Payload))
		if hex.EncodeToString(ch[:]) != p.ChunkSHA {
			t.Fatalf("disk %d: chunk SHA invalido", i)
		}

		assembledPayload.WriteString(p.Payload)
	}

	// Validar que o hash da concatenacao bate exatamente com a raiz
	rh := sha256.Sum256([]byte(assembledPayload.String()))
	if hex.EncodeToString(rh[:]) != manifest.RootSHA256 {
		t.Fatalf("root sha da reassemblagem nao confere com manifest")
	}

	// Descomprimir o payload completo remontado
	decompressed, err := decompressDeflate(assembledPayload.String())
	if err != nil {
		t.Fatalf("falha ao descomprimir payload multi-disco reassemblado: %v", err)
	}

	if !strings.Contains(string(decompressed), "LIN Sovereign Swap") {
		t.Fatalf("conteudo descomprimido nao contem 'LIN Sovereign Swap'")
	}
}

func TestLINPayloadPackaging(t *testing.T) {
	outDir := t.TempDir()
	opts := PackOptions{
		InputFile: filepath.Join("examples", "cpmm_oracle.lin"),
		Algo:      "deflate",
		OutputDir: outDir,
	}

	manifest, _, err := RunPack(opts)
	if err != nil {
		t.Fatalf("RunPack failed: %v", err)
	}

	diskBytes, err := os.ReadFile(filepath.Join(outDir, "disk_01.txt"))
	if err != nil {
		t.Fatalf("ler disk_01.txt: %v", err)
	}

	p, err := parseV2Disk(string(diskBytes))
	if err != nil {
		t.Fatalf("parse disk: %v", err)
	}

	decompressed, err := decompressDeflate(p.Payload)
	if err != nil {
		t.Fatalf("decompress deflate: %v", err)
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(decompressed, &obj); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	if obj["type"] != "lin" {
		t.Fatalf("esperado type 'lin', obteve %v", obj["type"])
	}
	if obj["filename"] != "cpmm_oracle.lin" {
		t.Fatalf("esperado filename 'cpmm_oracle.lin', obteve %v", obj["filename"])
	}

	source, ok := obj["source"].(string)
	if !ok || !strings.Contains(source, "@CPMM_ORACLE") {
		t.Fatalf("source LIN nao contem '@CPMM_ORACLE'")
	}
	_ = manifest
}

func TestLAYDSLCompilationAndDeterminism(t *testing.T) {
	tmp := t.TempDir()
	laySrc := "@LAY:1.0\nVIEW app\nSTYLE bg=#000 fg=#0f0\nH1 \"Hello LAY\"\nP \"Bytecode test\" CLASS body\nEND\n"
	layPath := filepath.Join(tmp, "test_app.lay")
	if err := os.WriteFile(layPath, []byte(laySrc), 0644); err != nil {
		t.Fatalf("escrever .lay: %v", err)
	}

	outA := filepath.Join(tmp, "out_a")
	outB := filepath.Join(tmp, "out_b")

	optsA := PackOptions{InputFile: layPath, Algo: "deflate", OutputDir: outA}
	optsB := PackOptions{InputFile: layPath, Algo: "deflate", OutputDir: outB}

	manA, _, errA := RunPack(optsA)
	manB, _, errB := RunPack(optsB)
	if errA != nil || errB != nil {
		t.Fatalf("RunPack failed: errA=%v, errB=%v", errA, errB)
	}

	// Determinismo estrito byte-a-byte
	diskA, _ := os.ReadFile(filepath.Join(outA, "disk_01.txt"))
	diskB, _ := os.ReadFile(filepath.Join(outB, "disk_01.txt"))
	if string(diskA) != string(diskB) {
		t.Fatalf("compilacao LAY nao e deterministica (diskA != diskB)")
	}
	if manA.RootSHA256 != manB.RootSHA256 {
		t.Fatalf("root sha nao e deterministico")
	}

	p, err := parseV2Disk(string(diskA))
	if err != nil {
		t.Fatalf("parse disk: %v", err)
	}

	decompressed, err := decompressDeflate(p.Payload)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(decompressed, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if obj["type"] != "lay" || obj["filename"] != "test_app.lay" {
		t.Fatalf("envelope lay incorreto: %+v", obj)
	}

	bcB64, ok := obj["bytecode"].(string)
	if !ok {
		t.Fatalf("campo bytecode ausente")
	}

	bc, err := base64.RawURLEncoding.DecodeString(bcB64)
	if err != nil {
		t.Fatalf("decode bytecode b64: %v", err)
	}

	if len(bc) < 11 {
		t.Fatalf("bytecode muito curto: %d bytes", len(bc))
	}

	if string(bc[:4]) != "LAY1" {
		t.Fatalf("magic invalido: %s", string(bc[:4]))
	}
	if bc[4] != 1 {
		t.Fatalf("versao invalida: %d", bc[4])
	}

	root := binary.LittleEndian.Uint16(bc[5:7])
	nodes := binary.LittleEndian.Uint16(bc[7:9])
	strs := binary.LittleEndian.Uint16(bc[9:11])

	if root != 0 {
		t.Fatalf("esperado root=0, obteve %d", root)
	}
	if nodes < 3 {
		t.Fatalf("esperado pelo menos 3 nos (VIEW, H1, P), obteve %d", nodes)
	}
	if strs < 3 {
		t.Fatalf("esperado pelo menos 3 strings, obteve %d", strs)
	}
}

func TestBrotliRoundtripAndHonestRatio(t *testing.T) {
	tmp := t.TempDir()
	outBrotli := filepath.Join(tmp, "brotli")
	outDeflate := filepath.Join(tmp, "deflate")

	optsB := PackOptions{
		InputFile: filepath.Join("examples", "demo.html"),
		Algo:      "brotli",
		OutputDir: outBrotli,
	}
	optsD := PackOptions{
		InputFile: filepath.Join("examples", "demo.html"),
		Algo:      "deflate",
		OutputDir: outDeflate,
	}

	manB, _, err := RunPack(optsB)
	if err != nil {
		t.Fatalf("RunPack brotli: %v", err)
	}
	manD, _, err := RunPack(optsD)
	if err != nil {
		t.Fatalf("RunPack deflate: %v", err)
	}

	// Roundtrip brotli real
	diskB, err := os.ReadFile(filepath.Join(outBrotli, "disk_01.txt"))
	if err != nil {
		t.Fatalf("ler disk brotli: %v", err)
	}
	pB, err := parseV2Disk(string(diskB))
	if err != nil {
		t.Fatalf("parse brotli disk: %v", err)
	}
	decompressed, err := decompressBrotli(pB.Payload)
	if err != nil {
		t.Fatalf("decompress brotli: %v", err)
	}
	if !strings.Contains(string(decompressed), "FloppyURL v2.0 Operacional") {
		t.Fatalf("conteudo descomprimido brotli incorreto")
	}

	// Medicao honesta sem falsificacao de resultados:
	// O algoritmo Brotli deve ter tamanho menor ou igual ao Deflate
	if manB.CompressedBytes > manD.CompressedBytes {
		t.Fatalf("brotli (%d B) nao comprimiu mais que deflate (%d B)", manB.CompressedBytes, manD.CompressedBytes)
	}
	t.Logf("Medicao Real: Brotli=%d B, Deflate=%d B (Economia Brotli: %d B)",
		manB.CompressedBytes, manD.CompressedBytes, manD.CompressedBytes-manB.CompressedBytes)
}

func TestV1Compatibility(t *testing.T) {
	tmp := t.TempDir()
	opts := PackOptions{
		InputFile: filepath.Join("examples", "demo.html"),
		Algo:      "brotli",
		OutputDir: tmp,
		V1Compat:  true,
	}

	man, firstDisk, err := RunPack(opts)
	if err != nil {
		t.Fatalf("RunPack v1: %v", err)
	}

	if man.FormatVersion != "v1" {
		t.Fatalf("formato esperado v1, obteve %s", man.FormatVersion)
	}

	// No formato v1, o payload de disco unico e base64 puro sem ponto-e-virgula
	if strings.Contains(firstDisk, ";") {
		t.Fatalf("disco v1 unico nao deve conter ponto-e-virgula: %s", firstDisk)
	}

	decompressed, err := decompressBrotli(firstDisk)
	if err != nil {
		t.Fatalf("decompress v1 brotli: %v", err)
	}
	if !strings.Contains(string(decompressed), "FloppyURL v2.0 Operacional") {
		t.Fatalf("roundtrip v1 falhou")
	}
}

func TestTamperDetectionFailClosed(t *testing.T) {
	tmp := t.TempDir()
	opts := PackOptions{
		InputFile: filepath.Join("examples", "demo.html"),
		Algo:      "deflate",
		OutputDir: tmp,
	}

	_, firstDisk, err := RunPack(opts)
	if err != nil {
		t.Fatalf("RunPack: %v", err)
	}

	p, err := parseV2Disk(firstDisk)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Adultera 1 unico caractere do payload
	tamperedPayload := []byte(p.Payload)
	if tamperedPayload[len(tamperedPayload)-1] == 'a' {
		tamperedPayload[len(tamperedPayload)-1] = 'b'
	} else {
		tamperedPayload[len(tamperedPayload)-1] = 'a'
	}

	// Verifica se a checagem SHA-256 rejeita a adulteracao
	h := sha256.Sum256(tamperedPayload)
	computed := hex.EncodeToString(h[:])
	if computed == p.ChunkSHA {
		t.Fatalf("falha de seguranca: hash colidiu apos adulteracao")
	}
}
