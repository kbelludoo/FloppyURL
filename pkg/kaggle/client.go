package kaggle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Client handles interaction with Kaggle's REST API.
type Client struct {
	Username   string
	Key        string
	BaseURL    string
	HTTPClient *http.Client
}

// Credentials matches the structure of ~/.kaggle/kaggle.json
type Credentials struct {
	Username string `json:"username"`
	Key      string `json:"key"`
}

// PushRequest contains parameters for uploading and starting a kernel on Kaggle.
type PushRequest struct {
	Slug           string `json:"slug"`
	NewTitle       string `json:"newTitle"`
	Text           string `json:"text"`
	Language       string `json:"language"`
	KernelType     string `json:"kernelType"`
	IsPrivate      bool   `json:"isPrivate"`
	EnableGpu      bool   `json:"enableGpu"`
	EnableInternet bool   `json:"enableInternet"`
}

// PushResponse represents Kaggle's reply to a kernel push.
type PushResponse struct {
	Ref           string `json:"ref"`
	URL           string `json:"url"`
	VersionNumber int    `json:"versionNumber"`
	Error         string `json:"error"`
	HasError      bool   `json:"hasError"`
	KernelID      int    `json:"kernelId"`
}

// StatusResponse represents kernel execution status.
type StatusResponse struct {
	Status             string `json:"status"` // "queued", "running", "complete", "error"
	FailureMessage     string `json:"failureMessage"`
	HasFailureMessage  bool   `json:"hasFailureMessage"`
}

// OutputFile describes an output artifact from a completed kernel.
type OutputFile struct {
	FileName      string `json:"fileName"`
	URL           string `json:"url"`
	ContentLength int64  `json:"contentLength"`
}

// OutputResponse contains execution logs and output files.
type OutputResponse struct {
	Files []OutputFile `json:"files"`
	Log   string       `json:"log"`
}

// NewClient creates a new Kaggle API client.
func NewClient(username, key string) *Client {
	return &Client{
		Username:   username,
		Key:        key,
		BaseURL:    "https://www.kaggle.com/api/v1",
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// LoadCredentials loads Kaggle credentials from a file or environment variables.
func LoadCredentials(jsonPath string) (*Client, error) {
	// First check environment variables
	userEnv := os.Getenv("KAGGLE_USERNAME")
	keyEnv := os.Getenv("KAGGLE_KEY")
	if userEnv != "" && keyEnv != "" {
		return NewClient(userEnv, keyEnv), nil
	}

	// Then try specified path
	candidatePaths := []string{}
	if jsonPath != "" {
		candidatePaths = append(candidatePaths, jsonPath)
	}

	// Also look in ~/.kaggle/kaggle.json and current directory
	if home, err := os.UserHomeDir(); err == nil {
		candidatePaths = append(candidatePaths, filepath.Join(home, ".kaggle", "kaggle.json"))
	}
	candidatePaths = append(candidatePaths, "kaggle.json")

	for _, p := range candidatePaths {
		data, err := os.ReadFile(p)
		if err == nil {
			var creds Credentials
			if err := json.Unmarshal(data, &creds); err == nil && creds.Username != "" && creds.Key != "" {
				return NewClient(creds.Username, creds.Key), nil
			}
		}
	}

	return nil, fmt.Errorf("kaggle credentials not found in paths or env vars")
}

// PushKernel sends a notebook or script to Kaggle and queues it for execution.
func (c *Client) PushKernel(ctx context.Context, req PushRequest) (*PushResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal push request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/kernels/push", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create push request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.SetBasicAuth(c.Username, c.Key)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute push request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read push response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("push failed with status %d: %s", resp.StatusCode, string(respBytes))
	}

	var pushResp PushResponse
	if err := json.Unmarshal(respBytes, &pushResp); err != nil {
		return nil, fmt.Errorf("unmarshal push response: %w", err)
	}

	if pushResp.HasError || pushResp.Error != "" {
		return &pushResp, fmt.Errorf("kaggle error: %s", pushResp.Error)
	}

	return &pushResp, nil
}

// GetStatus checks the current status of a kernel run.
func (c *Client) GetStatus(ctx context.Context, userName, kernelSlug string) (*StatusResponse, error) {
	url := fmt.Sprintf("%s/kernels/status?userName=%s&kernelSlug=%s", c.BaseURL, userName, kernelSlug)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create status request: %w", err)
	}

	httpReq.SetBasicAuth(c.Username, c.Key)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute status request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read status response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status request failed (%d): %s", resp.StatusCode, string(respBytes))
	}

	var statusResp StatusResponse
	if err := json.Unmarshal(respBytes, &statusResp); err != nil {
		return nil, fmt.Errorf("unmarshal status response: %w", err)
	}

	return &statusResp, nil
}

// GetOutput fetches logs and output files from a completed or running kernel.
func (c *Client) GetOutput(ctx context.Context, userName, kernelSlug string) (*OutputResponse, error) {
	url := fmt.Sprintf("%s/kernels/output?userName=%s&kernelSlug=%s", c.BaseURL, userName, kernelSlug)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create output request: %w", err)
	}

	httpReq.SetBasicAuth(c.Username, c.Key)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute output request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read output response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("output request failed (%d): %s", resp.StatusCode, string(respBytes))
	}

	var outResp OutputResponse
	if err := json.Unmarshal(respBytes, &outResp); err != nil {
		return nil, fmt.Errorf("unmarshal output response: %w", err)
	}

	return &outResp, nil
}

// DownloadFile downloads an artifact from a given direct URL to a local destination file.
func (c *Client) DownloadFile(ctx context.Context, downloadURL, destPath string) error {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create dest file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
