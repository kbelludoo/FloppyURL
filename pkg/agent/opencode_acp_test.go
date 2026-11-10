package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestACPToolsExecuteRegisteredHostFunctions(t *testing.T) {
	tools, err := acpTools(AvailableTools)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != len(AvailableTools) {
		t.Fatal("host tools lost")
	}
	path := filepath.Join(t.TempDir(), "fixture.txt")
	if err := os.WriteFile(path, []byte("fixture content"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if tool.Name() == "read_file" {
			result, err := tool.Execute(t.Context(), map[string]any{"path": path})
			if err != nil || result.Text != "fixture content" {
				t.Fatalf("registered read_file failed: %+v %v", result, err)
			}
			return
		}
	}
	t.Fatal("read_file was not registered")
}

func TestACPRejectsInvalidToolSchema(t *testing.T) {
	_, err := acpTools([]ToolDef{{Function: FunctionDef{Name: "bad", Parameters: json.RawMessage(`invalid`)}}})
	if err == nil {
		t.Fatal("invalid schema accepted")
	}
}
