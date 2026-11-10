package router

import (
	"strings"
)

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// StripANSI strips ANSI escape codes using a fast zero-regex single-pass byte scanner.
func StripANSI(input string) string {
	n := len(input)
	if n == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(n)
	i := 0
	for i < n {
		if input[i] == 0x1b && i+1 < n && input[i+1] == '[' {
			i += 2
			for i < n {
				c := input[i]
				i++
				if isAlpha(c) {
					break
				}
			}
		} else {
			b.WriteByte(input[i])
			i++
		}
	}
	return b.String()
}

// isGitIndexHeader checks if a line is a git diff index header: index <hex>..<hex> [mode]
func isGitIndexHeader(line string) bool {
	if !strings.HasPrefix(line, "index ") {
		return false
	}
	i := 6
	n := len(line)
	h1 := 0
	for i < n && isHex(line[i]) {
		h1++
		i++
	}
	if h1 < 7 || i+2 >= n || line[i] != '.' || line[i+1] != '.' {
		return false
	}
	i += 2
	h2 := 0
	for i < n && isHex(line[i]) {
		h2++
		i++
	}
	if h2 < 7 {
		return false
	}
	if i == n {
		return true
	}
	if line[i] == ' ' {
		i++
		for i < n && (line[i] >= '0' && line[i] <= '9') {
			i++
		}
		return i == n
	}
	return false
}

// CompressToolOutput applies lossless token reduction (RTK - Result Token Kompressor)
// to command/tool outputs, removing terminal artifacts, trailing spaces, and redundant whitespace.
func CompressToolOutput(input string) string {
	if len(input) == 0 {
		return ""
	}

	// 1. Strip ANSI escape sequences (linear scanner)
	noAnsi := StripANSI(input)

	// 2. Filter lines, strip git index metadata, and trim trailing whitespace
	lines := strings.Split(noAnsi, "\n")
	var cleaned []string
	for _, l := range lines {
		trimmed := strings.TrimRight(l, " \t\r")
		if !isGitIndexHeader(trimmed) {
			cleaned = append(cleaned, trimmed)
		}
	}
	joined := strings.Join(cleaned, "\n")

	// 3. Collapse 3+ newlines to 2
	var b strings.Builder
	b.Grow(len(joined))
	newlineCount := 0
	for i := 0; i < len(joined); i++ {
		c := joined[i]
		if c == '\n' {
			newlineCount++
			if newlineCount <= 2 {
				b.WriteByte('\n')
			}
		} else {
			newlineCount = 0
			b.WriteByte(c)
		}
	}

	return strings.TrimSpace(b.String())
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
