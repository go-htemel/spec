package spec

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// NameType describes the specification name
type NameType string

const (
	HTML NameType = "HTML"
	SVG  NameType = "SVG"
)

// Parser holds the state for document parsing.
type Parser struct {
	active      bool
	currElement *Element
	descParsed  bool
	Spec        *Spec
}

// NewSpecParser returns an instantiated parser for document parsing.
func NewSpecParser(name NameType) *Parser {
	return &Parser{
		Spec: &Spec{
			Name: string(name),
		},
	}
}

// Activate enables the parser to begin collecting data for an element.
func (p *Parser) Activate(element string) {
	p.active = true
	p.currElement = &Element{
		Tag: element,
	}
}

// Reset disables and resets the parsers state to begin parsing for new elements again.
func (p *Parser) Reset() {
	p.Spec.Elements = append(p.Spec.Elements, p.currElement)
	p.active = false
	p.currElement = nil
	p.descParsed = false
}

func findTag(doc *html.Node, tag string) (*html.Node, bool) {
	if doc == nil {
		return nil, false
	}

	if doc.Type == html.ElementNode && doc.Data == tag {
		return doc, true
	}

	for child := range doc.ChildNodes() {
		if result, ok := findTag(child, tag); ok {
			return result, ok
		}
	}

	return nil, false
}

func getIDIndex(attrs []html.Attribute, key, value string) (int, bool) {
	idx := slices.IndexFunc(attrs, func(attr html.Attribute) bool {
		return attr.Key == key
	})

	if idx != -1 {
		if attrs[idx].Val == value {
			return idx, true
		}
	}

	return -1, false
}

func getAttribute(attrs []html.Attribute, key string) (string, bool) {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Val, true
		}
	}

	return "", false
}

func gatherText(node *html.Node, builder *strings.Builder) string {
	if builder == nil {
		builder = &strings.Builder{}
	}

	if node.Type == html.TextNode {
		// TODO: We could probably do better here
		cleaned := strings.ReplaceAll(node.Data, "\n   ", "")
		cleaned = strings.ReplaceAll(cleaned, "\n ", "")
		builder.WriteString(cleaned)
	} else {
		for child := range node.ChildNodes() {
			gatherText(child, builder)
		}
	}

	return builder.String()
}
