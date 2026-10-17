package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	opencodesdk "github.com/ethpandaops/opencode-agent-sdk-go"
)

// A real in-memory JSON-RPC peer exercises the SDK lifecycle without a CLI,
// provider credentials, billable requests, or network access.
type protocolAgent struct {
	acp.Agent
	conn   *acp.AgentSideConnection
	prompt string
	model  string
	cwd    string
	fail   bool
	closed bool
}

func (a *protocolAgent) Initialize(context.Context, acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{ProtocolVersion: acp.ProtocolVersionNumber}, nil
}

func (a *protocolAgent) NewSession(_ context.Context, req acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	a.cwd = req.Cwd
	return acp.NewSessionResponse{SessionId: "ses_test"}, nil
}

func (a *protocolAgent) SetSessionConfigOption(_ context.Context, req acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	a.model = string(req.ValueId.Value)
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (a *protocolAgent) Prompt(ctx context.Context, req acp.PromptRequest) (acp.PromptResponse, error) {
	a.prompt = req.Prompt[0].Text.Text
	if a.fail {
		return acp.PromptResponse{}, errors.New("fixture provider rejected the request")
	}
	for _, text := range []string{"Ol", "á"} {
		if err := a.conn.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: req.SessionId,
			Update: acp.SessionUpdate{AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
				Content: acp.TextBlock(text),
			}},
		}); err != nil {
			return acp.PromptResponse{}, err
		}
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn, Usage: &acp.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 6}}, nil
}

type protocolTransport struct {
	conn      *acp.ClientSideConnection
	pipes     []io.Closer
	closeOnce sync.Once
	agent     *protocolAgent
}

func (t *protocolTransport) Conn() *acp.ClientSideConnection { return t.conn }
func (t *protocolTransport) Close() error {
	t.closeOnce.Do(func() {
		for _, pipe := range t.pipes {
			_ = pipe.Close()
		}
		t.agent.closed = true
	})
	return nil
}

func fixtureACP(t *testing.T, agent *protocolAgent) *ACPClient {
	t.Helper()
	return &ACPClient{Cwd: t.TempDir(), Model: "opencode/test-free", Timeout: 3 * time.Second,
		options: []opencodesdk.Option{opencodesdk.WithTransport(func(_ context.Context, handler acp.Client) (opencodesdk.Transport, error) {
			clientRead, agentWrite := io.Pipe()
			agentRead, clientWrite := io.Pipe()
			agent.conn = acp.NewAgentSideConnection(agent, agentWrite, agentRead)
			return &protocolTransport{conn: acp.NewClientSideConnection(handler, clientWrite, clientRead),
				pipes: []io.Closer{clientRead, clientWrite, agentRead, agentWrite}, agent: agent}, nil
		})}}
}

func TestACPCompletePreservesTranscriptAndModel(t *testing.T) {
	agent := &protocolAgent{}
	client := fixtureACP(t, agent)
	messages := []map[string]string{
		{"role": "system", "content": "Responda em português."},
		{"role": "user", "content": "Meu nome é Ana."},
		{"role": "assistant", "content": "Olá, Ana."},
		{"role": "user", "content": "Qual é meu nome?"},
	}
	result, err := client.Complete(t.Context(), "chosen-free", messages)
	if err != nil {
		t.Fatal(err)
	}
	if result.AssistantText != "Olá" || result.Usage.TotalTokens != 6 || result.SessionID != "ses_test" {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, message := range messages {
		if !strings.Contains(agent.prompt, message["content"]) {
			t.Fatalf("conversation content lost: %s", message["role"])
		}
	}
	if agent.model != "opencode/chosen-free" || agent.cwd != client.Cwd || !agent.closed {
		t.Fatalf("model, cwd or subprocess lifecycle lost: model=%s cwd=%s closed=%v", agent.model, agent.cwd, agent.closed)
	}
}

func TestACPPropagatesProviderErrorAndCloses(t *testing.T) {
	agent := &protocolAgent{fail: true}
	_, err := fixtureACP(t, agent).Query(t.Context(), "default", "hello")
	var rpcErr *opencodesdk.RequestError
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32603 || !agent.closed {
		t.Fatalf("error or lifecycle lost: error=%v closed=%v", err, agent.closed)
	}
	details, _ := json.Marshal(rpcErr.Data)
	if !strings.Contains(string(details), "provider rejected") {
		t.Fatalf("provider error details lost: %s", details)
	}
}

func TestACPRespectsCancellation(t *testing.T) {
	client := fixtureACP(t, &protocolAgent{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Query(ctx, "default", "hello"); err == nil {
		t.Fatal("cancelled turn succeeded")
	}
}

func TestACPRejectsInvalidConfiguration(t *testing.T) {
	t.Setenv("OPENCODE_TRANSPORT", "acp")
	t.Setenv("OPENCODE_CWD", t.TempDir())
	t.Setenv("OPENCODE_TIMEOUT", "0s")
	if _, err := NewACPFromEnv(); err == nil {
		t.Fatal("zero timeout accepted")
	}
	t.Setenv("OPENCODE_TIMEOUT", "1m")
	t.Setenv("OPENCODE_MAX_BUDGET_USD", "NaN")
	if _, err := NewACPFromEnv(); err == nil {
		t.Fatal("NaN budget accepted")
	}
}
