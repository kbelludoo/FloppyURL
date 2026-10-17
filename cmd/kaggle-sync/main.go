package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/kbelludoo/FloppyURL/pkg/kaggle"
)

func main() {
	cmd := flag.String("cmd", "watch", "Command: push, status, watch, download")
	user := flag.String("user", "kbelludoolord", "Kaggle username")
	slug := flag.String("slug", "go-bot-knowledge-brain", "Kernel slug")
	notebookPath := flag.String("notebook", "kaggle/build_bot_brain.ipynb", "Path to notebook file")
	outDir := flag.String("out", "output", "Output directory for downloaded artifacts")
	enableGpu := flag.Bool("gpu", false, "Enable GPU accelerator (warning: GPU queue can take hours)")
	flag.Parse()

	client, err := kaggle.LoadCredentials("kaggle.json")
	if err != nil {
		log.Fatalf("Failed to load Kaggle credentials: %v", err)
	}

	ctx := context.Background()

	switch *cmd {
	case "push":
		fmt.Printf("🚀 Pushing notebook %s to Kaggle (%s/%s)...\n", *notebookPath, *user, *slug)
		nbData, err := os.ReadFile(*notebookPath)
		if err != nil {
			log.Fatalf("Failed to read notebook: %v", err)
		}

		resp, err := client.PushKernel(ctx, kaggle.PushRequest{
			Slug:           fmt.Sprintf("%s/%s", *user, *slug),
			NewTitle:       "Go Bot Knowledge Brain",
			Text:           string(nbData),
			Language:       "python",
			KernelType:     "notebook",
			IsPrivate:      false,
			EnableGpu:      *enableGpu,
			EnableInternet: true,
		})
		if err != nil {
			log.Fatalf("Failed to push kernel: %v", err)
		}
		fmt.Printf("✅ Kernel pushed successfully!\n")
		fmt.Printf("   URL: %s\n", resp.URL)
		fmt.Printf("   Version: %d (ID: %d)\n", resp.VersionNumber, resp.KernelID)

	case "status":
		status, err := client.GetStatus(ctx, *user, *slug)
		if err != nil {
			log.Fatalf("Failed to get status: %v", err)
		}
		fmt.Printf("📊 Status: %s (Failure: %s)\n", status.Status, status.FailureMessage)

	case "watch":
		fmt.Printf("👀 Watching kernel %s/%s...\n", *user, *slug)
		for {
			status, err := client.GetStatus(ctx, *user, *slug)
			if err != nil {
				fmt.Printf("⚠️ Erro consultando status: %v (tentando novamente em 10s)\n", err)
				time.Sleep(10 * time.Second)
				continue
			}

			fmt.Printf("[%s] Status: %s\n", time.Now().Format("15:04:05"), status.Status)

			if status.Status == "complete" {
				fmt.Println("🎉 Kernel finalizado com sucesso!")
				downloadArtifacts(ctx, client, *user, *slug, *outDir)
				return
			}

			if status.Status == "error" || status.Status == "cancelAcknowledged" {
				log.Fatalf("❌ Kernel falhou ou foi cancelado! Mensagem: %s", status.FailureMessage)
			}

			time.Sleep(10 * time.Second)
		}

	case "download":
		downloadArtifacts(ctx, client, *user, *slug, *outDir)

	default:
		log.Fatalf("Comando desconhecido: %s", *cmd)
	}
}

func downloadArtifacts(ctx context.Context, client *kaggle.Client, user, slug, outDir string) {
	fmt.Printf("📥 Buscando arquivos de saída de %s/%s...\n", user, slug)
	out, err := client.GetOutput(ctx, user, slug)
	if err != nil {
		log.Fatalf("Erro ao buscar output: %v", err)
	}

	if len(out.Files) == 0 {
		fmt.Println("⚠️ Nenhum arquivo de saída encontrado ainda.")
		if out.Log != "" {
			fmt.Printf("Logs de execução:\n%s\n", out.Log)
		}
		return
	}

	for _, f := range out.Files {
		dest := fmt.Sprintf("%s/%s", outDir, f.FileName)
		fmt.Printf("⬇️ Baixando %s (%d bytes) para %s...\n", f.FileName, f.ContentLength, dest)
		if err := client.DownloadFile(ctx, f.URL, dest); err != nil {
			log.Printf("Erro baixando %s: %v", f.FileName, err)
		} else {
			fmt.Printf("✅ Salvo com sucesso: %s\n", dest)
		}
	}
}
