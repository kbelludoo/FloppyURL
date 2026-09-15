package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/andybalholm/brotli"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	jsonMinify "github.com/tdewolff/minify/v2/json"
	"github.com/tdewolff/minify/v2/svg"
	"github.com/tdewolff/minify/v2/xml"
)

func LoadMinifiers(m *minify.M) {
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("image/svg+xml", svg.Minify)
	m.AddFuncRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), js.Minify)
	m.AddFuncRegexp(regexp.MustCompile("[/+]json$"), jsonMinify.Minify)
	m.AddFuncRegexp(regexp.MustCompile("[/+]xml$"), xml.Minify)

	m.AddFunc("importmap", jsonMinify.Minify)
	m.AddFunc("speculationrules", jsonMinify.Minify)

	aspMinifier := &html.Minifier{}
	aspMinifier.TemplateDelims = [2]string{"<%", "%>"}
	m.Add("text/asp", aspMinifier)
	m.Add("text/x-ejs-template", aspMinifier)

	phpMinifier := &html.Minifier{}
	phpMinifier.TemplateDelims = [2]string{"<?", "?>"}
	m.Add("application/x-httpd-php", phpMinifier)

	tmplMinifier := &html.Minifier{}
	tmplMinifier.TemplateDelims = [2]string{"{{", "}}"}
	m.Add("text/x-go-template", tmplMinifier)
	m.Add("text/x-mustache-template", tmplMinifier)
	m.Add("text/x-handlebars-template", tmplMinifier)
}

type DiskManifest struct {
	SourceFile      string   `json:"source_file"`
	Algorithm       string   `json:"algorithm"`
	OriginalBytes   int      `json:"original_bytes"`
	MinifiedBytes   int      `json:"minified_bytes"`
	CompressedBytes int      `json:"compressed_bytes"`
	EncodedChars    int      `json:"encoded_chars"`
	TotalDisks      int      `json:"total_disks"`
	RootSHA256      string   `json:"root_sha256"`
	DiskHashes      []string `json:"disk_hashes"`
	FormatVersion   string   `json:"format_version"`
}

func compressPayload(data []byte, algo string) ([]byte, error) {
	var buf bytes.Buffer
	switch algo {
	case "brotli":
		writer := brotli.NewWriterLevel(&buf, brotli.BestCompression)
		if _, err := writer.Write(data); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
	case "deflate":
		writer, err := flate.NewWriter(&buf, flate.BestCompression)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write(data); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
	case "gzip":
		writer, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write(data); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("algoritmo desconhecido: %s (use brotli, deflate ou gzip)", algo)
	}
	return buf.Bytes(), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func main() {
	inputFile := flag.String("file", "", "Caminho para o arquivo HTML de entrada (obrigatorio)")
	algo := flag.String("algo", "brotli", "Algoritmo de compressao: brotli (max), deflate (instant zero-wasm), gzip")
	chunkSize := flag.Int("chunk-size", 1800000, "Tamanho maximo por disquete em caracteres (default: 1800000)")
	outputDir := flag.String("out-dir", "disks", "Diretorio para salvar os disquetes gerados")
	v1Compat := flag.Bool("v1", false, "Emitir formato legado v1 (sem hashes de integridade)")
	flag.Parse()

	if *inputFile == "" {
		log.Fatal("Uso: go run main.go -file <arquivo.html> [-algo brotli|deflate] [-chunk-size 1800000]")
	}

	fmt.Println("==================================================")
	fmt.Println(" FloppyURL v2.0 - Zero-Byte Attested Web Packager ")
	fmt.Println("==================================================")

	m := minify.New()
	LoadMinifiers(m)

	rawContent, err := os.ReadFile(*inputFile)
	if err != nil {
		log.Fatalf("Erro ao abrir arquivo de entrada: %v\n", err)
	}
	originalSize := len(rawContent)
	fmt.Printf("[1/5] Arquivo lido: %s (%d bytes)\n", *inputFile, originalSize)

	var minifiedBuf bytes.Buffer
	if err := m.Minify("text/html", &minifiedBuf, bytes.NewReader(rawContent)); err != nil {
		log.Printf("Aviso na minificacao: %v. Prosseguindo com conteudo original.\n", err)
		minifiedBuf.Reset()
		minifiedBuf.Write(rawContent)
	}
	minifiedBytes := minifiedBuf.Bytes()
	reduction := 0.0
	if originalSize > 0 {
		reduction = 100.0 * (1.0 - float64(len(minifiedBytes))/float64(originalSize))
	}
	fmt.Printf("[2/5] Minificacao concluida: %d bytes (reducao: %.2f%%)\n", len(minifiedBytes), reduction)

	compressedBytes, err := compressPayload(minifiedBytes, *algo)
	if err != nil {
		log.Fatalf("Falha na compressao [%s]: %v\n", *algo, err)
	}
	ratio := 1.0
	if len(compressedBytes) > 0 {
		ratio = float64(originalSize) / float64(len(compressedBytes))
	}
	fmt.Printf("[3/5] Compressao [%s]: %d bytes (taxa: %.2fx menor)\n", *algo, len(compressedBytes), ratio)

	encodedData := base64.RawURLEncoding.EncodeToString(compressedBytes)
	encodedLen := len(encodedData)
	fmt.Printf("[4/5] Base64 RawURL gerado: %d caracteres\n", encodedLen)

	rootHash := sha256Hex([]byte(encodedData))

	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Erro ao criar diretorio de saida: %v\n", err)
	}

	totalDisks := (encodedLen + *chunkSize - 1) / *chunkSize
	if totalDisks == 0 {
		totalDisks = 1
	}

	manifest := DiskManifest{
		SourceFile:      filepath.Base(*inputFile),
		Algorithm:       *algo,
		OriginalBytes:   originalSize,
		MinifiedBytes:   len(minifiedBytes),
		CompressedBytes: len(compressedBytes),
		EncodedChars:    encodedLen,
		TotalDisks:      totalDisks,
		RootSHA256:      rootHash,
		DiskHashes:      make([]string, 0, totalDisks),
		FormatVersion:   "v2",
	}
	if *v1Compat {
		manifest.FormatVersion = "v1"
	}

	fmt.Printf("[5/5] Gerando %d volume(s) RAID-0 (tamanho max por disco: %d bytes)...\n", totalDisks, *chunkSize)

	var firstDiskContent string

	for i := 0; i < totalDisks; i++ {
		diskNum := i + 1
		start := i * *chunkSize
		end := start + *chunkSize
		if end > encodedLen {
			end = encodedLen
		}

		chunkPayload := encodedData[start:end]
		chunkHash := sha256Hex([]byte(chunkPayload))
		manifest.DiskHashes = append(manifest.DiskHashes, chunkHash)

		var formattedDisk string
		if *v1Compat {
			if totalDisks == 1 {
				formattedDisk = chunkPayload
			} else {
				formattedDisk = fmt.Sprintf("v1;[%d/%d]%s", diskNum, totalDisks, chunkPayload)
			}
		} else {
			// Formato v2 autenticado: v2;[algo];[parte/total];[chunk_sha256];[root_sha256];[payload]
			formattedDisk = fmt.Sprintf("v2;%s;[%d/%d];%s;%s;%s",
				*algo, diskNum, totalDisks, chunkHash, rootHash, chunkPayload)
		}

		if diskNum == 1 {
			firstDiskContent = formattedDisk
		}

		diskFilename := filepath.Join(*outputDir, fmt.Sprintf("disk_%02d.txt", diskNum))
		if err := os.WriteFile(diskFilename, []byte(formattedDisk), 0644); err != nil {
			log.Fatalf("Falha ao salvar disco %d: %v\n", diskNum, err)
		}
		fmt.Printf("   -> Gravado: %s (%d chars | SHA256: %.12s...)\n", diskFilename, len(formattedDisk), chunkHash)
	}

	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(filepath.Join(*outputDir, "manifest.json"), manifestJSON, 0644)
	_ = os.WriteFile("base64", []byte(firstDiskContent), 0644)

	fmt.Println("--------------------------------------------------")
	fmt.Printf("CONCLUIDO COM SUCESSO!\n")
	fmt.Printf("Raiz Criptografica SHA-256: %s\n", rootHash)
	if totalDisks == 1 {
		fmt.Printf("URL Direta de Boot:\nhttp://localhost:8080/#%s\n", firstDiskContent)
		_ = os.WriteFile(filepath.Join(*outputDir, "boot_url.txt"), []byte("http://localhost:8080/#"+firstDiskContent), 0644)
	} else {
		fmt.Printf("Payload dividido em %d disquetes na pasta '%s/'.\n", totalDisks, *outputDir)
		fmt.Printf("Para rodar: abra http://localhost:8080/#%s e insira os proximos discos conforme solicitado.\n", firstDiskContent)
	}
	fmt.Println("==================================================")
}
