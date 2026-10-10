package router

import (
	"regexp"
	"strings"
)

var (
	// Multiple blank lines into a single blank line
	reMultiNewlines = regexp.MustCompile(`\n{3,}`)
	// Trailing whitespaces per line
	reTrailingSpaces = regexp.MustCompile(`[ \t]+\n`)
	// Common verbose git diff / status headers that consume tokens without signal
	reGitIndexHeaders = regexp.MustCompile(`(?m)^index [0-9a-f]{7,}\.\.[0-9a-f]{7,}( [0-9]+)?$`)
	// Strip ANSI escape codes
	reAnsiCodes = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
)

// CompressToolOutput applies lossless token reduction (RTK - Result Token Kompressor)
// to command/tool outputs, removing terminal artifacts, trailing spaces, and redundant whitespace.
func CompressToolOutput(input string) string {
	if len(input) == 0 {
		return input
	}

	// 1. Strip ANSI escape sequences
	out := reAnsiCodes.ReplaceAllString(input, "")

	// 2. Strip redundant git diff index hashes
	out = reGitIndexHeaders.ReplaceAllString(out, "")

	// 3. Trim trailing spaces on lines
	out = reTrailingSpaces.ReplaceAllString(out, "\n")

	// 4. Collapse 3+ consecutive newlines to 2
	out = reMultiNewlines.ReplaceAllString(out, "\n\n")

	// 5. Trim bounding whitespace
	return strings.TrimSpace(out)
}

// CompressMessages iterates through a slice of messages and compresses tool/system/assistant content.
func CompressMessages(msgs []map[string]interface{}) []map[string]interface{} {
	for i := range msgs {
		role, _ := msgs[i]["role"].(string)
		content, ok := msgs[i]["content"].(string)
		if !ok || len(content) == 0 {
			continue
		}

		if role == "tool" || role == "system" {
			msgs[i]["content"] = CompressToolOutput(content)
		}
	}
	return msgs
}
