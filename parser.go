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
	active       bool
	currElements []*Element
	descParsed   bool
	Spec         *Spec
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
// Several tags may be given when one heading documents multiple elements
// (e.g. "the sub and sup elements"); duplicates are ignored.
func (p *Parser) Activate(elements ...string) {
	p.active = true
	p.currElements = nil
	for _, tag := range elements {
		if slices.ContainsFunc(p.currElements, func(e *Element) bool { return e.Tag == tag }) {
			continue
		}
		p.currElements = append(p.currElements, &Element{Tag: tag})
	}
}

// setDescription applies the description to every element currently being collected.
func (p *Parser) setDescription(desc string) {
	for _, e := range p.currElements {
		e.Description = desc
	}
}

// Reset disables and resets the parsers state to begin parsing for new elements again.
func (p *Parser) Reset() {
	for _, e := range p.currElements {
		if !slices.ContainsFunc(p.Spec.Elements, func(x *Element) bool { return x.Tag == e.Tag }) {
			p.Spec.Elements = append(p.Spec.Elements, e)
		}
	}
	p.active = false
	p.currElements = nil
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

// findAllTags returns every element node with the given tag under doc, in document order.
func findAllTags(doc *html.Node, tag string) []*html.Node {
	var out []*html.Node
	if doc == nil {
		return out
	}
	if doc.Type == html.ElementNode && doc.Data == tag {
		out = append(out, doc)
	}
	for child := range doc.ChildNodes() {
		out = append(out, findAllTags(child, tag)...)
	}

	return out
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
