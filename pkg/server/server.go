package server

import (
	"bytes"
	"compress/flate"
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
	"os"
	"regexp"
	"strings"

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
	if m == nil {
		m = minify.New()
		m.AddFunc("text/html", minifyHtml.Minify)
	}
}

func EnableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	EnableCORS(w)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("<h1>⚡ PocketWeb Global Search & Packager Online (Go Engine)</h1>"))
}

func SearchHandler(w http.ResponseWriter, r *http.Request) {
	EnableCORS(w)
	if r.Method == http.MethodOptions {
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, `{"error":"parâmetro 'q' obrigatório"}`, http.StatusBadRequest)
		return
	}

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
		return strings.TrimSpace(html.UnescapeString(s))
	}

	var results []SearchItem
	liRegex := regexp.MustCompile(`(?s)<li class="b_algo"[^>]*>(.*?)</li>`)
	matches := liRegex.FindAllStringSubmatch(body, -1)

	for _, match := range matches {
		liHTML := match[1]

		titleRegex := regexp.MustCompile(`<h2><a[^>]+href="([^"]+)"[^>]*>(.*?)</a></h2>`)
		tMatches := titleRegex.FindStringSubmatch(liHTML)

		if len(tMatches) > 2 {
			actualURL := tMatches[1]
			title := cleanTags(tMatches[2])

			snippet := ""
			snippetRegex := regexp.MustCompile(`(?s)<p[^>]*>(.*?)</p>`)
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

func PackHandler(w http.ResponseWriter, r *http.Request) {
	EnableCORS(w)
	if r.Method == http.MethodOptions {
		return
	}

	initMinifier()

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

	var buf bytes.Buffer
	writer, _ := flate.NewWriter(&buf, flate.BestCompression)
	writer.Write(minified)
	writer.Close()
	compBytes := buf.Bytes()
	algo = "deflate"

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
		Engine:          "Go 1.25+ (PocketWeb Global Engine)",
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

// NewHandler creates an http.Handler that routes APIs and serves webDir on root.
func NewHandler(webDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", HealthHandler)
	mux.HandleFunc("/search", SearchHandler)
	mux.HandleFunc("/pack", PackHandler)

	if webDir != "" {
		if _, err := os.Stat(webDir); err == nil {
			fs := http.FileServer(http.Dir(webDir))
			mux.Handle("/", fs)
		}
	}

	return mux
}

// Start runs the server on given addr and serves webDir.
func Start(addr, webDir string) error {
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	initMinifier()
	handler := NewHandler(webDir)
	log.Printf("🚀 PocketWeb & FloppyURL Server online em http://localhost%s (pasta: %s)...\n", addr, webDir)
	return http.ListenAndServe(addr, handler)
}
