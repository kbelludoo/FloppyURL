package memory

import (
	"context"
	"fmt"
	"strings"
)

// SummarizerFunc defines a callback to generate a summary of older conversation turns.
type SummarizerFunc func(ctx context.Context, conversationText string) (string, error)

// CompactHistory condenses the oldest turns of conversation history when length exceeds maxMessages.
// It preserves system instructions and recent turns, compressing older context into a compact summary block.
func CompactHistory(ctx context.Context, history []any, maxMessages int, summarize SummarizerFunc) ([]any, bool, error) {
	if len(history) <= maxMessages || maxMessages < 6 {
		return history, false, nil
	}

	// Compress the older half of messages, keeping the newest half
	splitIndex := len(history) / 2
	olderTurns := history[:splitIndex]
	recentTurns := history[splitIndex:]

	var sb strings.Builder
	for _, m := range olderTurns {
		if msgMap, ok := m.(map[string]any); ok {
			role, _ := msgMap["role"].(string)
			content, _ := msgMap["content"].(string)
			if content != "" {
				sb.WriteString(fmt.Sprintf("%s: %s\n", role, content))
			}
		}
	}

	if summarize == nil {
		// Fallback: simple truncation
		return recentTurns, true, nil
	}

	summaryPrompt := fmt.Sprintf(
		"Resuma de forma concisa e técnica os principais pontos, decisões, comandos e status desta conversa anterior:\n\n%s",
		sb.String(),
	)

	summary, err := summarize(ctx, summaryPrompt)
	if err != nil {
		return recentTurns, true, err
	}

	summaryMsg := map[string]any{
		"role":    "user",
		"content": fmt.Sprintf("[Resumo do contexto anterior da conversa:\n%s]", strings.TrimSpace(summary)),
	}

	compacted := make([]any, 0, len(recentTurns)+1)
	compacted = append(compacted, summaryMsg)
	compacted = append(compacted, recentTurns...)

	return compacted, true, nil
}
