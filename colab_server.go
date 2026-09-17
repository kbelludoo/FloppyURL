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
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/tdewolff/minify/v2"
	minifyHtml "github.com/tdewolff/minify/v2/html"
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
	m.AddFunc("text/html", minifyHtml.Minify)
}

func enableCORS(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(&w)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("<h1>⚡ PocketWeb Global Search & Packager Online</h1>"))
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

	// 1. Busca Global via Motor Web de Alta Densidade
	endpoint := "https://www.bing.com/search?q=" + url.QueryEscape(query)
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7")

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
		s = html.UnescapeString(s)
		s = strings.ReplaceAll(s, "&nbsp;", " ")
		return strings.TrimSpace(s)
	}

	decodeBingURL := func(raw string) string {
		re := regexp.MustCompile(`(?:&amp;|&)u=a1([^&]+)`)
		m := re.FindStringSubmatch(raw)
		if len(m) > 1 {
			b64 := m[1]
			for len(b64)%4 != 0 {
				b64 += "="
			}
			dec, err := base64.URLEncoding.DecodeString(b64)
			if err == nil && len(dec) > 0 {
				return string(dec)
			}
		}
		return raw
	}

	liRegex := regexp.MustCompile(`<li class="b_algo"[^>]*>([\s\S]*?)</li>`)
	titleRegex := regexp.MustCompile(`<h2[^>]*><a[^>]+href="([^"]+)"[^>]*>([\s\S]*?)</a></h2>`)
	snippetRegex := regexp.MustCompile(`<p[^>]*>([\s\S]*?)</p>`)

	items := liRegex.FindAllStringSubmatch(body, -1)
	var results []SearchItem

	for _, item := range items {
		liHTML := item[1]
		tMatches := titleRegex.FindStringSubmatch(liHTML)
		if len(tMatches) > 2 {
			rawURL := tMatches[1]
			actualURL := decodeBingURL(rawURL)
			title := cleanTags(tMatches[2])

			snippet := ""
			sMatches := snippetRegex.FindStringSubmatch(liHTML)
			if len(sMatches) > 1 {
				snippet = cleanTags(sMatches[1])
			}

			if title != "" && strings.HasPrefix(actualURL, "http") && !strings.Contains(actualURL, "bing.com") {
				results = append(results, SearchItem{
					Title:   title,
					Snippet: snippet,
					URL:     actualURL,
				})
				if len(results) >= 10 {
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
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
		algo = "deflate"
	}

	client := &http.Client{}
	req, _ := http.NewRequest("GET", targetURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"falha ao baixar: %v"}`, err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	rawHTML, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"falha ao ler HTML: %v"}`, err), http.StatusInternalServerError)
		return
	}
	// Injeta base href e interceptor de navegação contínua
	htmlStr := string(rawHTML)
	injection := fmt.Sprintf(`<base href="%s"><script>document.addEventListener('click',function(e){var a=e.target.closest('a');if(a&&a.href&&!a.href.startsWith('javascript:')&&!a.href.startsWith('#')){e.preventDefault();window.parent.postMessage({type:'POCKET_NAV',url:a.href},'*');}});</script>`, targetURL)

	if strings.Contains(strings.ToLower(htmlStr), "<head>") {
		re := regexp.MustCompile(`(?i)<head>`)
		htmlStr = re.ReplaceAllString(htmlStr, "<head>"+injection)
	} else {
		htmlStr = injection + htmlStr
	}
	rawHTML = []byte(htmlStr)

	origSize := len(rawHTML)
	minified, err := m.Bytes("text/html", rawHTML)
	if err != nil {
		minified = rawHTML
	}

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
		algo = "deflate"
		var buf bytes.Buffer
		writer, _ := flate.NewWriter(&buf, flate.BestCompression)
		writer.Write(minified)
		writer.Close()
		compBytes = buf.Bytes()
	}

	compSize := len(compBytes)
	encoded := base64.RawURLEncoding.EncodeToString(compBytes)

	sum := sha256.Sum256([]byte(encoded))
	chunkSHA := hex.EncodeToString(sum[:])

	reduction := float64(origSize-compSize) / float64(origSize) * 100.0
	if reduction < 0 {
		reduction = 0
	}

	bootURL := fmt.Sprintf("https://kbelludoo.github.io/FloppyURL/#v2;%s;[1/1];%s;%s;%s", algo, chunkSHA, chunkSHA, encoded)

	res := PackResponse{
		Status:          "success",
		Engine:          "Go 1.22+ (PocketWeb Global Engine)",
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
	fmt.Printf("🚀 PocketWeb Global Server online na porta %s...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
