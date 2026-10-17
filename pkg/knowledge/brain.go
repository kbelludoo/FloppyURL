package knowledge

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	MagicHeader = "GOBRAIN1"
	DefaultDim  = 384
)

// KnowledgeChunk represents an actionable knowledge entry (code, fix, architecture).
type KnowledgeChunk struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Category string    `json:"category"`
	Tags     []string  `json:"tags"`
	Content  string    `json:"content"`
	Vector   []float32 `json:"vector,omitempty"`
}

// SearchResult wraps a matched knowledge chunk with its similarity score.
type SearchResult struct {
	Chunk KnowledgeChunk `json:"chunk"`
	Score float64        `json:"score"` // 0.0 to 1.0 (Cosine similarity)
}

// Brain is the high-performance in-memory vector database running in pure Go.
type Brain struct {
	mu           sync.RWMutex
	chunks       []KnowledgeChunk
	dim          int
	loaded       bool
	onDemandPath string
	totalOnDisk  int
}

var (
	defaultBrain *Brain
	brainOnce    sync.Once
)

func GetBrain() *Brain {
	brainOnce.Do(func() {
		defaultBrain = NewBrain(DefaultDim)
		// Seed with essential built-in Go bot knowledge
		defaultBrain.seedBuiltins()
		// Check for larger compiled brain file on disk (Opção A: Zero-RAM On-Demand)
		home, _ := os.UserHomeDir()
		paths := []string{
			"data/knowledge_brain.bin",
			filepath.Join(home, ".agent-go", "knowledge_brain.bin"),
			"/home/ubuntu/.agent-go/knowledge_brain.bin",
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				defaultBrain.SetOnDemandPath(p)
				break
			}
		}
	})
	return defaultBrain
}

func NewBrain(dim int) *Brain {
	if dim <= 0 {
		dim = DefaultDim
	}
	return &Brain{
		chunks: make([]KnowledgeChunk, 0, 100),
		dim:    dim,
	}
}

// AddChunk registers a knowledge item with its neural vector.
func (b *Brain) AddChunk(chunk KnowledgeChunk) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chunks = append(b.chunks, chunk)
	b.loaded = true
}

// SetOnDemandPath configures the Brain to query a large binary knowledge file directly
// from disk on demand, keeping baseline memory footprint at 0 MB without loading vectors to the heap.
func (b *Brain) SetOnDemandPath(filePath string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onDemandPath = filePath
	b.totalOnDisk = getNumChunksFromHeader(filePath)
	b.loaded = true
}

// OnDemandPath returns the active on-demand binary path if configured.
func (b *Brain) OnDemandPath() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.onDemandPath
}

// TotalChunks returns the total number of knowledge chunks (in-memory + on-demand disk).
func (b *Brain) TotalChunks() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.chunks) + b.totalOnDisk
}

// CosineSimilarity calculates the normalized dot product between two vectors.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// SearchByVector performs Cosine similarity search (using on-demand streaming if configured, or in-memory).
func (b *Brain) SearchByVector(queryVec []float32, topK int) []SearchResult {
	b.mu.RLock()
	onDemand := b.onDemandPath
	b.mu.RUnlock()

	var diskResults []SearchResult
	if onDemand != "" {
		if res, err := SearchOnDemand(onDemand, queryVec, topK); err == nil {
			diskResults = res
		}
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.chunks) == 0 {
		return diskResults
	}

	results := make([]SearchResult, 0, len(b.chunks)+len(diskResults))
	results = append(results, diskResults...)

	for _, chunk := range b.chunks {
		if len(chunk.Vector) == len(queryVec) {
			sim := CosineSimilarity(queryVec, chunk.Vector)
			results = append(results, SearchResult{
				Chunk: chunk,
				Score: sim,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results
}

// SearchSemantic converts any natural language query into a 384-dim dense vector
// in pure Go, performs Cosine Similarity search and blends with keyword boost for hybrid retrieval.
func (b *Brain) SearchSemantic(query string, topK int) []SearchResult {
	if strings.TrimSpace(query) == "" {
		return nil
	}

	b.mu.RLock()
	onDemand := b.onDemandPath
	b.mu.RUnlock()

	var diskResults []SearchResult
	if onDemand != "" {
		if res, err := SearchOnDemandSemantic(onDemand, query, topK); err == nil {
			diskResults = res
		}
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.chunks) == 0 {
		return diskResults
	}

	embedder := GetEmbedder()
	qVec := embedder.Embed(query)
	terms := strings.Fields(strings.ToLower(query))

	results := make([]SearchResult, 0, len(b.chunks)+len(diskResults))
	results = append(results, diskResults...)

	for _, chunk := range b.chunks {
		sim := 0.0
		if len(chunk.Vector) == len(qVec) {
			sim = CosineSimilarity(qVec, chunk.Vector)
		}

		// Hybrid keyword boost
		contentLower := strings.ToLower(chunk.Title + " " + chunk.Content + " " + strings.Join(chunk.Tags, " "))
		keywordBoost := 0.0
		for _, term := range terms {
			if strings.Contains(contentLower, term) {
				keywordBoost += 0.15
			}
			for _, tag := range chunk.Tags {
				if strings.Contains(strings.ToLower(tag), term) {
					keywordBoost += 0.25
				}
			}
		}

		finalScore := (sim * 0.6) + (keywordBoost * 0.4)
		if finalScore > 1.0 {
			finalScore = 1.0
		}

		if finalScore > 0.05 {
			results = append(results, SearchResult{
				Chunk: chunk,
				Score: finalScore,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results
}

// SearchKeyword performs high-speed keyword + tag relevance matching when vector is not passed.
func (b *Brain) SearchKeyword(query string, topK int) []SearchResult {
	b.mu.RLock()
	defer b.mu.RUnlock()

	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil
	}

	results := make([]SearchResult, 0, len(b.chunks))
	for _, c := range b.chunks {
		contentLower := strings.ToLower(c.Title + " " + c.Content + " " + strings.Join(c.Tags, " "))
		score := 0.0
		for _, term := range terms {
			if strings.Contains(contentLower, term) {
				score += 1.0
			}
			for _, tag := range c.Tags {
				if strings.Contains(strings.ToLower(tag), term) {
					score += 2.0
				}
			}
		}
		if score > 0 {
			normScore := score / float64(len(terms)*3)
			if normScore > 1.0 {
				normScore = 1.0
			}
			results = append(results, SearchResult{
				Chunk: c,
				Score: normScore,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results
}

// ExportBinary writes all chunks and vectors into a compact binary file format.
func (b *Brain) ExportBinary(filePath string) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	// 1. Header (Magic + Version + NumChunks + VectorDim)
	if _, err := f.Write([]byte(MagicHeader)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(1)); err != nil { // Version 1
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(len(b.chunks))); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(b.dim)); err != nil {
		return err
	}

	// Helper to write string with 4-byte length prefix
	writeString := func(s string) error {
		bytes := []byte(s)
		if err := binary.Write(f, binary.LittleEndian, uint32(len(bytes))); err != nil {
			return err
		}
		_, err := f.Write(bytes)
		return err
	}

	for _, c := range b.chunks {
		if err := writeString(c.ID); err != nil {
			return err
		}
		if err := writeString(c.Title); err != nil {
			return err
		}
		if err := writeString(c.Category); err != nil {
			return err
		}
		tagsJoined := strings.Join(c.Tags, ",")
		if err := writeString(tagsJoined); err != nil {
			return err
		}
		if err := writeString(c.Content); err != nil {
			return err
		}

		// Vector data
		vecLen := uint16(len(c.Vector))
		if err := binary.Write(f, binary.LittleEndian, vecLen); err != nil {
			return err
		}
		for _, val := range c.Vector {
			if err := binary.Write(f, binary.LittleEndian, val); err != nil {
				return err
			}
		}
	}

	return nil
}

// LoadBinary reads chunks and neural vectors from a compiled binary file.
func (b *Brain) LoadBinary(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	r := bytes.NewReader(data)

	// Magic check
	magic := make([]byte, 8)
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != MagicHeader {
		return fmt.Errorf("invalid brain file magic header")
	}

	var version uint32
	var numChunks uint32
	var dim uint16
	_ = binary.Read(r, binary.LittleEndian, &version)
	_ = binary.Read(r, binary.LittleEndian, &numChunks)
	_ = binary.Read(r, binary.LittleEndian, &dim)

	readString := func() (string, error) {
		var length uint32
		if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
			return "", err
		}
		buf := make([]byte, length)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf), nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.dim = int(dim)
	b.chunks = make([]KnowledgeChunk, 0, numChunks)

	for i := 0; i < int(numChunks); i++ {
		id, err := readString()
		if err != nil {
			break
		}
		title, _ := readString()
		cat, _ := readString()
		tagsJoined, _ := readString()
		content, _ := readString()

		var vecLen uint16
		_ = binary.Read(r, binary.LittleEndian, &vecLen)
		vec := make([]float32, vecLen)
		for v := 0; v < int(vecLen); v++ {
			_ = binary.Read(r, binary.LittleEndian, &vec[v])
		}

		tags := []string{}
		if tagsJoined != "" {
			tags = strings.Split(tagsJoined, ",")
		}

		b.chunks = append(b.chunks, KnowledgeChunk{
			ID:       id,
			Title:    title,
			Category: cat,
			Tags:     tags,
			Content:  content,
			Vector:   vec,
		})
	}

	b.loaded = true
	return nil
}

// seedBuiltins pre-populates specialized Go bot engineering techniques.
func (b *Brain) seedBuiltins() {
	b.chunks = append(b.chunks, []KnowledgeChunk{
		{
			ID:       "go_bot_leak_prevention",
			Title:    "Prevenção de Memory Leaks em Bots Go de Longa Duração",
			Category: "architecture",
			Tags:     []string{"memory", "leak", "http", "transport", "goroutine"},
			Content: `Em bots Go que rodam 24/7 (como Huntera, Baiak, Tibidle), a principal causa de consumo excessivo de RAM e Swap é o fechamento incorreto de conexões HTTP.
Regra de Ouro:
1. Sempre feche resp.Body com 'defer resp.Body.Close()'.
2. Antes de fechar, descarte o restante dos bytes: 'io.Copy(io.Discard, resp.Body)'. Se não fizer isso, o Go mantém o socket TCP aberto esperando mais dados e vaza memória no pool de conexões.
3. Use um http.Transport compartilhado com MaxIdleConnsPerHost ajustado (ex: 20 a 50) e IdleConnTimeout: 90*time.Second.`,
			Vector: make([]float32, DefaultDim),
		},
		{
			ID:       "go_bot_systemd_supervisor",
			Title:    "Padrão de Serviço Systemd com Auto-Heal para Bots",
			Category: "devops",
			Tags:     []string{"systemd", "restart", "linux", "vps"},
			Content: `Para garantir que bots em Go reiniciem sozinhos sem deixar processos zumbis:
[Service]
Type=simple
User=ubuntu
WorkingDirectory=/home/ubuntu/meu-bot
ExecStart=/home/ubuntu/meu-bot/bot-linux
Restart=always
RestartSec=3
KillMode=process
MemoryHigh=80M
MemoryMax=100M

O 'MemoryHigh' faz o Linux desacelerar e forçar o GC do Go antes de dar OOM kill. O 'RestartSec=3' impede loop de restart frenético.`,
			Vector: make([]float32, DefaultDim),
		},
		{
			ID:       "go_bot_websocket_idle_keepalive",
			Title:    "Reconexão Resiliente de WebSockets em Jogos Idle",
			Category: "networking",
			Tags:     []string{"websocket", "reconnect", "tibia", "idle"},
			Content: `Bots que usam WebSockets para servidores de jogos (PokeIdle, PokeIdleWorld, Sigma) caem frequentemente por timeout de proxy reverso (Cloudflare corta WebSockets ociosos a cada 100s).
Solução em Go:
Implemente um Ping/Pong heartbeat com time.NewTicker(30 * time.Second) enviando ping frame. Em caso de erro no conn.WriteMessage, dispare um canal 'reconnectChan' com backoff exponencial (1s, 2s, 4s, até max 30s).`,
			Vector: make([]float32, DefaultDim),
		},
		{
			ID:       "go_bot_gc_tuning",
			Title:    "Tuning do Garbage Collector do Go em VPS de 1GB RAM",
			Category: "performance",
			Tags:     []string{"gogc", "gomemlimit", "ram", "swap"},
			Content: `Para evitar que binários Go disputem RAM com outros bots na VPS de 1GB:
Configure nas variáveis de ambiente do serviço systemd:
GOMEMLIMIT=40MiB
GOGC=80

O GOMEMLIMIT (introduzido no Go 1.19) avisa o runtime para rodar o Garbage Collector de forma agressiva assim que o processo se aproximar de 40MB, eliminando uso desnecessário de Swap.`,
			Vector: make([]float32, DefaultDim),
		},
	}...)
}

type countingReader struct {
	r         io.Reader
	bytesRead int64
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.bytesRead += int64(n)
	return n, err
}

func getNumChunksFromHeader(filePath string) int {
	f, err := os.Open(filePath)
	if err != nil {
		return 0
	}
	defer f.Close()

	header := make([]byte, 18)
	if _, err := io.ReadFull(f, header); err != nil {
		return 0
	}
	if string(header[:8]) != MagicHeader {
		return 0
	}
	numChunks := binary.LittleEndian.Uint32(header[12:16])
	return int(numChunks)
}

type streamCandidate struct {
	offset int64
	score  float64
}

// SearchOnDemand performs high-speed streaming cosine similarity search directly against
// the on-disk binary brain without loading all chunks/vectors into heap memory.
// RAM usage during search: < 1 MB. RAM usage in background: 0 MB.
func SearchOnDemand(filePath string, queryVec []float32, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 5
	}
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cr := &countingReader{r: f}
	br := bufio.NewReaderSize(cr, 128*1024) // 128KB buffer for streaming

	// 1. Read header (18 bytes)
	magic := make([]byte, 8)
	if _, err := io.ReadFull(br, magic); err != nil || string(magic) != MagicHeader {
		return nil, fmt.Errorf("invalid brain file magic header")
	}

	var u4 [4]byte
	var u2 [2]byte

	if _, err := io.ReadFull(br, u4[:]); err != nil {
		return nil, err
	}
	_ = binary.LittleEndian.Uint32(u4[:]) // Version

	if _, err := io.ReadFull(br, u4[:]); err != nil {
		return nil, err
	}
	numChunks := binary.LittleEndian.Uint32(u4[:])

	if _, err := io.ReadFull(br, u2[:]); err != nil {
		return nil, err
	}
	dim := binary.LittleEndian.Uint16(u2[:])

	if int(dim) != len(queryVec) {
		return nil, fmt.Errorf("dimension mismatch: file has %d, query has %d", dim, len(queryVec))
	}

	// Calculate L2 norm of queryVec
	var normQuery float64
	for _, v := range queryVec {
		normQuery += float64(v * v)
	}
	normQuery = math.Sqrt(normQuery)
	if normQuery == 0 {
		normQuery = 1.0
	}

	// Top-K tracking
	candidates := make([]streamCandidate, 0, topK+1)

	// Reusable buffer for vector bytes (dim * 4 bytes)
	vecBytesLen := int(dim) * 4
	vecBuf := make([]byte, vecBytesLen)

	for i := 0; i < int(numChunks); i++ {
		// Exact stream position where this chunk begins
		chunkOffset := cr.bytesRead - int64(br.Buffered())

		// Read lengths and discard string contents without heap allocations
		// ID
		if _, err := io.ReadFull(br, u4[:]); err != nil {
			break
		}
		idLen := int(binary.LittleEndian.Uint32(u4[:]))
		if _, err := br.Discard(idLen); err != nil {
			break
		}

		// Title
		if _, err := io.ReadFull(br, u4[:]); err != nil {
			break
		}
		titleLen := int(binary.LittleEndian.Uint32(u4[:]))
		if _, err := br.Discard(titleLen); err != nil {
			break
		}

		// Category
		if _, err := io.ReadFull(br, u4[:]); err != nil {
			break
		}
		catLen := int(binary.LittleEndian.Uint32(u4[:]))
		if _, err := br.Discard(catLen); err != nil {
			break
		}

		// Tags
		if _, err := io.ReadFull(br, u4[:]); err != nil {
			break
		}
		tagsLen := int(binary.LittleEndian.Uint32(u4[:]))
		if _, err := br.Discard(tagsLen); err != nil {
			break
		}

		// Content
		if _, err := io.ReadFull(br, u4[:]); err != nil {
			break
		}
		contentLen := int(binary.LittleEndian.Uint32(u4[:]))
		if _, err := br.Discard(contentLen); err != nil {
			break
		}

		// Vector length (uint16)
		if _, err := io.ReadFull(br, u2[:]); err != nil {
			break
		}
		chunkVecLen := int(binary.LittleEndian.Uint16(u2[:]))

		if chunkVecLen != int(dim) {
			if _, err := br.Discard(chunkVecLen * 4); err != nil {
				break
			}
			continue
		}

		// Read vector bytes into reusable buffer
		if _, err := io.ReadFull(br, vecBuf); err != nil {
			break
		}

		// Fast Cosine similarity with 4x unrolling
		var dot, normB float64
		k := 0
		for ; k <= int(dim)-4; k += 4 {
			b0 := binary.LittleEndian.Uint32(vecBuf[k*4 : (k+1)*4])
			b1 := binary.LittleEndian.Uint32(vecBuf[(k+1)*4 : (k+2)*4])
			b2 := binary.LittleEndian.Uint32(vecBuf[(k+2)*4 : (k+3)*4])
			b3 := binary.LittleEndian.Uint32(vecBuf[(k+3)*4 : (k+4)*4])

			f0 := float64(math.Float32frombits(b0))
			f1 := float64(math.Float32frombits(b1))
			f2 := float64(math.Float32frombits(b2))
			f3 := float64(math.Float32frombits(b3))

			dot += float64(queryVec[k])*f0 + float64(queryVec[k+1])*f1 + float64(queryVec[k+2])*f2 + float64(queryVec[k+3])*f3
			normB += f0*f0 + f1*f1 + f2*f2 + f3*f3
		}
		for ; k < int(dim); k++ {
			b := binary.LittleEndian.Uint32(vecBuf[k*4 : (k+1)*4])
			f := float64(math.Float32frombits(b))
			dot += float64(queryVec[k]) * f
			normB += f * f
		}

		sim := 0.0
		if normB > 0 {
			sim = dot / (normQuery * math.Sqrt(normB))
		}

		// Keep top-K candidates
		if len(candidates) < topK {
			candidates = append(candidates, streamCandidate{offset: chunkOffset, score: sim})
			sort.Slice(candidates, func(a, b int) bool {
				return candidates[a].score < candidates[b].score
			})
		} else if sim > candidates[0].score {
			candidates[0] = streamCandidate{offset: chunkOffset, score: sim}
			sort.Slice(candidates, func(a, b int) bool {
				return candidates[a].score < candidates[b].score
			})
		}
	}

	// Sort candidates in descending order of score
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Now read ONLY the top-K chunks full data
	results := make([]SearchResult, 0, len(candidates))
	for _, cand := range candidates {
		if cand.score <= 0.0 {
			continue
		}
		chunk, err := readChunkAt(f, cand.offset)
		if err != nil {
			continue
		}
		results = append(results, SearchResult{
			Chunk: chunk,
			Score: cand.score,
		})
	}

	return results, nil
}

// SearchOnDemandSemantic converts a natural language query into a dense vector,
// streams the on-disk binary brain to extract top vector candidates, and applies
// keyword re-ranking for ultra-precise hybrid search.
func SearchOnDemandSemantic(filePath string, query string, topK int) ([]SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	embedder := GetEmbedder()
	qVec := embedder.Embed(query)

	candidatePool := topK * 2
	if candidatePool < 10 {
		candidatePool = 10
	}

	candidates, err := SearchOnDemand(filePath, qVec, candidatePool)
	if err != nil {
		return nil, err
	}

	terms := strings.Fields(strings.ToLower(query))
	for i := range candidates {
		chunk := &candidates[i].Chunk
		contentLower := strings.ToLower(chunk.Title + " " + chunk.Content + " " + strings.Join(chunk.Tags, " "))
		keywordBoost := 0.0
		for _, term := range terms {
			if strings.Contains(contentLower, term) {
				keywordBoost += 0.15
			}
			for _, tag := range chunk.Tags {
				if strings.Contains(strings.ToLower(tag), term) {
					keywordBoost += 0.25
				}
			}
		}

		finalScore := (candidates[i].Score * 0.6) + (keywordBoost * 0.4)
		if finalScore > 1.0 {
			finalScore = 1.0
		}
		candidates[i].Score = finalScore
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if topK > 0 && len(candidates) > topK {
		candidates = candidates[:topK]
	}
	return candidates, nil
}

func readChunkAt(f *os.File, offset int64) (KnowledgeChunk, error) {
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return KnowledgeChunk{}, err
	}
	readString := func() (string, error) {
		var length uint32
		if err := binary.Read(f, binary.LittleEndian, &length); err != nil {
			return "", err
		}
		buf := make([]byte, length)
		if _, err := io.ReadFull(f, buf); err != nil {
			return "", err
		}
		return string(buf), nil
	}

	id, err := readString()
	if err != nil {
		return KnowledgeChunk{}, err
	}
	title, _ := readString()
	cat, _ := readString()
	tagsJoined, _ := readString()
	content, _ := readString()

	tags := []string{}
	if tagsJoined != "" {
		tags = strings.Split(tagsJoined, ",")
	}

	return KnowledgeChunk{
		ID:       id,
		Title:    title,
		Category: cat,
		Tags:     tags,
		Content:  content,
	}, nil
}

