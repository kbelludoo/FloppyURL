// Microbenchmark of the state machine in pkg/agent/agent.go.
// It deliberately replaces the remote LLM and real tools with deterministic fakes.
// This is NOT the full FloppyURL agent.
package main

import (
    "fmt"
    "os"
    "strconv"
    "strings"
)

type ToolCall struct { ID, Name, Arguments string }
type Message struct {
    Role, Content, ToolCallID string
    ToolCalls []ToolCall
}
type Agent struct {
    History []Message
    MaxRounds int
}
func NewAgent() *Agent { return &Agent{MaxRounds: 20} }

// complete emulates a provider requesting one tool and then returning a final answer.
func complete(messages []Message) (Message, bool) {
    last := messages[len(messages)-1]
    if last.Role == "tool" {
        return Message{Role: "assistant", Content: "feito:" + last.Content}, true
    }
    return Message{Role: "assistant", ToolCalls: []ToolCall{{
        ID: "call_0", Name: "echo", Arguments: last.Content,
    }}}, false
}
func executeTool(call ToolCall) string {
    if call.Name == "echo" { return "ok:" + call.Arguments }
    return "Tool error: unknown tool"
}
func (a *Agent) Run(input string) (string, error) {
    messages := make([]Message, 0, len(a.History)+6)
    messages = append(messages, Message{Role:"system", Content:"You are an AI agent."})
    messages = append(messages, a.History...)
    messages = append(messages, Message{Role:"user", Content:input})
    for round := 0; round < a.MaxRounds; round++ {
        reply, stop := complete(messages)
        messages = append(messages, reply)
        if stop || len(reply.ToolCalls)==0 {
            a.History = append(a.History, Message{Role:"user",Content:input}, reply)
            if len(a.History) > 40 { a.History = a.History[len(a.History)-40:] }
            text := strings.TrimSpace(reply.Content)
            if text == "" { text = "(sem resposta)" }
            return text, nil
        }
        for _, call := range reply.ToolCalls {
            messages = append(messages, Message{
                Role:"tool", Content:executeTool(call), ToolCallID:call.ID,
            })
        }
    }
    return "", fmt.Errorf("agent reached maximum rounds (%d)", a.MaxRounds)
}
func main() {
    n := 250000
    if len(os.Args)>1 {
        v,err:=strconv.Atoi(os.Args[1]);if err!=nil||v<=0 { panic("invalid iterations") }; n=v
    }
    a:=NewAgent()
    var checksum uint64
    for i:=0;i<n;i++ {
        result,err:=a.Run(fmt.Sprintf("pedido-%d",i%100))
        if err!=nil {panic(err)}
        checksum+=uint64(len(result))
    }
    fmt.Printf("language=go iterations=%d checksum=%d history=%d\n",n,checksum,len(a.History))
}
