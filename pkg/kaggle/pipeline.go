package kaggle

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kbelludoo/FloppyURL/pkg/knowledge"
)

// PipelineOrchestrator manages the autonomous cycle between VPS error collection,
// Kaggle offloaded GPU/CPU training, and live hot-swapping of the knowledge brain.
type PipelineOrchestrator struct {
	mu           sync.RWMutex
	client       *Client
	user         string
	slug         string
	notebookPath string
	destBrain    string
	isTraining   bool
	lastTrained  time.Time
}

// NewPipelineOrchestrator creates a training orchestrator.
func NewPipelineOrchestrator(client *Client, user, slug, notebookPath, destBrain string) *PipelineOrchestrator {
	return &PipelineOrchestrator{
		client:       client,
		user:         user,
		slug:         slug,
		notebookPath: notebookPath,
		destBrain:    destBrain,
	}
}

// TriggerTraining sends the notebook to Kaggle and starts an asynchronous training job.
func (p *PipelineOrchestrator) TriggerTraining(ctx context.Context, enableGpu bool) error {
	p.mu.Lock()
	if p.isTraining {
		p.mu.Unlock()
		return fmt.Errorf("treinamento no Kaggle ja em andamento")
	}
	p.isTraining = true
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.isTraining = false
		p.mu.Unlock()
	}()

	nbData, err := os.ReadFile(p.notebookPath)
	if err != nil {
		return fmt.Errorf("read notebook: %w", err)
	}

	pushResp, err := p.client.PushKernel(ctx, PushRequest{
		Slug:           fmt.Sprintf("%s/%s", p.user, p.slug),
		NewTitle:       "Go Bot Knowledge Brain",
		Text:           string(nbData),
		Language:       "python",
		KernelType:     "notebook",
		IsPrivate:      false,
		EnableGpu:      enableGpu,
		EnableInternet: true,
	})
	if err != nil {
		return fmt.Errorf("push kernel: %w", err)
	}

	log.Printf("[Pipeline] Kernel enviado para o Kaggle! ID: %d, Versao: %d", pushResp.KernelID, pushResp.VersionNumber)

	// Poll until complete
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			status, err := p.client.GetStatus(ctx, p.user, p.slug)
			if err != nil {
				log.Printf("[Pipeline] Erro checando status: %v", err)
				continue
			}

			log.Printf("[Pipeline] Kaggle Status: %s", status.Status)

			if status.Status == "complete" {
				log.Printf("[Pipeline] Execucao no Kaggle finalizada com sucesso! Baixando artefato...")
				return p.downloadAndHotSwap(ctx)
			}

			if status.Status == "error" || status.Status == "cancelAcknowledged" {
				return fmt.Errorf("execucao no Kaggle falhou: %s", status.FailureMessage)
			}
		}
	}
}

// downloadAndHotSwap downloads knowledge_brain.bin and hot-reloads it into the active in-memory brain.
func (p *PipelineOrchestrator) downloadAndHotSwap(ctx context.Context) error {
	out, err := p.client.GetOutput(ctx, p.user, p.slug)
	if err != nil {
		return fmt.Errorf("get output: %w", err)
	}

	var targetURL string
	for _, f := range out.Files {
		if filepath.Base(f.FileName) == "knowledge_brain.bin" {
			targetURL = f.URL
			break
		}
	}

	if targetURL == "" {
		return fmt.Errorf("arquivo knowledge_brain.bin nao encontrado nos outputs do Kaggle")
	}

	tmpDest := p.destBrain + ".tmp"
	if err := p.client.DownloadFile(ctx, targetURL, tmpDest); err != nil {
		return fmt.Errorf("download file: %w", err)
	}

	// Rename atomically
	if err := os.Rename(tmpDest, p.destBrain); err != nil {
		return fmt.Errorf("atomic rename: %w", err)
	}

	// Hot-swap in running brain instance
	brain := knowledge.GetBrain()
	if err := brain.LoadBinary(p.destBrain); err != nil {
		return fmt.Errorf("hot reload binary: %w", err)
	}

	p.mu.Lock()
	p.lastTrained = time.Now()
	p.mu.Unlock()

	log.Printf("[Pipeline] 🧠 Cerebro de Conhecimento atualizado com sucesso! Chunks ativos: %d", brain.TotalChunks())
	return nil
}
