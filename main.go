// Command floppyurl is the FloppyURL packager: it turns a web page into
// attested, optionally encrypted "floppy disks" embedded in Base64URL.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/kbelludoo/FloppyURL/internal/floppy"
)

const version = "2.1.0"

func main() {
	inputFile := flag.String("file", "", "Caminho para o arquivo HTML de entrada (obrigatorio)")
	algo := flag.String("algo", floppy.AlgoBrotli, "Algoritmo de compressao: brotli (max), deflate (boot zero-wasm) ou gzip")
	chunkSize := flag.Int("chunk-size", floppy.DefaultChunkSize, "Tamanho maximo por disquete em caracteres")
	outputDir := flag.String("out-dir", "disks", "Diretorio para salvar os disquetes gerados")
	v1Compat := flag.Bool("v1", false, "Emitir formato legado v1 (sem hashes de integridade)")
	passphrase := flag.String("pass", "", "Senha para criptografia AES-256-GCM do payload (vazio = sem criptografia)")
	noInline := flag.Bool("no-inline", false, "Desativar inlining automatico de CSS/JS/imagens locais")
	showVersion := flag.Bool("version", false, "Mostrar versao e sair")
	flag.Parse()

	if *showVersion {
		fmt.Printf("FloppyURL v%s\n", version)
		return
	}

	if *inputFile == "" {
		log.Fatal("Uso: go run main.go -file <arquivo.html> [-algo brotli|deflate|gzip] [-chunk-size N] [-out-dir DIR] [-pass SENHA] [-v1] [-no-inline]")
	}

	fmt.Println("==================================================")
	fmt.Printf(" FloppyURL v%s - Zero-Byte Attested Web Packager \n", version)
	fmt.Println("==================================================")

	rawContent, err := os.ReadFile(*inputFile)
	if err != nil {
		log.Fatalf("Erro ao abrir arquivo de entrada: %v\n", err)
	}
	fmt.Printf("[1/6] Arquivo lido: %s (%d bytes)\n", *inputFile, len(rawContent))

	opts := floppy.Options{
		Algo:       strings.ToLower(*algo),
		ChunkSize:  *chunkSize,
		V1Compat:   *v1Compat,
		Passphrase: *passphrase,
		Inline:     !*noInline,
		SourceName: *inputFile,
	}

	result, err := floppy.Pack(rawContent, filepath.Dir(*inputFile), opts)
	if err != nil {
		log.Fatalf("Falha no empacotamento: %v\n", err)
	}

	m := result.Manifest
	if opts.Inline {
		fmt.Printf("[2/6] Minificacao concluida: %d bytes (de %d bytes, reducao: %.2f%%)\n",
			m.MinifiedBytes, m.OriginalBytes, pct(m.OriginalBytes, m.MinifiedBytes))
	} else {
		fmt.Printf("[2/6] Minificacao concluida: %d bytes (inlining desativado)\n", m.MinifiedBytes)
	}
	fmt.Printf("[3/6] Compressao [%s]: %d bytes (taxa: %.2fx menor)\n",
		m.Algorithm, m.CompressedBytes, ratio(m.OriginalBytes, m.CompressedBytes))
	if m.Encrypted {
		fmt.Printf("[4/6] Criptografia AES-256-GCM aplicada (PBKDF2-SHA256, %d iteracoes)\n", m.PBKDF2Iterations)
	} else {
		fmt.Println("[4/6] Criptografia desativada")
	}
	fmt.Printf("[5/6] Base64 RawURL gerado: %d caracteres\n", m.EncodedChars)

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		log.Fatalf("Erro ao criar diretorio de saida: %v\n", err)
	}

	fmt.Printf("[6/6] Gravando %d volume(s) RAID-0 em '%s/'...\n", m.TotalDisks, *outputDir)
	for _, disk := range result.Disks {
		name := filepath.Join(*outputDir, fmt.Sprintf("disk_%02d.txt", disk.Number))
		if err := os.WriteFile(name, []byte(disk.Formatted), 0o644); err != nil {
			log.Fatalf("Falha ao salvar disco %d: %v\n", disk.Number, err)
		}
		fmt.Printf("   -> Gravado: %s (%d chars | SHA256: %.12s...)\n", name, len(disk.Formatted), disk.SHA256)
	}

	manifestJSON, _ := json.MarshalIndent(m, "", "  ")
	_ = os.WriteFile(filepath.Join(*outputDir, "manifest.json"), manifestJSON, 0o644)
	// Compatibilidade com o fluxo v1: primeiro disco tambem no arquivo 'base64'.
	_ = os.WriteFile("base64", []byte(result.FirstDisk), 0o644)

	fmt.Println("--------------------------------------------------")
	fmt.Println("CONCLUIDO COM SUCESSO!")
	fmt.Printf("Raiz criptografica SHA-256: %s\n", m.RootSHA256)
	if m.TotalDisks == 1 {
		fmt.Printf("URL Direta de Boot:\nhttp://localhost:8080/#%s\n", result.FirstDisk)
		_ = os.WriteFile(filepath.Join(*outputDir, "boot_url.txt"), []byte("http://localhost:8080/#"+result.FirstDisk), 0o644)
	} else {
		fmt.Printf("Payload dividido em %d disquetes na pasta '%s/'.\n", m.TotalDisks, *outputDir)
		fmt.Printf("Para rodar: abra o bootloader e insira os discos (ou arraste os arquivos).\n")
	}
	fmt.Println("==================================================")
}

func pct(total, part int) float64 {
	if total == 0 {
		return 0
	}
	return 100.0 * (1.0 - float64(part)/float64(total))
}

func ratio(total, part int) float64 {
	if part == 0 {
		return 0
	}
	return float64(total) / float64(part)
}
