package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type KaggleCreds struct {
	Username string `json:"username"`
	Key      string `json:"key"`
}

func main() {
	data, err := os.ReadFile("kaggle.json")
	if err != nil {
		fmt.Printf("Error reading kaggle.json: %v\n", err)
		return
	}

	var creds KaggleCreds
	if err := json.Unmarshal(data, &creds); err != nil {
		fmt.Printf("Error unmarshaling creds: %v\n", err)
		return
	}

	req, err := http.NewRequest("GET", "https://www.kaggle.com/api/v1/kernels/status?userName=kbelludoolord&kernelSlug=go-bot-knowledge-brain", nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}
	req.SetBasicAuth(creds.Username, creds.Key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Printf("Response: %s\n", string(body))
}
