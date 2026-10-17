package navigator

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// InteractiveElement represents a button, link, input or select field on the page.
type InteractiveElement struct {
	Index       int    `json:"index"`       // Unique numeric ID for LLM referencing (#1, #2, etc.)
	Tag         string `json:"tag"`         // "a", "button", "input", "textarea", "select"
	Type        string `json:"type"`        // "text", "password", "submit", "checkbox", "radio", "link", etc.
	Name        string `json:"name"`        // HTML name attribute
	ID          string `json:"id"`          // HTML id attribute
	Text        string `json:"text"`        // Visible inner text or label
	Placeholder string `json:"placeholder"` // Placeholder text for inputs
	Href        string `json:"href"`        // Resolved destination URL for links
	Value       string `json:"value"`       // Current input value
	FormID      string `json:"form_id"`     // Associated form ID if any
}

// PageSnapshot represents a compact, token-optimized semantic representation of a webpage.
type PageSnapshot struct {
	URL        string               `json:"url"`
	Title      string               `json:"title"`
	StatusCode int                  `json:"status_code"`
	CleanText  string               `json:"clean_text"`
	Elements   []InteractiveElement `json:"elements"`
	Forms      map[string]*HTMLForm `json:"forms"`
}

// HTMLForm tracks form fields and submission destination.
type HTMLForm struct {
	ID     string            `json:"id"`
	Action string            `json:"action"`
	Method string            `json:"method"`
	Fields map[string]string `json:"fields"`
}

// ParseHTML parses an HTML document into a token-efficient semantic PageSnapshot.
func ParseHTML(baseURI string, statusCode int, r io.Reader) (*PageSnapshot, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	baseURL, _ := url.Parse(baseURI)

	snap := &PageSnapshot{
		URL:        baseURI,
		StatusCode: statusCode,
		Forms:      make(map[string]*HTMLForm),
	}

	var elementCounter int
	var currentFormID string
	var textBuf strings.Builder

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)

			// Skip non-visible tags
			if tag == "script" || tag == "style" || tag == "noscript" || tag == "svg" || tag == "iframe" {
				return
			}

			if tag == "title" && n.FirstChild != nil {
				snap.Title = strings.TrimSpace(n.FirstChild.Data)
			}

			if tag == "form" {
				formID := getAttr(n, "id")
				if formID == "" {
					formID = fmt.Sprintf("form_%d", len(snap.Forms)+1)
				}
				action := getAttr(n, "action")
				if baseURL != nil && action != "" {
					if actURL, err := baseURL.Parse(action); err == nil {
						action = actURL.String()
					}
				} else if action == "" {
					action = baseURI
				}
				method := strings.ToUpper(getAttr(n, "method"))
				if method == "" {
					method = "GET"
				}

				snap.Forms[formID] = &HTMLForm{
					ID:     formID,
					Action: action,
					Method: method,
					Fields: make(map[string]string),
				}
				prevFormID := currentFormID
				currentFormID = formID
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					traverse(c)
				}
				currentFormID = prevFormID
				return
			}

			// Interactive elements
			isInteractive := false
			var elem InteractiveElement

			switch tag {
			case "a":
				href := getAttr(n, "href")
				if href != "" && !strings.HasPrefix(href, "javascript:") && !strings.HasPrefix(href, "#") {
					resolved := href
					if baseURL != nil {
						if u, err := baseURL.Parse(href); err == nil {
							resolved = u.String()
						}
					}
					text := extractNodeText(n)
					if strings.TrimSpace(text) != "" || getAttr(n, "title") != "" {
						elementCounter++
						elem = InteractiveElement{
							Index: elementCounter,
							Tag:   "a",
							Type:  "link",
							Text:  cleanWhitespace(text),
							Href:  resolved,
							ID:    getAttr(n, "id"),
						}
						isInteractive = true
					}
				}

			case "button":
				elementCounter++
				elemType := getAttr(n, "type")
				if elemType == "" {
					elemType = "button"
				}
				elem = InteractiveElement{
					Index:  elementCounter,
					Tag:    "button",
					Type:   elemType,
					Text:   cleanWhitespace(extractNodeText(n)),
					ID:     getAttr(n, "id"),
					Name:   getAttr(n, "name"),
					FormID: currentFormID,
				}
				isInteractive = true

			case "input":
				inputType := strings.ToLower(getAttr(n, "type"))
				if inputType == "" {
					inputType = "text"
				}
				if inputType != "hidden" {
					elementCounter++
					name := getAttr(n, "name")
					val := getAttr(n, "value")
					elem = InteractiveElement{
						Index:       elementCounter,
						Tag:         "input",
						Type:        inputType,
						Name:        name,
						ID:          getAttr(n, "id"),
						Placeholder: getAttr(n, "placeholder"),
						Value:       val,
						Text:        getAttr(n, "aria-label"),
						FormID:      currentFormID,
					}
					isInteractive = true

					if currentFormID != "" && name != "" {
						if form, ok := snap.Forms[currentFormID]; ok {
							form.Fields[name] = val
						}
					}
				}

			case "textarea":
				elementCounter++
				name := getAttr(n, "name")
				val := extractNodeText(n)
				elem = InteractiveElement{
					Index:       elementCounter,
					Tag:         "textarea",
					Type:        "textarea",
					Name:        name,
					ID:          getAttr(n, "id"),
					Placeholder: getAttr(n, "placeholder"),
					Value:       cleanWhitespace(val),
					FormID:      currentFormID,
				}
				isInteractive = true

			case "select":
				elementCounter++
				elem = InteractiveElement{
					Index:  elementCounter,
					Tag:    "select",
					Type:   "select",
					Name:   getAttr(n, "name"),
					ID:     getAttr(n, "id"),
					FormID: currentFormID,
				}
				isInteractive = true
			}

			if isInteractive {
				snap.Elements = append(snap.Elements, elem)
			}
		}

		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				textBuf.WriteString(t)
				textBuf.WriteString(" ")
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}

	traverse(doc)
	snap.CleanText = cleanWhitespace(textBuf.String())
	return snap, nil
}

// RenderLLMSnapshot converts the snapshot into a token-optimized prompt representation.
func (s *PageSnapshot) RenderLLMSnapshot(maxElements int) string {
	if maxElements <= 0 {
		maxElements = 40
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== PÁGINA ATUAL ===\n"))
	sb.WriteString(fmt.Sprintf("URL: %s\n", s.URL))
	sb.WriteString(fmt.Sprintf("Título: %s (Status HTTP: %d)\n\n", s.Title, s.StatusCode))

	// Truncate clean text to avoid token blowup
	cleanExcerpt := s.CleanText
	if len(cleanExcerpt) > 1500 {
		cleanExcerpt = cleanExcerpt[:1500] + " ... [conteúdo resumido para economia de tokens]"
	}
	sb.WriteString("--- RESUMO DO CONTEÚDO VISÍVEL ---\n")
	sb.WriteString(cleanExcerpt)
	sb.WriteString("\n\n--- ELEMENTOS INTERATIVOS DISPONÍVEIS ---\n")

	if len(s.Elements) == 0 {
		sb.WriteString("(Nenhum elemento interativo encontrado)\n")
		return sb.String()
	}

	count := 0
	for _, el := range s.Elements {
		count++
		if count > maxElements {
			sb.WriteString(fmt.Sprintf("... e mais %d elementos não listados.\n", len(s.Elements)-maxElements))
			break
		}

		switch el.Tag {
		case "a":
			desc := el.Text
			if desc == "" {
				desc = el.Href
			}
			sb.WriteString(fmt.Sprintf("[#%d] LINK: %q -> %s\n", el.Index, desc, el.Href))
		case "button":
			sb.WriteString(fmt.Sprintf("[#%d] BOTÃO (%s): %q\n", el.Index, el.Type, el.Text))
		case "input", "textarea":
			placeholder := ""
			if el.Placeholder != "" {
				placeholder = fmt.Sprintf(" (placeholder: %q)", el.Placeholder)
			}
			val := ""
			if el.Value != "" {
				val = fmt.Sprintf(" [valor atual: %q]", el.Value)
			}
			sb.WriteString(fmt.Sprintf("[#%d] CAMPO (%s: %s)%s%s\n", el.Index, el.Tag, el.Name, placeholder, val))
		case "select":
			sb.WriteString(fmt.Sprintf("[#%d] SELETOR (name: %s)\n", el.Index, el.Name))
		}
	}

	return sb.String()
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func extractNodeText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
			sb.WriteString(" ")
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

func cleanWhitespace(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
