// Package floppy implements the FloppyURL payload pipeline:
// asset inlining, HTML minification, compression, optional AES-GCM
// encryption, Base64URL encoding and RAID-0 style disk chunking with
// SHA-256 attestation.
package floppy

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	jsonMinify "github.com/tdewolff/minify/v2/json"
	"github.com/tdewolff/minify/v2/svg"
	"github.com/tdewolff/minify/v2/xml"
)

// Supported disk format versions.
const (
	FormatV1  = "v1"
	FormatV2  = "v2"
	FormatV2E = "v2e" // v2 + AES-GCM encryption
)

// Supported compression algorithms.
const (
	AlgoBrotli  = "brotli"
	AlgoDeflate = "deflate"
	AlgoGzip    = "gzip"
)

// DefaultChunkSize is the maximum number of Base64 characters per disk
// (~1.8 MB, within common browser URL fragment limits).
const DefaultChunkSize = 1800000

// PBKDF2Iterations is the key-derivation work factor used when a
// passphrase is provided. It must stay in sync with the bootloader
// (WebCrypto deriveKey).
const PBKDF2Iterations = 200000

// ---------------------------------------------------------------------------
// Minifiers
// ---------------------------------------------------------------------------

// LoadMinifiers registers the full minifier suite (HTML, CSS, JS, SVG,
// JSON, XML and common template dialects) on m.
func LoadMinifiers(m *minify.M) {
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("image/svg+xml", svg.Minify)
	m.AddFuncRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), js.Minify)
	m.AddFuncRegexp(regexp.MustCompile(`[/+]json$`), jsonMinify.Minify)
	m.AddFuncRegexp(regexp.MustCompile(`[/+]xml$`), xml.Minify)

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

// MinifyHTML minifies an HTML document (with inlined CSS/JS) returning
// the smallest deterministic representation. On minifier failure it
// returns the original content unchanged.
func MinifyHTML(src []byte) ([]byte, error) {
	m := minify.New()
	LoadMinifiers(m)
	var buf bytes.Buffer
	if err := m.Minify("text/html", &buf, bytes.NewReader(src)); err != nil {
		return src, err
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// Asset inlining
// ---------------------------------------------------------------------------

var (
	cssLinkRe = regexp.MustCompile(`<link\b[^>]*\bhref\s*=\s*("([^"]+)"|'([^']+)')[^>]*>`)
	scriptRe  = regexp.MustCompile(`<script\b([^>]*)\bsrc\s*=\s*("([^"]+)"|'([^']+)')([^>]*)>(\s*)</script>`)
	imgRe     = regexp.MustCompile(`<img\b[^>]*\bsrc\s*=\s*("([^"]+)"|'([^']+)')[^>]*>`)
)

func attrValue(group2, group3 string) string {
	if group2 != "" {
		return group2
	}
	return group3
}

// isLocal reports whether urlRef points at a local file (as opposed to
// absolute URLs, protocol-relative URLs, data:, #fragments, etc).
func isLocal(urlRef string) bool {
	if urlRef == "" {
		return false
	}
	if strings.Contains(urlRef, "://") || strings.HasPrefix(urlRef, "//") ||
		strings.HasPrefix(urlRef, "data:") || strings.HasPrefix(urlRef, "#") ||
		strings.HasPrefix(urlRef, "mailto:") || strings.HasPrefix(urlRef, "javascript:") {
		return false
	}
	return true
}

func readLocal(srcDir, urlRef string) ([]byte, bool) {
	clean := path.Clean("/" + strings.Split(urlRef, "?")[0])
	p := filepath.Join(srcDir, clean)
	if data, err := os.ReadFile(p); err == nil {
		return data, true
	}
	return nil, false
}

var cssMime = map[string]string{
	".css": "text/css",
}

var imageMime = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".svg": "image/svg+xml",
	".ico": "image/x-icon", ".bmp": "image/bmp", ".avif": "image/avif",
}

// InlineAssets replaces local <link rel=stylesheet>, <script src> and
// <img src> references with their inlined equivalents (<style>,
// <script>, data: URIs) so the whole site fits in a single payload.
// References that cannot be resolved on disk are left untouched.
func InlineAssets(src []byte, srcDir string) ([]byte, []string) {
	var notes []string

	out := cssLinkRe.ReplaceAllFunc(src, func(match []byte) []byte {
		g := cssLinkRe.FindSubmatch(match)
		href := attrValue(string(g[2]), string(g[3]))
		if !isLocal(href) || !strings.HasSuffix(strings.ToLower(href), ".css") {
			return match
		}
		data, ok := readLocal(srcDir, href)
		if !ok {
			return match
		}
		notes = append(notes, "inlined css: "+href)
		return append(append([]byte("<style>"), data...), []byte("</style>")...)
	})

	out = scriptRe.ReplaceAllFunc(out, func(match []byte) []byte {
		g := scriptRe.FindSubmatch(match)
		attrsBefore, attrsAfter := string(g[1]), string(g[5])
		if strings.Contains(attrsBefore+attrsAfter, "module") {
			return match // ES modules cannot be inlined meaningfully
		}
		srcRef := attrValue(string(g[3]), string(g[4]))
		if !isLocal(srcRef) || !strings.HasSuffix(strings.ToLower(srcRef), ".js") {
			return match
		}
		data, ok := readLocal(srcDir, srcRef)
		if !ok {
			return match
		}
		notes = append(notes, "inlined js: "+srcRef)
		return append(append([]byte("<script>"), data...), []byte("</script>")...)
	})

	out = imgRe.ReplaceAllFunc(out, func(match []byte) []byte {
		g := imgRe.FindSubmatch(match)
		srcRef := attrValue(string(g[2]), string(g[3]))
		if !isLocal(srcRef) {
			return match
		}
		data, ok := readLocal(srcDir, srcRef)
		if !ok {
			return match
		}
		ext := strings.ToLower(filepath.Ext(srcRef))
		mimeType, known := imageMime[ext]
		if !known {
			return match
		}
		// Detect the real format to avoid serving mismatched MIME types.
		if detected := detectImageMime(data); detected != "" {
			mimeType = detected
		}
		notes = append(notes, "inlined image: "+srcRef)
		uri := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
		return bytes.Replace(match, g[1], []byte(`"`+uri+`"`), 1)
	})

	return out, notes
}

func detectImageMime(data []byte) string {
	switch {
	case len(data) >= 8 && string(data[1:4]) == "PNG":
		return "image/png"
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8:
		return "image/jpeg"
	case len(data) >= 6 && string(data[0:3]) == "GIF":
		return "image/gif"
	case len(data) >= 5 && (bytes.HasPrefix(bytes.TrimSpace(data), []byte("<svg")) ||
		bytes.HasPrefix(bytes.TrimSpace(data), []byte("<?xml"))):
		return "image/svg+xml"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Compression
// ---------------------------------------------------------------------------

// Compress compresses data with the named algorithm at maximum level.
func Compress(data []byte, algo string) ([]byte, error) {
	var buf bytes.Buffer
	switch algo {
	case AlgoBrotli:
		w := brotli.NewWriterLevel(&buf, brotli.BestCompression)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	case AlgoDeflate:
		w, err := flate.NewWriter(&buf, flate.BestCompression)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	case AlgoGzip:
		w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("algoritmo desconhecido: %s (use %s, %s ou %s)", algo, AlgoBrotli, AlgoDeflate, AlgoGzip)
	}
	return buf.Bytes(), nil
}

// Decompress reverses Compress.
func Decompress(data []byte, algo string) ([]byte, error) {
	switch algo {
	case AlgoBrotli:
		return io.ReadAll(brotli.NewReader(bytes.NewReader(data)))
	case AlgoDeflate:
		r := flate.NewReader(bytes.NewReader(data))
		return io.ReadAll(r)
	case AlgoGzip:
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(r)
	default:
		return nil, fmt.Errorf("algoritmo desconhecido: %s", algo)
	}
}

// ---------------------------------------------------------------------------
// Encryption (AES-256-GCM, key derived with PBKDF2-HMAC-SHA256)
// ---------------------------------------------------------------------------

// PBKDF2Key implements PBKDF2 (RFC 8018) with HMAC-SHA256.
func PBKDF2Key(password, salt []byte, iterations, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	var dk []byte
	buf := make([]byte, 4)
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(buf, uint32(block))
		prf.Write(buf)
		u := prf.Sum(nil)

		t := make([]byte, len(u))
		copy(t, u)
		for i := 1; i < iterations; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for j := range t {
				t[j] ^= u[j]
			}
		}
		dk = append(dk, t...)
	}
	return dk[:keyLen]
}

const (
	saltLen = 16
	ivLen   = 12
)

// encrypt seals data with AES-256-GCM. The returned wire format is
// salt(16) || iv(12) || ciphertext+tag.
func encrypt(data []byte, passphrase string, iterations int) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	iv := make([]byte, ivLen)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	key := PBKDF2Key([]byte(passphrase), salt, iterations, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, iv, data, nil)
	out := make([]byte, 0, saltLen+ivLen+len(sealed))
	out = append(out, salt...)
	out = append(out, iv...)
	out = append(out, sealed...)
	return out, nil
}

// decrypt opens a salt||iv||ciphertext wire produced by encrypt.
func decrypt(wire []byte, passphrase string, iterations int) ([]byte, error) {
	if len(wire) < saltLen+ivLen+16 {
		return nil, errors.New("payload criptografado truncado")
	}
	salt := wire[:saltLen]
	iv := wire[saltLen : saltLen+ivLen]
	ct := wire[saltLen+ivLen:]
	key := PBKDF2Key([]byte(passphrase), salt, iterations, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, iv, ct, nil)
}

// ---------------------------------------------------------------------------
// Packaging
// ---------------------------------------------------------------------------

// Options configures Pack.
type Options struct {
	Algo       string // brotli | deflate | gzip (default brotli)
	ChunkSize  int    // max Base64 chars per disk (default DefaultChunkSize)
	V1Compat   bool   // emit legacy v1 disks (no hashes, no encryption)
	Passphrase string // "" disables encryption
	Inline     bool   // inline local CSS/JS/images before minifying
	SourceName string // display name recorded in the manifest
}

// Disk is one volume of the RAID-0 emulated set.
type Disk struct {
	Number    int    // 1-based index
	Total     int    // total disks in the set
	Payload   string // raw Base64URL chunk carried by this disk
	Formatted string // full disk string (headers + payload)
	SHA256    string // SHA-256 hex of Payload
}

// Manifest mirrors manifest.json.
type Manifest struct {
	SourceFile       string   `json:"source_file"`
	Algorithm        string   `json:"algorithm"`
	OriginalBytes    int      `json:"original_bytes"`
	MinifiedBytes    int      `json:"minified_bytes"`
	MinifiedSHA256   string   `json:"minified_sha256"`
	CompressedBytes  int      `json:"compressed_bytes"`
	EncodedChars     int      `json:"encoded_chars"`
	TotalDisks       int      `json:"total_disks"`
	RootSHA256       string   `json:"root_sha256"`
	DiskHashes       []string `json:"disk_hashes"`
	FormatVersion    string   `json:"format_version"`
	Encrypted        bool     `json:"encrypted"`
	PBKDF2Iterations int      `json:"pbkdf2_iterations,omitempty"`
}

// Result is the outcome of Pack.
type Result struct {
	Manifest  Manifest
	Disks     []Disk
	FirstDisk string // formatted content of disk 1 (for direct boot URLs)
}

// SHA256Hex returns the hex-encoded SHA-256 of data.
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// chunk splits s into ceil-sized pieces; the last piece may be shorter
// but is never empty (unless s itself is empty).
func chunk(s string, size int) []string {
	if size <= 0 {
		size = DefaultChunkSize
	}
	n := (len(s) + size - 1) / size
	if n == 0 {
		n = 1
	}
	parts := make([]string, 0, n)
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		parts = append(parts, s[i:end])
	}
	if len(parts) == 0 {
		parts = append(parts, "")
	}
	return parts
}

// Pack runs the full pipeline: inline → minify → compress → (encrypt)
// → Base64URL → chunk → attested disks + manifest.
func Pack(source []byte, srcDir string, opts Options) (*Result, error) {
	if opts.Algo == "" {
		opts.Algo = AlgoBrotli
	}
	if opts.Algo != AlgoBrotli && opts.Algo != AlgoDeflate && opts.Algo != AlgoGzip {
		return nil, fmt.Errorf("algoritmo inválido: %q", opts.Algo)
	}
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = DefaultChunkSize
	}
	if opts.V1Compat && opts.Passphrase != "" {
		return nil, errors.New("formato v1 legado não suporta criptografia")
	}

	srcDir = filepath.Clean(srcDir)

	var notes []string
	if opts.Inline {
		var inlined []byte
		inlined, notes = InlineAssets(source, srcDir)
		source = inlined
	}
	originalSize := len(source)

	minified, err := MinifyHTML(source)
	if err != nil {
		notes = append(notes, "minificação falhou, usando conteúdo original: "+err.Error())
	}

	compressed, err := Compress(minified, opts.Algo)
	if err != nil {
		return nil, err
	}

	format := FormatV2
	payload := compressed
	if opts.Passphrase != "" {
		format = FormatV2E
		if payload, err = encrypt(compressed, opts.Passphrase, PBKDF2Iterations); err != nil {
			return nil, err
		}
	}
	if opts.V1Compat {
		format = FormatV1
	}

	encoded := base64.RawURLEncoding.EncodeToString(payload)
	parts := chunk(encoded, opts.ChunkSize)

	rootHash := SHA256Hex([]byte(encoded))
	disks := make([]Disk, 0, len(parts))
	hashes := make([]string, 0, len(parts))
	for i, part := range parts {
		num := i + 1
		hash := SHA256Hex([]byte(part))
		hashes = append(hashes, hash)

		var formatted string
		switch format {
		case FormatV1:
			if len(parts) == 1 {
				formatted = part
			} else {
				formatted = fmt.Sprintf("v1;[%d/%d]%s", num, len(parts), part)
			}
		case FormatV2E:
			formatted = fmt.Sprintf("v2e;%s;[%d/%d];%s;%s;%s", opts.Algo, num, len(parts), hash, rootHash, part)
		default: // FormatV2
			formatted = fmt.Sprintf("v2;%s;[%d/%d];%s;%s;%s", opts.Algo, num, len(parts), hash, rootHash, part)
		}

		disks = append(disks, Disk{
			Number:    num,
			Total:     len(parts),
			Payload:   part,
			Formatted: formatted,
			SHA256:    hash,
		})
	}

	sourceName := opts.SourceName
	if sourceName == "" {
		sourceName = "index.html"
	}

	manifest := Manifest{
		SourceFile:       filepath.Base(sourceName),
		Algorithm:        opts.Algo,
		OriginalBytes:    originalSize,
		MinifiedBytes:    len(minified),
		MinifiedSHA256:   SHA256Hex(minified),
		CompressedBytes:  len(compressed),
		EncodedChars:     len(encoded),
		TotalDisks:       len(disks),
		RootSHA256:       rootHash,
		DiskHashes:       hashes,
		FormatVersion:    format,
		Encrypted:        opts.Passphrase != "",
		PBKDF2Iterations: 0,
	}
	if opts.Passphrase != "" {
		manifest.PBKDF2Iterations = PBKDF2Iterations
	}

	return &Result{
		Manifest:  manifest,
		Disks:     disks,
		FirstDisk: disks[0].Formatted,
	}, nil
}
