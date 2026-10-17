package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type PushPayload struct {
	ID                     int64    `json:"id"`
	Slug                   string   `json:"slug"`
	NewTitle               string   `json:"newTitle"`
	Text                   string   `json:"text"`
	Language               string   `json:"language"`
	KernelType             string   `json:"kernelType"`
	IsPrivate              bool     `json:"isPrivate"`
	EnableGpu              bool     `json:"enableGpu"`
	EnableTpu              bool     `json:"enableTpu"`
	EnableInternet         bool     `json:"enableInternet"`
	DatasetDataSources     []string `json:"datasetDataSources"`
	CompetitionDataSources []string `json:"competitionDataSources"`
	KernelDataSources      []string `json:"kernelDataSources"`
	ModelDataSources       []string `json:"modelDataSources"`
}

func main() {
	scriptBytes, err := os.ReadFile("kaggle/build_bot_brain.py")
	if err != nil {
		fmt.Printf("Error reading script: %v\n", err)
		os.Exit(1)
	}

	// Remove potential UTF-8 BOM if present
	cleanScript := bytes.TrimPrefix(scriptBytes, []byte("\xef\xbb\xbf"))

	payload := PushPayload{
		ID:                     139287868,
		Slug:                   "go-bot-knowledge-brain",
		NewTitle:               "Go Bot Knowledge Brain",
		Text:                   string(cleanScript),
		Language:               "python",
		KernelType:             "script",
		IsPrivate:              false,
		EnableGpu:              true,
		EnableTpu:              false,
		EnableInternet:         true,
		DatasetDataSources:     []string{},
		CompetitionDataSources: []string{},
		KernelDataSources:      []string{},
		ModelDataSources:       []string{},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Error marshaling json: %v\n", err)
		os.Exit(1)
	}

	req, err := http.NewRequest("POST", "https://www.kaggle.com/api/v1/kernels/push", bytes.NewReader(data))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("kbelludoolord", "911e802b4d1dfaa7faebecc253a4ad99")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("HTTP request error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Response: %s\n", string(body))
}
