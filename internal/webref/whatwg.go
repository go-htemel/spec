package webref

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// WHATWGElement is one row of the "List of elements" table in the HTML standard's index
// (multipage/indices.html). Rows that document several elements (h1–h6, sub/sup) are expanded.
type WHATWGElement struct {
	Description string
	Categories  []string // e.g. flow, phrasing, interactive; footnote markers removed; "none" -> empty
	Children    string   // the Children column as text, e.g. "empty", "phrasing", "transparent"
	// ChildrenNote is the Children column as written, footnote markers included ("transparent*").
	ChildrenNote string
	Attributes   []string // the Attributes column without "globals"
	Interface    string
}

// EmptyContentModel reports whether the Children column says "empty" (content model: nothing).
// That is wider than the HTML syntax's void elements: iframe, template and selectedcontent have an
// empty content model but still take an end tag, so this must not be used to set spec.Element.Void.
func (e WHATWGElement) EmptyContentModel() bool {
	return strings.EqualFold(strings.TrimSpace(e.Children), "empty")
}

// WHATWGAttribute is one row of the "List of attributes" table.
type WHATWGAttribute struct {
	Name        string
	Global      bool     // the Element(s) column says "HTML elements"
	Elements    []string // element names listed in the Element(s) column
	Description string
	Value       string // the Value column as text, e.g. "Boolean attribute", `"anonymous"; "use-credentials"`
}

// WHATWGIndex is the parsed content of indices.html.
type WHATWGIndex struct {
	Elements   map[string]WHATWGElement
	Attributes []WHATWGAttribute
}

// ParseIndices parses the element and attribute index tables. Tables are located by their caption
// ("List of elements", "List of attributes") and columns by header text, so column reordering or
// added columns upstream do not silently shift data.
func ParseIndices(r io.Reader) (*WHATWGIndex, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	idx := &WHATWGIndex{Elements: map[string]WHATWGElement{}}

	elTable := findTable(doc, "List of elements")
	if elTable == nil {
		return nil, fmt.Errorf(`no table captioned "List of elements"`)
	}
	header, rows := tableRows(elTable)
	col := columnIndex(header)
	for _, need := range []string{"element", "categories", "children", "attributes"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("elements table has no %q column (headers: %v)", need, headerTexts(header))
		}
	}
	for _, row := range rows {
		names := codeTexts(cell(row, col["element"]))
		if len(names) == 0 {
			continue
		}
		e := WHATWGElement{
			Description:  cellText(cell(row, col["description"])),
			Children:     stripMarks(cellText(cell(row, col["children"]))),
			ChildrenNote: cellText(cell(row, col["children"])),
			Interface:    strings.Join(codeTexts(cell(row, col["interface"])), " "),
		}
		for _, c := range splitList(cellText(cell(row, col["categories"]))) {
			if c != "none" {
				e.Categories = append(e.Categories, c)
			}
		}
		sort.Strings(e.Categories)
		for _, a := range splitList(cellText(cell(row, col["attributes"]))) {
			// "globals" is the global attributes; embed's "any*" and custom elements' "any, as decided
			// by the element's author" say any attribute is allowed, which is no attribute name.
			if a != "globals" && a != "any" && !strings.HasPrefix(a, "any,") {
				e.Attributes = append(e.Attributes, a)
			}
		}
		for _, n := range names {
			idx.Elements[n] = e
		}
	}

	atTable := findTable(doc, "List of attributes")
	if atTable == nil {
		return nil, fmt.Errorf(`no table captioned "List of attributes"`)
	}
	header, rows = tableRows(atTable)
	col = columnIndex(header)
	for _, need := range []string{"attribute", "elements", "value"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("attributes table has no %q column (headers: %v)", need, headerTexts(header))
		}
	}
	for _, row := range rows {
		names := codeTexts(cell(row, col["attribute"]))
		if len(names) == 0 {
			continue
		}
		elCell := cell(row, col["elements"])
		a := WHATWGAttribute{
			Name:        names[0],
			Global:      strings.Contains(cellText(elCell), "HTML elements"),
			Elements:    codeTexts(elCell),
			Description: cellText(cell(row, col["description"])),
			Value:       cellText(cell(row, col["value"])),
		}
		idx.Attributes = append(idx.Attributes, a)
	}

	return idx, nil
}

// ValueClass is what the Value column of the attributes table says about an attribute's type.
type ValueClass struct {
	Kind        string
	URL         bool
	Allowed     []string
	AllowEmpty  bool
	AllowCustom bool // an enumerated list that also admits free text ("module"; ...; a valid MIME type string)
}

// ClassifyValue maps the Value column of the attributes table onto a generator attribute kind.
func ClassifyValue(v string) ValueClass {
	t := strings.TrimSpace(stripMarks(v))
	lower := strings.ToLower(t)
	switch {
	case strings.HasPrefix(lower, "boolean attribute"):
		return ValueClass{Kind: KindBool}
	case strings.HasPrefix(t, `"`) || strings.HasPrefix(t, "“"):
		c := ValueClass{Kind: KindEnum}
		for _, part := range strings.Split(t, ";") {
			part = strings.TrimSpace(part)
			switch {
			case part == "the empty string":
				c.AllowEmpty = true
			case strings.HasPrefix(part, `"`) || strings.HasPrefix(part, "“"):
				if part = strings.Trim(part, "\"“”"); part != "" {
					c.Allowed = append(c.Allowed, part)
				}
			case part != "":
				c.AllowCustom = true // prose such as "a valid MIME type string that is not ..."
			}
		}
		sort.Strings(c.Allowed)
		return c
	case strings.Contains(lower, "space-separated tokens"), strings.HasPrefix(lower, "autofill field name"):
		return ValueClass{Kind: KindSST}
	case strings.Contains(lower, "url"):
		return ValueClass{Kind: KindString, URL: true}
	case strings.Contains(lower, "list of"), strings.Contains(lower, ", or "):
		// Lists and unions ("valid month string, ..., or valid duration string"; a number "or \"any\"")
		// are not one number.
		return ValueClass{Kind: KindString}
	case strings.HasPrefix(lower, "valid integer"), strings.HasPrefix(lower, "valid non-negative integer"):
		return ValueClass{Kind: KindNumber}
	case strings.HasPrefix(lower, "valid floating-point number"):
		return ValueClass{Kind: KindFloat}
	}

	return ValueClass{Kind: KindString}
}

// ExtractIndexTables returns a minimal HTML document containing only the "List of elements" and
// "List of attributes" tables from the HTML standard, so that just those tables can be vendored
// instead of the whole page. The result parses with ParseIndices.
func ExtractIndexTables(r io.Reader) ([]byte, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString("<!DOCTYPE html>\n<meta charset=\"utf-8\">\n<title>HTML Standard index tables (extract)</title>\n")
	for _, caption := range []string{"List of elements", "List of attributes"} {
		t := findTable(doc, caption)
		if t == nil {
			return nil, fmt.Errorf("no table captioned %q", caption)
		}
		if err := html.Render(&buf, t); err != nil {
			return nil, err
		}
		buf.WriteByte('\n')
	}

	return buf.Bytes(), nil
}

func findTable(n *html.Node, caption string) *html.Node {
	if n.Type == html.ElementNode && n.Data == "table" {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "caption" && strings.HasPrefix(cellText(c), caption) {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if t := findTable(c, caption); t != nil {
			return t
		}
	}

	return nil
}

// tableRows returns the header cells (from thead, or the first row) and the body rows' cells.
func tableRows(table *html.Node) (header []*html.Node, rows [][]*html.Node) {
	var walk func(n *html.Node, inHead bool)
	walk = func(n *html.Node, inHead bool) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			switch c.Data {
			case "thead":
				walk(c, true)
			case "tbody", "tfoot":
				walk(c, false)
			case "tr":
				var cells []*html.Node
				for td := c.FirstChild; td != nil; td = td.NextSibling {
					if td.Type == html.ElementNode && (td.Data == "td" || td.Data == "th") {
						cells = append(cells, td)
					}
				}
				if inHead || header == nil {
					if header == nil {
						header = cells
					}
				} else {
					rows = append(rows, cells)
				}
			}
		}
	}
	walk(table, false)

	return header, rows
}

// columnIndex maps a normalised header ("element", "elements", "value", ...) to its column.
func columnIndex(header []*html.Node) map[string]int {
	out := map[string]int{}
	for i, h := range header {
		var b strings.Builder
		for _, r := range strings.ToLower(cellText(h)) {
			if r >= 'a' && r <= 'z' {
				b.WriteRune(r)
			}
		}
		out[b.String()] = i // "Element(s)" -> "elements", "Parents†" -> "parents"
	}

	return out
}

func headerTexts(header []*html.Node) []string {
	var out []string
	for _, h := range header {
		out = append(out, cellText(h))
	}

	return out
}

func cell(row []*html.Node, i int) *html.Node {
	if i < 0 || i >= len(row) {
		return nil
	}

	return row[i]
}

func cellText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	return strings.Join(strings.Fields(b.String()), " ")
}

func codeTexts(n *html.Node) []string {
	if n == nil {
		return nil
	}
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "code" {
			if t := cellText(n); t != "" && !contains(out, t) {
				out = append(out, t)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	return out
}

// stripMarks removes footnote markers (*, †, ‡) used throughout the index tables.
func stripMarks(s string) string {
	return strings.NewReplacer("*", "", "†", "", "‡", "").Replace(s)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(stripMarks(s), ";") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}

	return out
}
