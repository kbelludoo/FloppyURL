package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// CircuitBreaker tracks provider failures (429, 403, 503) and prevents wasteful retries.
type CircuitBreaker struct {
	mu        sync.RWMutex
	cooldowns map[string]time.Time
}

var defaultCircuitBreaker = &CircuitBreaker{
	cooldowns: make(map[string]time.Time),
}

// IsAvailable checks if the provider is currently eligible for traffic.
func (cb *CircuitBreaker) IsAvailable(providerName string) bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	until, exists := cb.cooldowns[providerName]
	if !exists {
		return true
	}
	return time.Now().After(until)
}

// RecordFailure places a provider in cooldown for a given duration.
func (cb *CircuitBreaker) RecordFailure(providerName string, duration time.Duration) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.cooldowns[providerName] = time.Now().Add(duration)
}

// RecordSuccess clears any failure records for the provider.
func (cb *CircuitBreaker) RecordSuccess(providerName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.cooldowns, providerName)
}

// Hermes Tool Call Fallback Parser
var (
	toolCallXMLRegex = regexp.MustCompile(`(?s)<tool_call>\s*(\{.*?\})\s*</tool_call>`)
	jsonCodeBlockRegex = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{\\s*\"(?:name|tool|function)\".*?\\})\\s*```")
)

// ParseHermesToolCalls extracts function calls if the LLM outputted them as text or XML instead of API tool_calls.
func ParseHermesToolCalls(content string) []ToolCall {
	var toolCalls []ToolCall

	// 1. Check for XML format: <tool_call>{"name":"...","arguments":{...}}</tool_call>
	xmlMatches := toolCallXMLRegex.FindAllStringSubmatch(content, -1)
	for i, match := range xmlMatches {
		if len(match) > 1 {
			if tc := parseSingleToolCallJSON([]byte(match[1]), fmt.Sprintf("call_xml_%d", i)); tc != nil {
				toolCalls = append(toolCalls, *tc)
			}
		}
	}
	if len(toolCalls) > 0 {
		return toolCalls
	}

	// 2. Check for markdown code blocks: ```json {"name": "...", "arguments": {...}} ```
	codeMatches := jsonCodeBlockRegex.FindAllStringSubmatch(content, -1)
	for i, match := range codeMatches {
		if len(match) > 1 {
			if tc := parseSingleToolCallJSON([]byte(match[1]), fmt.Sprintf("call_code_%d", i)); tc != nil {
				toolCalls = append(toolCalls, *tc)
			}
		}
	}
	if len(toolCalls) > 0 {
		return toolCalls
	}

	// 3. Check if the entire content is a bare JSON tool call object
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		if tc := parseSingleToolCallJSON([]byte(trimmed), "call_raw_0"); tc != nil {
			toolCalls = append(toolCalls, *tc)
		}
	}

	return toolCalls
}

func parseSingleToolCallJSON(data []byte, id string) *ToolCall {
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil
	}

	name := ""
	if n, ok := generic["name"].(string); ok {
		name = n
	} else if n, ok := generic["tool"].(string); ok {
		name = n
	} else if n, ok := generic["function"].(string); ok {
		name = n
	}

	if name == "" {
		return nil
	}

	// Extract arguments
	argsStr := "{}"
	if a, ok := generic["arguments"]; ok {
		if s, ok := a.(string); ok {
			argsStr = s
		} else if m, err := json.Marshal(a); err == nil {
			argsStr = string(m)
		}
	} else if p, ok := generic["parameters"]; ok {
		if s, ok := p.(string); ok {
			argsStr = s
		} else if m, err := json.Marshal(p); err == nil {
			argsStr = string(m)
		}
	}

	tc := &ToolCall{
		ID:   id,
		Type: "function",
	}
	tc.Function.Name = name
	tc.Function.Arguments = argsStr
	return tc
}
