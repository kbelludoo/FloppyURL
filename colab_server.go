package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
)

type PackResponse struct {
	Status          string  `json:"status"`
	Engine          string  `json:"engine"`
	Algorithm       string  `json:"algorithm"`
	OriginalURL     string  `json:"original_url"`
	OriginalBytes   int     `json:"original_bytes"`
	CompressedBytes int     `json:"compressed_bytes"`
	EncodedChars    int     `json:"encoded_chars"`
	ReductionPct    float64 `json:"reduction_pct"`
	SHA256          string  `json:"sha256"`
	BootURL         string  `json:"boot_url"`
}

type SearchItem struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

var m *minify.M

func initMinifier() {
	m = minify.New()
	m.AddFunc("text/html", html.Minify)
}

func enableCORS(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(&w)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("<h1>⚡ PocketWeb Cloud Search & Packager Online</h1>"))
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(&w)
	if r.Method == http.MethodOptions {
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, `{"error":"parâmetro 'q' obrigatório"}`, http.StatusBadRequest)
		return
	}

	data := url.Values{}
	data.Set("q", query)

	req, err := http.NewRequest("POST", "https://lite.duckduckgo.com/lite/", strings.NewReader(data.Encode()))
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	body := string(bodyBytes)

	cleanTags := func(s string) string {
		re := regexp.MustCompile(`<[^>]*>`)
		s = re.ReplaceAllString(s, "")
		s = strings.ReplaceAll(s, "&nbsp;", " ")
		s = strings.ReplaceAll(s, "&amp;", "&")
		s = strings.ReplaceAll(s, "&quot;", "\"")
		return strings.TrimSpace(s)
	}

	trRegex := regexp.MustCompile(`<tr>([\s\S]*?)</tr>`)
	aRegex := regexp.MustCompile(`<a[^>]+href="([^"]+)"[^>]*>([\s\S]*?)</a>`)

	trs := trRegex.FindAllStringSubmatch(body, -1)
	var items []SearchItem

	for i := 0; i < len(trs); i++ {
		rowHTML := trs[i][1]
		aMatches := aRegex.FindStringSubmatch(rowHTML)
		if len(aMatches) > 2 {
			rawHref := aMatches[1]
			title := cleanTags(aMatches[2])

			if !strings.Contains(rawHref, "duckduckgo.com") && (strings.HasPrefix(rawHref, "http://") || strings.HasPrefix(rawHref, "https://")) {
				snippet := ""
				if i+1 < len(trs) {
					snippet = cleanTags(trs[i+1][1])
				}

				items = append(items, SearchItem{
					Title:   title,
					Snippet: snippet,
					URL:     rawHref,
				})
				if len(items) >= 10 {
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func packHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(&w)
	if r.Method == http.MethodOptions {
		return
	}

	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		var reqBody map[string]string
		if json.NewDecoder(r.Body).Decode(&reqBody) == nil {
			targetURL = reqBody["url"]
		}
	}

	if targetURL == "" {
		http.Error(w, `{"error":"parâmetro 'url' obrigatório"}`, http.StatusBadRequest)
		return
	}

	algo := r.URL.Query().Get("algo")
	if algo == "" {
		algo = "deflate" // Padrão: 0-byte native decompression no navegador
	}

	// 1. Download do site alvo
	client := &http.Client{}
	req, _ := http.NewRequest("GET", targetURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"falha ao baixar alvo: %v"}`, err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	rawHTML, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"falha ao ler HTML: %v"}`, err), http.StatusInternalServerError)
		return
	}
	origSize := len(rawHTML)

	// 2. Minificação AST de alta densidade
	minified, err := m.Bytes("text/html", rawHTML)
	if err != nil {
		minified = rawHTML
	}

	// 3. Compressão
	var compBytes []byte
	if algo == "brotli" {
		var buf bytes.Buffer
		writer := brotli.NewWriterOptions(&buf, brotli.WriterOptions{Quality: 11})
		writer.Write(minified)
		writer.Close()
		compBytes = buf.Bytes()
	} else if algo == "gzip" {
		var buf bytes.Buffer
		writer, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		writer.Write(minified)
		writer.Close()
		compBytes = buf.Bytes()
	} else {
		// Deflate raw (padrão zero-wasm)
		algo = "deflate"
		var buf bytes.Buffer
		writer, _ := flate.NewWriter(&buf, flate.BestCompression)
		writer.Write(minified)
		writer.Close()
		compBytes = buf.Bytes()
	}

	compSize := len(compBytes)
	encoded := base64.RawURLEncoding.EncodeToString(compBytes)

	// SHA-256 do texto Base64
	sum := sha256.Sum256([]byte(encoded))
	chunkSHA := hex.EncodeToString(sum[:])

	reduction := float64(origSize-compSize) / float64(origSize) * 100.0
	if reduction < 0 {
		reduction = 0
	}

	bootURL := fmt.Sprintf("https://kbelludoo.github.io/FloppyURL/#v2;%s;[1/1];%s;%s;%s", algo, chunkSHA, chunkSHA, encoded)

	res := PackResponse{
		Status:          "success",
		Engine:          "Go 1.22+ (PocketWeb High-Concurrency)",
		Algorithm:       algo,
		OriginalURL:     targetURL,
		OriginalBytes:   origSize,
		CompressedBytes: compSize,
		EncodedChars:    len(encoded),
		ReductionPct:    reduction,
		SHA256:          chunkSHA,
		BootURL:         bootURL,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func main() {
	initMinifier()
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/search", searchHandler)
	http.HandleFunc("/pack", packHandler)

	port := "8080"
	fmt.Printf("🚀 PocketWeb Server rodando na porta %s...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
