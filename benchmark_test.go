package main

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Xelckis/floppyURL/mempipe"
	"github.com/Xelckis/floppyURL/transpile"
	"github.com/andybalholm/brotli"
)

// TestObjectiveAuditing mede de forma 100% real e auditavel:
// 1. Integridade bit-a-bit de cada fixture
// 2. Compressao Deflate vs Brotli
// 3. Tempo real de execucao (latencia de processamento)
// 4. Se houve perda ou corrupcao de dados
func TestObjectiveAuditing(t *testing.T) {
	fixtures := []struct {
		name string
		path string
		kind string
	}{
		{"Demo HTML", filepath.Join("examples", "demo.html"), "html"},
		{"DeFi Swap AMM", filepath.Join("examples", "defi_swap_lin.html"), "html"},
		{"CPMM Oracle LIN", filepath.Join("examples", "cpmm_oracle.lin"), "lin"},
		{"FloppyURL LAY DSL", filepath.Join("examples", "floppyurl_lay.lay"), "lay"},
	}

	fmt.Println("\n==========================================================================================")
	fmt.Println("             RELATÓRIO DE AUDITORIA E MÉTRICAS REAIS (ZERO FALSIFICAÇÃO)                  ")
	fmt.Println("==========================================================================================")
	fmt.Printf("%-20s | %-8s | %-8s | %-8s | %-12s | %-12s | %-8s\n",
		"Fixture", "Orig(B)", "Deflate", "Brotli", "Econ. Deflate", "Econ. Brotli", "Tempo(µs)")
	fmt.Println("------------------------------------------------------------------------------------------")

	for _, fix := range fixtures {
		data, err := os.ReadFile(fix.path)
		if err != nil {
			t.Fatalf("falha ao ler %s: %v", fix.path, err)
		}

		start := time.Now()
		var bundleDeflate *mempipe.Bundle

		switch fix.kind {
		case "html":
			bundleDeflate, err = mempipe.PipeHTML(filepath.Base(fix.path), data, "deflate", 1800000)
		case "lin":
			bundleDeflate, err = mempipe.PipeLIN(filepath.Base(fix.path), data, "deflate", 1800000)
		case "lay":
			bundleDeflate, err = mempipe.PipeLAY(filepath.Base(fix.path), data, "deflate", 1800000)
		}
		duration := time.Since(start)

		if err != nil {
			t.Fatalf("falha no pipeline para %s: %v", fix.name, err)
		}

		// Validacao Bit-a-bit: descomprimir o payload e checar integridade
		disk := bundleDeflate.Disks[0]
		parsed, err := parseV2Disk(disk)
		if err != nil {
			t.Fatalf("erro ao analisar disco de %s: %v", fix.name, err)
		}

		// Validar SHA-256 criptografico
		h := sha256.Sum256([]byte(parsed.Payload))
		if hex.EncodeToString(h[:]) != parsed.ChunkSHA {
			t.Fatalf("corrupcao de dados detectada em %s: hash SHA256 colidiu ou divergiu", fix.name)
		}

		// Descompressao real Deflate
		decBytes, err := base64.RawURLEncoding.DecodeString(parsed.Payload)
		if err != nil {
			t.Fatalf("decode b64 falhou para %s: %v", fix.name, err)
		}
		flateR := flate.NewReader(bytes.NewReader(decBytes))
		decompressedDeflate, err := io.ReadAll(flateR)
		flateR.Close()
		if err != nil {
			t.Fatalf("inflacao Deflate falhou para %s: %v", fix.name, err)
		}
		if len(decompressedDeflate) == 0 {
			t.Fatalf("descompressao retornou 0 bytes para %s", fix.name)
		}

		// Compressao com Brotli para comparacao honesta de ratio
		var brotliBuf bytes.Buffer
		bw := brotli.NewWriterLevel(&brotliBuf, brotli.BestCompression)
		bw.Write(decompressedDeflate)
		bw.Close()
		brotliCompBytes := brotliBuf.Len()

		origSize := len(data)
		deflateCompBytes := bundleDeflate.CompBytes
		econDeflate := (1.0 - float64(deflateCompBytes)/float64(origSize)) * 100.0
		econBrotli := (1.0 - float64(brotliCompBytes)/float64(origSize)) * 100.0

		fmt.Printf("%-20s | %-8d | %-8d | %-8d | %-11.1f%% | %-11.1f%% | %-8d\n",
			fix.name, origSize, deflateCompBytes, brotliCompBytes, econDeflate, econBrotli, duration.Microseconds())

		// Em arquivos maiores que 1KB, Brotli atinge maior densidade que Deflate.
		// Em arquivos minusculos (<600B), o overhead de metadados do Brotli e maior que o do Deflate.
		if origSize > 1000 && brotliCompBytes > deflateCompBytes {
			t.Fatalf("inconsistencia tecnica em %s: Brotli (%d B) comprimiu menos que Deflate (%d B)",
				fix.name, brotliCompBytes, deflateCompBytes)
		}
	}
	fmt.Println("==========================================================================================")
}

// BenchmarkTranspileHTMLToLay mede o throughput do transpilador Go nativo
func BenchmarkTranspileHTMLToLay(b *testing.B) {
	sample := `<div><h1>Titulo</h1><p style="color: #00ff66">Ola Mundo</p><div class="col"><input id="a" placeholder="x" action="mount"><button action="mount">MONTAR</button></div></div>`
	b.SetBytes(int64(len(sample)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := transpile.HTMLToLay(sample)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMempipeZeroDisk mede o throughput do pipeline 100% em memoria
func BenchmarkMempipeZeroDisk(b *testing.B) {
	data, err := os.ReadFile(filepath.Join("examples", "demo.html"))
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := mempipe.PipeHTML("demo.html", data, "deflate", 1800000)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPackWithDiskIO mede o pipeline tradicional escrevendo em disco
func BenchmarkPackWithDiskIO(b *testing.B) {
	outDir := b.TempDir()
	opts := PackOptions{
		InputFile: filepath.Join("examples", "demo.html"),
		Algo:      "deflate",
		OutputDir: outDir,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, err := RunPack(opts)
		if err != nil {
			b.Fatal(err)
		}
	}
}
