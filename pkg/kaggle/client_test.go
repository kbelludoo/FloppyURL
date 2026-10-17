package kaggle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKaggleClient_Push_Status_Output(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/kernels/push", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok || u != "testuser" || p != "testkey" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		resp := PushResponse{
			Ref:           "/code/testuser/test-slug",
			URL:           "https://www.kaggle.com/code/testuser/test-slug",
			VersionNumber: 1,
			KernelID:      12345,
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/kernels/status", func(w http.ResponseWriter, r *http.Request) {
		u := r.URL.Query().Get("userName")
		slug := r.URL.Query().Get("kernelSlug")
		if u != "testuser" || slug != "test-slug" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		resp := StatusResponse{
			Status: "complete",
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/kernels/output", func(w http.ResponseWriter, r *http.Request) {
		resp := OutputResponse{
			Files: []OutputFile{
				{FileName: "knowledge_brain.bin", URL: "http://example.com/knowledge_brain.bin", ContentLength: 1024},
			},
			Log: "Success output",
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("testuser", "testkey")
	client.BaseURL = server.URL

	ctx := context.Background()

	// 1. Test Push
	pushResp, err := client.PushKernel(ctx, PushRequest{
		Slug:     "testuser/test-slug",
		NewTitle: "Test Kernel",
		Text:     `{"cells":[]}`,
		Language: "python",
	})
	if err != nil {
		t.Fatalf("PushKernel failed: %v", err)
	}
	if pushResp.KernelID != 12345 {
		t.Errorf("expected kernel ID 12345, got %d", pushResp.KernelID)
	}

	// 2. Test Status
	statusResp, err := client.GetStatus(ctx, "testuser", "test-slug")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if statusResp.Status != "complete" {
		t.Errorf("expected complete, got %s", statusResp.Status)
	}

	// 3. Test Output
	outResp, err := client.GetOutput(ctx, "testuser", "test-slug")
	if err != nil {
		t.Fatalf("GetOutput failed: %v", err)
	}
	if len(outResp.Files) != 1 || outResp.Files[0].FileName != "knowledge_brain.bin" {
		t.Errorf("unexpected files in output: %+v", outResp.Files)
	}
}
