package main

// LAY: Declarative UI DSL for the FloppyURL family.
//
// LAY is a deterministic, AI-readable layout language in the spirit of LIN:
// minimal surface syntax, domain-specific (UI only), and a compact binary
// bytecode (.laybc) that a tiny browser runtime can execute without WASM.
//
// Source grammar (line-oriented, indentation-sensitive):
//
//   @LAY:1.0
//   VIEW <name>                    // root node
//   STYLE <key>=<val> ...          // inline style block for current node
//   <TAG> ["text"] [key=val ...]   // child node
//   END                            // close current node (optional with indentation)
//
// Tags map to HTML elements: VIEW->div, ROW->div.row, COL->div.col,
// H1,H2,H3,P,PRE,BUTTON,INPUT,SPAN,A,IMG,IFRAME,SECTION.
//
// Binary format (.laybc):
//   magic   4 bytes  "LAY1"
//   version 1 byte    0x01
//   root    2 bytes  u16 index into node table
//   nodes   2 bytes  u16 node count
//   strings 2 bytes  u16 string count
//   [string table: u16 len + bytes, repeated]
//   [node table: 14 bytes each, repeated]
//
// Node record (14 bytes):
//   parent    2  u16
//   tag       1  u8
//   flags     1  u8  (bit0 hasText, bit1 hasStyle, bit2 hasAttrs, bit3 hasAction)
//   text      2  u16 string index
//   style     2  u16 string index
//   attrs     2  u16 string index
//   action    2  u16 string index
//   childCnt  1  u8
//   _pad      1  u8
//
// Deterministic: same source -> identical bytes, always.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

const layMagic = "LAY1"

// Tag IDs
const (
	tagView    = 0
	tagDiv     = 1
	tagRow     = 2
	tagCol     = 3
	tagH1      = 4
	tagH2      = 5
	tagH3      = 6
	tagP       = 7
	tagPre     = 8
	tagButton  = 9
	tagInput   = 10
	tagSpan    = 11
	tagA       = 12
	tagImg     = 13
	tagIframe  = 14
	tagSection = 15
)

var tagIDs = map[string]uint8{
	"VIEW":    tagView,
	"DIV":     tagDiv,
	"ROW":     tagRow,
	"COL":     tagCol,
	"H1":      tagH1,
	"H2":      tagH2,
	"H3":      tagH3,
	"P":       tagP,
	"PRE":     tagPre,
	"BUTTON":  tagButton,
	"INPUT":   tagInput,
	"SPAN":    tagSpan,
	"A":       tagA,
	"IMG":     tagImg,
	"IFRAME":  tagIframe,
	"SECTION": tagSection,
}

type layNode struct {
	parent   int
	tag      uint8
	text     string
	style    string
	attrs    string
	action   string
	childCnt int
}

type layCompiler struct {
	nodes   []layNode
	strings []string
	strMap  map[string]int
}

func newLAYCompiler() *layCompiler {
	return &layCompiler{
		strMap: make(map[string]int),
	}
}

func (c *layCompiler) intern(s string) uint16 {
	if s == "" {
		return 0xFFFF
	}
	idx, ok := c.strMap[s]
	if !ok {
		idx = len(c.strings)
		c.strings = append(c.strings, s)
		c.strMap[s] = idx
	}
	return uint16(idx)
}

// parseLine splits a LAY source line into tokens, respecting quoted strings.
func layTokenize(line string) []string {
	var tokens []string
	var buf strings.Builder
	inStr := false
	flush := func() {
		if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch == '"' {
			inStr = !inStr
			buf.WriteByte(ch)
			continue
		}
		if !inStr && (ch == ' ' || ch == '\t') {
			flush()
			continue
		}
		buf.WriteByte(ch)
	}
	flush()
	return tokens
}

func stripComment(line string) string {
	inStr := false
	for i := 0; i < len(line); i++ {
		if line[i] == '"' {
			inStr = !inStr
		}
		if !inStr && i+1 < len(line) && line[i] == '/' && line[i+1] == '/' {
			return line[:i]
		}
	}
	return line
}

func countIndent(line string) int {
	n := 0
	for _, ch := range line {
		if ch == ' ' || ch == '\t' {
			n++
		} else {
			break
		}
	}
	return n
}

func (c *layCompiler) compile(src string) ([]byte, error) {
	lines := strings.Split(src, "\n")

	// Stack of node indices by indentation level.
	type frame struct {
		idx     int
		indent  int
	}
	var stack []frame
	var rootIdx int = -1

	for lineNo, raw := range lines {
		line := stripComment(raw)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if lineNo == 0 && strings.HasPrefix(trimmed, "@LAY:") {
			continue
		}
		if strings.HasPrefix(trimmed, "@") {
			// skip annotations like @APP{...}
			continue
		}

		indent := countIndent(line)

		kw := strings.ToUpper(strings.TrimSpace(strings.SplitN(trimmed, " ", 2)[0]))

		// END and STYLE have special stack behavior — don't pre-pop for them.
		if kw != "STYLE" && kw != "END" {
			// Pop deeper frames until we're at a parent
			for len(stack) > 0 && indent < stack[len(stack)-1].indent {
				stack = stack[:len(stack)-1]
			}
			// Pop same-level sibling (non-container)
			for len(stack) > 0 && indent == stack[len(stack)-1].indent {
				stack = stack[:len(stack)-1]
			}
		}

		parent := -1
		if len(stack) > 0 {
			parent = stack[len(stack)-1].idx
		}

		tokens := layTokenize(trimmed)
		if len(tokens) == 0 {
			continue
		}

		switch kw {
		case "VIEW":
			node := layNode{parent: parent, tag: tagView}
			idx := c.addNode(node)
			if rootIdx == -1 {
				rootIdx = idx
			}
			if parent >= 0 {
				c.nodes[parent].childCnt++
			}
			stack = append(stack, frame{idx: idx, indent: indent})
		case "STYLE":
			if len(stack) == 0 {
				return nil, fmt.Errorf("line %d: STYLE outside any node", lineNo+1)
			}
			cur := stack[len(stack)-1].idx
			styleParts := tokens[1:]
			c.nodes[cur].style = strings.Join(styleParts, " ")
		case "END":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			tagID, ok := tagIDs[kw]
			if !ok {
				return nil, fmt.Errorf("line %d: unknown tag %q", lineNo+1, tokens[0])
			}
			node := layNode{parent: parent, tag: tagID}
			idx := c.addNode(node)
			if parent >= 0 {
				c.nodes[parent].childCnt++
			}
			c.parseExtra(tokens[1:], idx)
			// Only containers push onto stack; we conservatively push all
			// and pop on next line at same/lower indent.
			stack = append(stack, frame{idx: idx, indent: indent})
		}
	}

	if rootIdx == -1 && len(c.nodes) > 0 {
		rootIdx = 0
	}
	if rootIdx == -1 {
		return nil, fmt.Errorf("no VIEW root node found")
	}

	return c.encode(rootIdx)
}

func (c *layCompiler) addNode(n layNode) int {
	idx := len(c.nodes)
	c.nodes = append(c.nodes, n)
	return idx
}

// parseExtra extracts optional "text" and key=val attributes from tokens.
func (c *layCompiler) parseExtra(tokens []string, idx int) {
	var attrs []string
	for _, tok := range tokens {
		if len(tok) > 0 && tok[0] == '"' {
			// Quoted text content
			inner := tok
			if len(inner) >= 2 && inner[0] == '"' && inner[len(inner)-1] == '"' {
				inner = inner[1 : len(inner)-1]
			}
			c.nodes[idx].text = inner
			continue
		}
		if strings.Contains(tok, "=") {
			eq := strings.IndexByte(tok, '=')
			key := tok[:eq]
			val := tok[eq+1:]
			switch strings.ToUpper(key) {
			case "STYLE":
				c.nodes[idx].style = val
			case "ACTION":
				c.nodes[idx].action = val
			default:
				attrs = append(attrs, tok)
			}
		}
	}
	if len(attrs) > 0 {
		c.nodes[idx].attrs = strings.Join(attrs, " ")
	}
}

func (c *layCompiler) encode(rootIdx int) ([]byte, error) {
	// Pre-intern all node strings so the string table is complete before writing.
	type nodeRef struct {
		text   uint16
		style  uint16
		attrs  uint16
		action uint16
	}
	refs := make([]nodeRef, len(c.nodes))
	for i, n := range c.nodes {
		refs[i].text = c.intern(n.text)
		refs[i].style = c.intern(n.style)
		refs[i].attrs = c.intern(n.attrs)
		refs[i].action = c.intern(n.action)
	}

	var buf bytes.Buffer
	buf.WriteString(layMagic)
	buf.WriteByte(0x01) // version

	binary.Write(&buf, binary.LittleEndian, uint16(rootIdx))
	binary.Write(&buf, binary.LittleEndian, uint16(len(c.nodes)))
	binary.Write(&buf, binary.LittleEndian, uint16(len(c.strings)))

	// String table
	for _, s := range c.strings {
		binary.Write(&buf, binary.LittleEndian, uint16(len(s)))
		buf.WriteString(s)
	}

	// Node table
	for i, n := range c.nodes {
		var flags uint8
		if n.text != "" {
			flags |= 1 << 0
		}
		if n.style != "" {
			flags |= 1 << 1
		}
		if n.attrs != "" {
			flags |= 1 << 2
		}
		if n.action != "" {
			flags |= 1 << 3
		}

		parent := uint16(n.parent)
		if n.parent < 0 {
			parent = 0xFFFF
		}
		binary.Write(&buf, binary.LittleEndian, parent)
		buf.WriteByte(n.tag)
		buf.WriteByte(flags)
		binary.Write(&buf, binary.LittleEndian, refs[i].text)
		binary.Write(&buf, binary.LittleEndian, refs[i].style)
		binary.Write(&buf, binary.LittleEndian, refs[i].attrs)
		binary.Write(&buf, binary.LittleEndian, refs[i].action)
		buf.WriteByte(uint8(n.childCnt))
		buf.WriteByte(0x00) // padding
	}

	return buf.Bytes(), nil
}

// CompileLAY is the public entry point.
func CompileLAY(src []byte) ([]byte, error) {
	c := newLAYCompiler()
	return c.compile(string(src))
}
