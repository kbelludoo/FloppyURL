package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	opencodesdk "github.com/ethpandaops/opencode-agent-sdk-go"
)

// ACPClient delegates each turn to the official CLI over stdio JSON-RPC.
// The CLI owns authentication, provider requests, and the session's tool loop.
type ACPClient struct {
	CLIPath      string
	Cwd          string
	Model        string
	Timeout      time.Duration
	MaxBudgetUSD float64
	options      []opencodesdk.Option
}

// NewACPFromEnv returns nil unless ACP was explicitly selected.
func NewACPFromEnv() (*ACPClient, error) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("OPENCODE_TRANSPORT")), "acp") {
		return nil, nil
	}
	cwd := strings.TrimSpace(os.Getenv("OPENCODE_CWD"))
	if cwd == "" {
		cwd = "."
	}
	absolute, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("OPENCODE_CWD: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("OPENCODE_CWD: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("OPENCODE_CWD must be a directory")
	}
	c := &ACPClient{
		CLIPath: strings.TrimSpace(os.Getenv("OPENCODE_CLI_PATH")),
		Cwd:     absolute,
		Model:   strings.TrimSpace(os.Getenv("OPENCODE_MODEL")),
		Timeout: 2 * time.Minute,
	}
	if c.Model == "" {
		c.Model = "opencode/deepseek-v4.1-flash-free"
	} else if !strings.Contains(c.Model, "/") {
		c.Model = "opencode/" + c.Model
	}
	if value := strings.TrimSpace(os.Getenv("OPENCODE_TIMEOUT")); value != "" {
		c.Timeout, err = time.ParseDuration(value)
		if err != nil || c.Timeout <= 0 {
			return nil, fmt.Errorf("OPENCODE_TIMEOUT must be a positive duration, e.g. 2m")
		}
	}
	if value := strings.TrimSpace(os.Getenv("OPENCODE_MAX_BUDGET_USD")); value != "" {
		c.MaxBudgetUSD, err = strconv.ParseFloat(value, 64)
		if err != nil || !(c.MaxBudgetUSD > 0) || math.IsInf(c.MaxBudgetUSD, 0) {
			return nil, fmt.Errorf("OPENCODE_MAX_BUDGET_USD must be a finite positive number")
		}
	}
	return c, nil
}

func (c *ACPClient) ResolveModel(model string) string {
	model = strings.TrimSpace(model)
	switch model {
	case "", "default", "opencode":
		return c.Model
	case "mimo":
		return "opencode/mimo-v2.6-flash-free"
	}
	if !strings.Contains(model, "/") {
		return "opencode/" + model
	}
	return model
}

func (c *ACPClient) HandlesModel(model string) bool {
	return model == "" || model == "default" || model == "opencode" ||
		model == "mimo" || model == "big-pickle" || model == c.Model ||
		strings.HasPrefix(model, "opencode/") || strings.HasSuffix(model, "-free")
}

// Complete preserves the entire transcript as text context. ACP has no
// per-session OpenAI system-message or function-call response interface.
func (c *ACPClient) Complete(ctx context.Context, model string, messages any, tools ...opencodesdk.Tool) (*opencodesdk.QueryResult, error) {
	transcript, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("encode ACP conversation: %w", err)
	}
	prompt := "Continue the conversation below and answer its final request. " +
		"The JSON contains the complete conversation history, including role labels and tool results.\n\n" + string(transcript)
	return c.Query(ctx, model, prompt, tools...)
}

// Query starts and closes one CLI subprocess per request, so HTTP requests do
// not share sessions or mutable model state. Cancelling ctx stops the turn.
func (c *ACPClient) Query(ctx context.Context, model, prompt string, tools ...opencodesdk.Tool) (*opencodesdk.QueryResult, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("ACP prompt is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	opts := []opencodesdk.Option{
		opencodesdk.WithCwd(c.Cwd),
		opencodesdk.WithModel(c.ResolveModel(model)),
	}
	if c.CLIPath != "" {
		opts = append(opts, opencodesdk.WithCLIPath(c.CLIPath))
	}
	if c.MaxBudgetUSD > 0 {
		opts = append(opts, opencodesdk.WithMaxBudgetUSD(c.MaxBudgetUSD))
	}
	if len(tools) > 0 {
		opts = append(opts, opencodesdk.WithSDKTools(tools...))
	}
	opts = append(opts, c.options...)
	result, err := opencodesdk.Query(ctx, prompt, opts...)
	if err != nil {
		return nil, fmt.Errorf("OpenCode ACP (CLI >= 1.14.20; run opencode auth login): %w", err)
	}
	if result == nil || strings.TrimSpace(result.AssistantText) == "" {
		return nil, fmt.Errorf("OpenCode ACP returned no assistant text")
	}
	return result, nil
}
