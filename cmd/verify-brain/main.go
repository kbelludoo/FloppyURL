package main

import (
	"fmt"
	"log"

	"github.com/kbelludoo/FloppyURL/pkg/knowledge"
)

func main() {
	candidates := []string{
		"output/knowledge_brain.bin",
		"/home/ubuntu/.agent-go/knowledge_brain.bin",
		"knowledge_brain.bin",
	}

	brain := knowledge.NewBrain(384)
	loaded := false
	for _, p := range candidates {
		if err := brain.LoadBinary(p); err == nil {
			fmt.Printf("📦 Arquivo carregado de: %s\n", p)
			loaded = true
			break
		}
	}
	if !loaded {
		log.Fatalf("Erro: nenhum arquivo de cérebro encontrado nos caminhos testados")
	}

	fmt.Printf("🧠 CÉREBRO ESPECIALISTA CARREGADO COM SUCESSO!\n")
	fmt.Printf("   Total de chunks indexados: %d\n", brain.TotalChunks())

	// Testando busca 1: Tibia socket latency
	results := brain.SearchKeyword("tibia socket latency", 3)
	fmt.Printf("\n🔍 Teste 1: 'tibia socket latency':\n")
	for i, r := range results {
		fmt.Printf("   [%d] Score: %.4f | ID: %s | Título: %s\n", i+1, r.Score, r.Chunk.ID, r.Chunk.Title)
		content := r.Chunk.Content
		if len(content) > 120 {
			content = content[:120] + "..."
		}
		fmt.Printf("       Trecho: %s\n", content)
	}

	// Testando busca 2: Goroutine leak pprof
	results2 := brain.SearchKeyword("goroutine leak pprof", 3)
	fmt.Printf("\n🔍 Teste 2: 'goroutine leak pprof':\n")
	for i, r := range results2 {
		fmt.Printf("   [%d] Score: %.4f | ID: %s | Título: %s\n", i+1, r.Score, r.Chunk.ID, r.Chunk.Title)
		content := r.Chunk.Content
		if len(content) > 120 {
			content = content[:120] + "..."
		}
		fmt.Printf("       Trecho: %s\n", content)
	}

	// Testando busca 3: Cloudflared RAM tunnel
	results3 := brain.SearchKeyword("cloudflared tunnel ram", 3)
	fmt.Printf("\n🔍 Teste 3: 'cloudflared tunnel ram':\n")
	for i, r := range results3 {
		fmt.Printf("   [%d] Score: %.4f | ID: %s | Título: %s\n", i+1, r.Score, r.Chunk.ID, r.Chunk.Title)
		content := r.Chunk.Content
		if len(content) > 120 {
			content = content[:120] + "..."
		}
		fmt.Printf("       Trecho: %s\n", content)
	}
}
