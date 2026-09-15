// Package lint — compilador lint-P0 importavel e 100% em memoria.
//
// DSL declarativa de UI irma do LIN: sintaxe minima orientada a layout,
// bytecode proprio compacto (.laybc), deterministica, IA-legivel.
// Source canonico (line-oriented):
//
//	@LAY:1.0
//	VIEW <name>                    // no raiz
//	STYLE <key>=<val> ...          // bloco de estilo do no corrente
//	<TAG> ["text"] [key=val ...]   // no filho
//	END
//
// Tags: VIEW->div, ROW->div.row, COL->div.col, H1,H2,H3,P,PRE,BUTTON,INPUT,
// SPAN,A,IMG,IFRAME,SECTION.
//
// Binario (.laybc): magic 4B "LAY1" + versao 1B 0x01 + root u16 + nos u16 +
// strings u16 + [tabela strings: u16 len + bytes] + [tabela nos: 14B cada].
// No (14B): parent u16 + tag u8 + flags u8 + text u16 + style u16 + attrs u16 +
// action u16 + childCnt u8 + pad u8. Mesma entrada -> bytes identicos, sempre.
//
// Seguranca estilo LIN: SHA-256 e envelope v2 vivem nas irmas linp/linz;
// este pacote nao toca disco nem rede (puro []byte -> []byte).
package lint

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

const Magic = "LAY1"
const Version = 0x01

// Tag IDs
const (
	TagView    = 0
	TagDiv     = 1
	TagRow     = 2
	TagCol     = 3
	TagH1      = 4
	TagH2      = 5
	TagH3      = 6
	TagP       = 7
	TagPre     = 8
	TagButton  = 9
	TagInput   = 10
	TagSpan    = 11
	TagA       = 12
	TagImg     = 13
	TagIframe  = 14
	TagSection = 15
)

var tagIDs = map[string]uint8{
	"VIEW":    TagView,
	"DIV":     TagDiv,
	"ROW":     TagRow,
	"COL":     TagCol,
	"H1":      TagH1,
	"H2":      TagH2,
	"H3":      TagH3,
	"P":       TagP,
	"PRE":     TagPre,
	"BUTTON":  TagButton,
	"INPUT":   TagInput,
	"SPAN":    TagSpan,
	"A":       TagA,
	"IMG":     TagImg,
	"IFRAME":  TagIframe,
	"SECTION": TagSection,
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

// layTokenize splits a LAY source line into tokens, respecting quoted strings.
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
		idx    int
		indent int
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
			node := layNode{parent: parent, tag: TagView}
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
	buf.WriteString(Magic)
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

// Compile is the public entry point: .lay source -> .laybc, pure in-memory.
func Compile(src []byte) ([]byte, error) {
	c := newLAYCompiler()
	return c.compile(string(src))
}

// Stats re-reads .laybc (fail-closed) and returns root, node count, string count.
func Stats(bc []byte) (root, nnodes, nstr int, err error) {
	if len(bc) < 4+1+2+2+2 {
		return 0, 0, 0, fmt.Errorf("bytecode truncado")
	}
	if string(bc[:4]) != Magic {
		return 0, 0, 0, fmt.Errorf("magic invalido")
	}
	if bc[4] != Version {
		return 0, 0, 0, fmt.Errorf("versao desconhecida")
	}
	root = int(binary.LittleEndian.Uint16(bc[5:]))
	nnodes = int(binary.LittleEndian.Uint16(bc[7:]))
	nstr = int(binary.LittleEndian.Uint16(bc[9:]))
	off := 11
	for i := 0; i < nstr; i++ {
		if off+2 > len(bc) {
			return 0, 0, 0, fmt.Errorf("string table truncada")
		}
		ln := int(binary.LittleEndian.Uint16(bc[off:]))
		off += 2 + ln
		if off > len(bc) {
			return 0, 0, 0, fmt.Errorf("string truncada")
		}
	}
	if off+nnodes*14 != len(bc) {
		return 0, 0, 0, fmt.Errorf("tabela de nos inconsistente")
	}
	return root, nnodes, nstr, nil
}
