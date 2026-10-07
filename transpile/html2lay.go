package transpile

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

var cssMap = map[string]string{
	"background":     "bg",
	"color":          "fg",
	"font-family":    "font",
	"font-size":      "size",
	"padding":        "pad",
	"margin":         "margin",
	"width":          "w",
	"height":         "h",
	"display":        "display",
	"border":         "border",
	"border-radius":  "radius",
	"text-align":     "align",
	"align-items":    "valign",
	"gap":            "gap",
	"flex-direction": "flex",
	"font-weight":    "weight",
	"text-shadow":    "shadow",
	"cursor":         "cursor",
	"opacity":        "opacity",
}

var allowedActions = map[string]bool{
	"mount":     true,
	"clear":     true,
	"increment": true,
}

var (
	reScriptTag = regexp.MustCompile(`(?i)<script[\s>]`)
	reStyleTag  = regexp.MustCompile(`(?i)<style[\s>]`)
)

type layNode struct {
	tag   string
	attrs []string
	style map[string]string
	text  string
	kids  []*layNode
}

// HTMLToLay transpiles HTML-subset into canonical .lay format (fail-closed).
func HTMLToLay(rawHTML string) (string, error) {
	if reScriptTag.MatchString(rawHTML) {
		return "", fmt.Errorf("<script> fora do subset lint-P0")
	}
	if reStyleTag.MatchString(rawHTML) {
		return "", fmt.Errorf("<style> bloco fora do subset lint-P0 (use STYLE inline do subset)")
	}

	z := html.NewTokenizer(strings.NewReader(rawHTML))
	var root *layNode
	var stack []*layNode

	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			err := z.Err()
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("erro no parser HTML: %w", err)
		}

		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			tag := strings.ToLower(tok.Data)

			if tag == "html" || tag == "head" || tag == "body" || tag == "meta" || tag == "title" || tag == "link" {
				continue
			}

			if tag == "script" || tag == "style" || tag == "form" || tag == "table" ||
				tag == "video" || tag == "audio" || tag == "canvas" || tag == "iframe" {
				return "", fmt.Errorf("<%s> fora do subset lint-P0", tag)
			}

			st := make(map[string]string)
			attrsMap := make(map[string]string)
			for _, a := range tok.Attr {
				k := strings.ToLower(a.Key)
				attrsMap[k] = a.Val
			}

			// Style inline
			if rawStyle, ok := attrsMap["style"]; ok {
				delete(attrsMap, "style")
				for _, part := range strings.Split(rawStyle, ";") {
					part = strings.TrimSpace(part)
					if part == "" {
						continue
					}
					if !strings.Contains(part, ":") {
						return "", fmt.Errorf("css invalido: %q", part)
					}
					kv := strings.SplitN(part, ":", 2)
					k := strings.ToLower(strings.TrimSpace(kv[0]))
					v := strings.TrimSpace(kv[1])
					lk, exists := cssMap[k]
					if !exists {
						return "", fmt.Errorf("css %q fora do subset lint-P0", k)
					}
					st[lk] = v
				}
			}

			cls := strings.TrimSpace(attrsMap["class"])
			delete(attrsMap, "class")
			classWords := strings.Fields(cls)

			layTag := "DIV"
			isRow := false
			isCol := false
			for _, w := range classWords {
				if w == "row" {
					isRow = true
				}
				if w == "col" {
					isCol = true
				}
			}

			if tag == "div" && isRow {
				layTag = "ROW"
			} else if tag == "div" && isCol {
				layTag = "COL"
			} else if tag == "div" {
				layTag = "DIV"
			} else {
				switch tag {
				case "h1", "h2", "h3", "p", "pre", "button", "input", "span", "a", "img", "section":
					layTag = strings.ToUpper(tag)
				default:
					return "", fmt.Errorf("<%s> fora do subset lint-P0", tag)
				}
			}

			var rest []string
			if cls != "" && !isRow && !isCol {
				rest = append(rest, "CLASS="+cls)
			}

			for _, k := range []string{"id", "placeholder", "href", "src", "alt"} {
				if v, ok := attrsMap[k]; ok {
					rest = append(rest, fmt.Sprintf("%s=%s", k, v))
					delete(attrsMap, k)
				}
			}

			for k, v := range attrsMap {
				if strings.ToLower(k) == "action" {
					if !allowedActions[v] {
						return "", fmt.Errorf("ACTION=%q desconhecida (mount|clear|increment)", v)
					}
					rest = append(rest, "ACTION="+v)
				} else {
					return "", fmt.Errorf("attr %q fora do subset lint-P0", k)
				}
			}

			node := &layNode{
				tag:   layTag,
				attrs: rest,
				style: st,
			}

			if len(stack) > 0 {
				stack[len(stack)-1].kids = append(stack[len(stack)-1].kids, node)
			} else {
				if root != nil {
					return "", fmt.Errorf("multiplas raizes (P0 exige 1 VIEW)")
				}
				root = node
			}

			if tag != "input" && tag != "img" && tt != html.SelfClosingTagToken {
				stack = append(stack, node)
			}

		case html.EndTagToken:
			tok := z.Token()
			tag := strings.ToLower(tok.Data)
			if tag == "html" || tag == "head" || tag == "body" {
				continue
			}
			if len(stack) > 0 {
				topTag := stack[len(stack)-1].tag
				if topTag == strings.ToUpper(tag) || (tag == "div" && (topTag == "ROW" || topTag == "COL" || topTag == "DIV")) {
					stack = stack[:len(stack)-1]
				}
			}

		case html.TextToken:
			text := strings.TrimSpace(z.Token().Data)
			if text == "" {
				continue
			}
			if len(stack) == 0 {
				return "", fmt.Errorf("texto fora de no: %q", text)
			}
			cur := stack[len(stack)-1]
			if cur.text != "" {
				return "", fmt.Errorf("texto misto/nested fora do subset P0")
			}
			if len(text) > 500 {
				return "", fmt.Errorf("texto >500 chars fora do P0")
			}
			cur.text = text
		}
	}

	if root == nil {
		return "", fmt.Errorf("nenhuma raiz layout encontrada")
	}
	if len(stack) > 0 {
		return "", fmt.Errorf("tags nao fechadas")
	}

	lines := emitNode(root, 0)
	return strings.Join(lines, "\n") + "\n", nil
}

func emitNode(n *layNode, ind int) []string {
	if ind == 0 {
		lines := []string{"@LAY:1.0", "VIEW app"}
		if len(n.style) > 0 {
			var styleKeys []string
			for k := range n.style {
				styleKeys = append(styleKeys, k)
			}
			sort.Strings(styleKeys)
			var parts []string
			for _, k := range styleKeys {
				parts = append(parts, fmt.Sprintf("%s=%s", k, n.style[k]))
			}
			lines = append(lines, "STYLE "+strings.Join(parts, " "))
		}
		for _, kid := range n.kids {
			lines = append(lines, emitNode(kid, 1)...)
		}
		lines = append(lines, "END")
		return lines
	}

	pad := strings.Repeat("    ", ind)
	line := pad + n.tag
	if n.text != "" {
		line += fmt.Sprintf(" %q", n.text)
	}
	if len(n.style) > 0 {
		var styleKeys []string
		for k := range n.style {
			styleKeys = append(styleKeys, k)
		}
		sort.Strings(styleKeys)
		var parts []string
		for _, k := range styleKeys {
			parts = append(parts, fmt.Sprintf("%s=%s", k, n.style[k]))
		}
		line += " STYLE " + strings.Join(parts, " ")
	}
	if len(n.attrs) > 0 {
		sort.Strings(n.attrs)
		line += " " + strings.Join(n.attrs, " ")
	}

	lines := []string{line}
	for _, kid := range n.kids {
		lines = append(lines, emitNode(kid, ind+1)...)
	}
	return lines
}
