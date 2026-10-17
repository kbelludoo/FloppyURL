package agent

import (
	"context"
	"encoding/json"
	"fmt"

	opencodesdk "github.com/ethpandaops/opencode-agent-sdk-go"
)

// acpTools exposes the same host functions used by the ordinary agent loop.
// OpenCode calls them through the SDK's authenticated loopback MCP bridge.
func acpTools(defs []ToolDef) ([]opencodesdk.Tool, error) {
	tools := make([]opencodesdk.Tool, 0, len(defs))
	for _, def := range defs {
		var schema map[string]any
		if err := json.Unmarshal(def.Function.Parameters, &schema); err != nil {
			return nil, fmt.Errorf("ACP tool %s schema: %w", def.Function.Name, err)
		}
		name := def.Function.Name
		tools = append(tools, opencodesdk.NewTool(name, def.Function.Description, schema,
			func(ctx context.Context, input map[string]any) (opencodesdk.ToolResult, error) {
				args, err := json.Marshal(input)
				if err != nil {
					return opencodesdk.ToolResult{}, err
				}
				text, err := ExecuteTool(ctx, name, string(args))
				return opencodesdk.ToolResult{Text: text}, err
			}))
	}
	return tools, nil
}
