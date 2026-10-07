package main

import (
	"bytes"
	"compress/flate"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/tdewolff/minify/v2"
	minifyHtml "github.com/tdewolff/minify/v2/html"
)

func TestRealWebsitesLiveSavings(t *testing.T) {
	if testing.Short() {
		t.Skip("Pulando teste de rede ao vivo em modo rapido (-short)")
	}

	// Sites reais com diferentes arquiteturas:
	// 1. Wikipedia: HTML semântico com renderização no servidor (SSR)
	// 2. Hacker News: HTML minimalista clássico
	// 3. Portal Público (Ex: W3C / G1 / Gov): Páginas de texto e notícias
	sites := []struct {
		name string
		url  string
		tipo string
	}{
		{"Wikipedia (Artigo Brasil)", "https://pt.wikipedia.org/wiki/Brasil", "SSR / Wikitext"},
		{"Hacker News (Home)", "https://news.ycombinator.com", "HTML Puro"},
		{"W3C Standards Home", "https://www.w3.org", "Semantic Web"},
	}

	client := &http.Client{Timeout: 15 * time.Second}
	m := minify.New()
	m.AddFunc("text/html", minifyHtml.Minify)

	fmt.Println("\n====================================================================================================")
	fmt.Println("             NAVEGAÇÃO REAL AO VIVO: MEDIÇÃO DE ECONOMIA DO FLOPPYURL                              ")
	fmt.Println("====================================================================================================")
	fmt.Printf("%-26s | %-14s | %-10s | %-10s | %-10s | %-8s\n",
		"Site", "Arquitetura", "Bruto(KB)", "Minif(KB)", "Floppy(KB)", "Economia")
	fmt.Println("----------------------------------------------------------------------------------------------------")

	for _, s := range sites {
		req, err := http.NewRequest("GET", s.url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) PocketWeb-Auditor/2.0")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("%-26s | Falha de conexao: %v\n", s.name, err)
			continue
		}

		rawBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		rawSize := len(rawBytes)

		// 1. Minificacao AST
		var minBuf bytes.Buffer
		if err := m.Minify("text/html", &minBuf, bytes.NewReader(rawBytes)); err != nil {
			minBuf.Reset()
			minBuf.Write(rawBytes)
		}
		minSize := minBuf.Len()

		// 2. Compressao Deflate de Alta Densidade (FloppyURL)
		var compBuf bytes.Buffer
		writer, _ := flate.NewWriter(&compBuf, flate.BestCompression)
		writer.Write(minBuf.Bytes())
		writer.Close()
		floppySize := compBuf.Len()

		economy := (1.0 - float64(floppySize)/float64(rawSize)) * 100.0

		fmt.Printf("%-26s | %-14s | %-10.1f | %-10.1f | %-10.1f | %-6.1f%%\n",
			s.name, s.tipo, float64(rawSize)/1024.0, float64(minSize)/1024.0, float64(floppySize)/1024.0, economy)
	}
	fmt.Println("====================================================================================================")
}
