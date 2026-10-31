package webref

import (
	"sort"
	"strings"

	"github.com/go-htemel/spec"
)

var categoryVocab = func() map[string]bool {
	m := map[string]bool{}
	for _, c := range spec.ContentCategories {
		m[c] = true
	}
	return m
}()

var contentKinds = map[string]bool{
	spec.ContentKindEmpty: true, spec.ContentKindText: true, spec.ContentKindTransparent: true,
	spec.ContentKindChildren: true, spec.ContentKindUnknown: true,
}

// NormalizeCategory maps the index's wording of a content category ("flow", "Flow content",
// "script-supporting elements", "sectioning root", "metadata content*") onto the
// spec.ContentCategories vocabulary. It returns "" for wording that names no category.
func NormalizeCategory(s string) string {
	t := strings.Join(strings.Fields(strings.ToLower(stripMarks(s))), " ")
	for _, suffix := range []string{" content", " elements", " element"} {
		t = strings.TrimSuffix(t, suffix)
	}
	t = strings.ReplaceAll(t, " ", "-")
	if categoryVocab[t] {
		return t
	}

	return ""
}

// normalizeCategories normalizes, de-duplicates and sorts a category list, returning the entries
// that could not be mapped separately.
func normalizeCategories(in []string) (out, unmapped []string) {
	for _, c := range in {
		n := NormalizeCategory(c)
		switch {
		case n == "":
			unmapped = append(unmapped, c)
		case !contains(out, n):
			out = append(out, n)
		}
	}
	sort.Strings(out)

	return out, unmapped
}

// textOnlyChildren are whole Children cells (marks removed, lower case) that mean "text".
var textOnlyChildren = map[string]bool{
	"text":                                  true, // style, title, textarea, rp
	"script, data, or script documentation": true, // script
}

// DeriveContentModel maps the Children column of one index row onto a content model. isElement
// reports whether a word is an HTML element name. The second result lists the parts of the cell
// that could not be mapped; when it is non-empty the model has Kind unknown, because dropping a
// part could forbid children the specification allows.
//
// Phrasings in the vendored index (footnote markers removed): "empty"; "text"; "script, data, or
// script documentation"; "transparent"; "flow"; "phrasing"; "metadata content"; "heading content";
// "script-supporting elements"; element names, once as "one img"; combinations of those separated
// by ";" ("li; script-supporting elements", "source; track; transparent", "text; div; phrasing").
// "varies" (noscript) and "per [SVG]"/"per [MATHML]" have no mapping. Order and conditions ("one
// summary element followed by flow content") are not part of the column: the model is the union
// of the parts.
func DeriveContentModel(w WHATWGElement, isElement func(string) bool) (*spec.ContentModel, []string) {
	cm := &spec.ContentModel{Note: w.ChildrenNote}
	whole := strings.ToLower(strings.TrimSpace(w.Children))
	switch {
	case whole == "empty":
		cm.Kind = spec.ContentKindEmpty
		return cm, nil
	case textOnlyChildren[whole]:
		cm.Kind, cm.Text = spec.ContentKindText, true
		return cm, nil
	}

	var unmapped []string
	if whole == "" {
		unmapped = append(unmapped, "(empty Children cell)")
	}
	transparent, other := false, false
	for _, part := range splitList(whole) {
		name := strings.TrimPrefix(strings.TrimPrefix(part, "one "), "zero or more ")
		switch cat := NormalizeCategory(part); {
		case part == "transparent":
			transparent = true
		case part == "text":
			cm.Text, other = true, true
		case cat != "":
			if !contains(cm.Categories, cat) {
				cm.Categories = append(cm.Categories, cat)
			}
			if cat == "flow" || cat == "phrasing" {
				cm.Text = true // phrasing content includes text; flow content includes phrasing content
			}
			other = true
		case isElement(name):
			if !contains(cm.Elements, name) {
				cm.Elements = append(cm.Elements, name)
			}
		default:
			unmapped = append(unmapped, part)
		}
	}
	sort.Strings(cm.Categories)
	sort.Strings(cm.Elements)
	switch {
	case len(unmapped) > 0:
		return &spec.ContentModel{Kind: spec.ContentKindUnknown, Note: w.ChildrenNote}, unmapped
	case transparent:
		if other {
			// Not in the vendored index; categories next to "transparent" would be ambiguous.
			return &spec.ContentModel{Kind: spec.ContentKindUnknown, Note: w.ChildrenNote}, []string{whole}
		}
		cm.Kind = spec.ContentKindTransparent
	case cm.Text && len(cm.Categories) == 0 && len(cm.Elements) == 0:
		cm.Kind = spec.ContentKindText
	default:
		cm.Kind = spec.ContentKindChildren
	}

	return cm, nil
}
