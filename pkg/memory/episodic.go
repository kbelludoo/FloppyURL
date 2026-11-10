package memory

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryRecord represents a single episodic / archival knowledge entry.
type MemoryRecord struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags"`
}

// SearchResult bundles a record with its relevance score.
type SearchResult struct {
	Record MemoryRecord `json:"record"`
	Score  float64      `json:"score"`
}

// EpisodicMemory manages long-term persistence and lexical search over past memories.
type EpisodicMemory struct {
	mu       sync.RWMutex
	filePath string
	Records  []MemoryRecord `json:"records"`
}

var (
	defaultEpisodic *EpisodicMemory
	episodicOnce    sync.Once
	wordRegex       = regexp.MustCompile(`[\p{L}\p{N}]+`)
)

func GetEpisodicMemory() *EpisodicMemory {
	episodicOnce.Do(func() {
		dir := getDefaultMemoryDir()
		defaultEpisodic = NewEpisodicMemory(filepath.Join(dir, "episodic_memory.json"))
	})
	return defaultEpisodic
}

func NewEpisodicMemory(filePath string) *EpisodicMemory {
	em := &EpisodicMemory{
		filePath: filePath,
		Records:  []MemoryRecord{},
	}
	em.load()

	// Seed with initial baseline knowledge if empty
	if len(em.Records) == 0 {
		em.Records = append(em.Records,
			MemoryRecord{
				ID:        "mem_init_01",
				Timestamp: time.Now(),
				Title:     "Solução para falha de porta no PokeIdle",
				Content:   "Quando o bot-poke na porta 3006 cair, reiniciar com 'sudo systemctl restart pokeidle.service'. Aguardar 5 segundos e verificar /api/status.",
				Tags:      []string{"pokeidle", "3006", "systemd", "restart"},
			},
			MemoryRecord{
				ID:        "mem_init_02",
				Timestamp: time.Now(),
				Title:     "Arquitetura de túneis Cloudflare na VPS",
				Content:   "A VPS possui serviços com Cloudflare Tunnel (cloudflared-bot, cloudflared-term, cloudflared-fleet, floppyurl-tunnel). Evitar iniciar processos bash avulsos para não duplicar consumo de RAM.",
				Tags:      []string{"cloudflared", "tunnel", "ram", "vps"},
			},
			MemoryRecord{
				ID:        "mem_init_03",
				Timestamp: time.Now(),
				Title:     "Desativação do OpenCode Serve",
				Content:   "O daemon opencode consumia ~300MB de RAM e retornava 403 FreeTierError para chamadas fora do app. Foi substituído pelo fallback Go puro (Apinex, CodeCraft, TokenHarbor) que usa ~4.5MB.",
				Tags:      []string{"opencode", "ram", "apinex", "freebuff"},
			},
		)
		_ = em.save()
	}

	return em
}

// Save stores a new memory record and persists it to disk.
func (em *EpisodicMemory) Save(title, content string, tags []string) (*MemoryRecord, error) {
	em.mu.Lock()
	defer em.mu.Unlock()

	cleanTags := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(strings.ToLower(t))
		if t != "" {
			cleanTags = append(cleanTags, t)
		}
	}

	rec := MemoryRecord{
		ID:        fmt.Sprintf("mem_%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Title:     strings.TrimSpace(title),
		Content:   strings.TrimSpace(content),
		Tags:      cleanTags,
	}

	em.Records = append(em.Records, rec)
	if err := em.save(); err != nil {
		return nil, err
	}
	return &rec, nil
}

// Search performs a BM25-inspired lexical relevance search over the memory database.
func (em *EpisodicMemory) Search(query string, topK int) []SearchResult {
	em.mu.RLock()
	defer em.mu.RUnlock()

	queryTerms := tokenize(query)
	if len(queryTerms) == 0 || len(em.Records) == 0 {
		return nil
	}

	// Calculate document frequencies
	docFreq := make(map[string]int)
	for _, rec := range em.Records {
		seen := make(map[string]bool)
		text := strings.ToLower(rec.Title + " " + rec.Content + " " + strings.Join(rec.Tags, " "))
		terms := tokenize(text)
		for _, t := range terms {
			if !seen[t] {
				docFreq[t]++
				seen[t] = true
			}
		}
	}

	nDocs := float64(len(em.Records))
	avgDocLen := 0.0
	for _, rec := range em.Records {
		avgDocLen += float64(len(tokenize(rec.Content)))
	}
	avgDocLen /= nDocs
	if avgDocLen == 0 {
		avgDocLen = 1
	}

	const k1 = 1.2
	const b = 0.75

	var results []SearchResult
	for _, rec := range em.Records {
		docTerms := tokenize(strings.ToLower(rec.Title + " " + rec.Content))
		docLen := float64(len(docTerms))

		// Count term frequencies
		tf := make(map[string]float64)
		for _, t := range docTerms {
			tf[t]++
		}

		score := 0.0
		for _, qTerm := range queryTerms {
			freq := tf[qTerm]
			// Bonus for tag matches
			for _, tag := range rec.Tags {
				if tag == qTerm || strings.Contains(tag, qTerm) {
					score += 2.5
				}
			}
			// Bonus for title matches
			if strings.Contains(strings.ToLower(rec.Title), qTerm) {
				score += 3.0
			}

			df := float64(docFreq[qTerm])
			if df > 0 {
				idf := math.Log(1.0 + (nDocs-df+0.5)/(df+0.5))
				numerator := freq * (k1 + 1.0)
				denominator := freq + k1*(1.0-b+b*(docLen/avgDocLen))
				score += idf * (numerator / denominator)
			}
		}

		if score > 0.01 {
			results = append(results, SearchResult{
				Record: rec,
				Score:  score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK <= 0 || topK > len(results) {
		topK = len(results)
	}

	return results[:topK]
}

// ListRecent returns the most recent N memory records.
func (em *EpisodicMemory) ListRecent(limit int) []MemoryRecord {
	em.mu.RLock()
	defer em.mu.RUnlock()

	n := len(em.Records)
	if n == 0 {
		return nil
	}
	if limit <= 0 || limit > n {
		limit = n
	}

	res := make([]MemoryRecord, limit)
	for i := 0; i < limit; i++ {
		res[i] = em.Records[n-1-i]
	}
	return res
}

// Delete removes a record by ID.
func (em *EpisodicMemory) Delete(id string) bool {
	em.mu.Lock()
	defer em.mu.Unlock()

	found := false
	var updated []MemoryRecord
	for _, rec := range em.Records {
		if rec.ID == id {
			found = true
			continue
		}
		updated = append(updated, rec)
	}
	if found {
		em.Records = updated
		_ = em.save()
	}
	return found
}

func tokenize(text string) []string {
	matches := wordRegex.FindAllString(strings.ToLower(text), -1)
	return matches
}

func (em *EpisodicMemory) load() {
	data, err := os.ReadFile(em.filePath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &em.Records)
}

func (em *EpisodicMemory) save() error {
	if em.filePath == "" {
		return nil
	}
	dir := filepath.Dir(em.filePath)
	_ = os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(em.Records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(em.filePath, data, 0644)
}
