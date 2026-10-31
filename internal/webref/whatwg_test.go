package webref

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseIndicesSynthetic(t *testing.T) {
	f, err := os.Open("testdata/indices_synthetic.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	idx, err := ParseIndices(f)
	if err != nil {
		t.Fatal(err)
	}

	a := idx.Elements["a"]
	if !reflect.DeepEqual(a.Categories, []string{"flow", "interactive", "palpable", "phrasing"}) {
		t.Errorf("a categories = %v", a.Categories)
	}
	if !reflect.DeepEqual(a.Attributes, []string{"href", "target", "download"}) {
		t.Errorf("a attributes = %v", a.Attributes)
	}
	if a.EmptyContentModel() || !idx.Elements["br"].EmptyContentModel() {
		t.Errorf("empty content model: a=%v br=%v", a.EmptyContentModel(), idx.Elements["br"].EmptyContentModel())
	}
	if idx.Elements["h2"].Description != "Heading" {
		t.Errorf("multi-element row not expanded: %+v", idx.Elements["h2"])
	}
	if len(idx.Elements["head"].Categories) != 0 {
		t.Errorf(`"none" category kept: %v`, idx.Elements["head"].Categories)
	}

	got := map[string]WHATWGAttribute{}
	for _, at := range idx.Attributes {
		got[at.Name] = at
	}
	if !got["hidden"].Global || got["href"].Global || !reflect.DeepEqual(got["href"].Elements, []string{"a", "area"}) {
		t.Errorf("attribute scopes wrong: %+v", got)
	}
}

func TestClassifyValue(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want ValueClass
	}{
		{"Boolean attribute", ValueClass{Kind: KindBool}},
		{"Valid URL potentially surrounded by spaces", ValueClass{Kind: KindString, URL: true}},
		{`"until-found"; "hidden"; the empty string`, ValueClass{Kind: KindEnum, Allowed: []string{"hidden", "until-found"}, AllowEmpty: true}},
		{`"module"; "importmap"; a valid MIME type string that is not a JavaScript MIME type essence match`, ValueClass{Kind: KindEnum, Allowed: []string{"importmap", "module"}, AllowCustom: true}},
		{"Set of space-separated tokens", ValueClass{Kind: KindSST}},
		{"Set of space-separated tokens consisting of valid non-empty URLs", ValueClass{Kind: KindSST}},
		{"Autofill field name and related tokens*", ValueClass{Kind: KindSST}},
		{"Valid non-negative integer", ValueClass{Kind: KindNumber}},
		{"Valid floating-point number*", ValueClass{Kind: KindFloat}},
		{`Valid floating-point number greater than zero, or "any"`, ValueClass{Kind: KindString}},
		{"Valid month string, valid date string, valid non-negative integer, or valid duration string", ValueClass{Kind: KindString}},
		{"Valid list of floating-point numbers*", ValueClass{Kind: KindString}},
		{"Text*", ValueClass{Kind: KindString}},
	} {
		if got := ClassifyValue(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ClassifyValue(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

// TestVendoredIndex pins what the vendored snapshot yields for the attributes the generator used
// to need overlay stand-ins for, so a re-vendor that changes the wording of the index shows up here
// instead of silently changing the generated types.
func TestVendoredIndex(t *testing.T) {
	f, err := os.Open("../../data/whatwg/indices.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	idx, err := ParseIndices(f)
	if err != nil {
		t.Fatal(err)
	}
	class := func(name, el string) ValueClass {
		t.Helper()
		for _, a := range idx.Attributes {
			if a.Name == name && (el == "" && a.Global || el != "" && contains(a.Elements, el)) {
				return ClassifyValue(a.Value)
			}
		}
		t.Fatalf("index has no %s/%s", el, name)

		return ValueClass{}
	}
	for _, tc := range []struct {
		name, el string
		want     ValueClass
	}{
		{"hidden", "", ValueClass{Kind: KindEnum, Allowed: []string{"hidden", "until-found"}, AllowEmpty: true}},
		{"autocorrect", "", ValueClass{Kind: KindEnum, Allowed: []string{"off", "on"}, AllowEmpty: true}},
		{"popover", "", ValueClass{Kind: KindEnum, Allowed: []string{"auto", "hint", "manual"}, AllowEmpty: true}},
		{"itemscope", "", ValueClass{Kind: KindBool}},
		{"itemid", "", ValueClass{Kind: KindString, URL: true}},
		{"class", "", ValueClass{Kind: KindSST}},
		{"async", "script", ValueClass{Kind: KindBool}},
		{"closedby", "dialog", ValueClass{Kind: KindEnum, Allowed: []string{"any", "closerequest", "none"}}},
		{"autocomplete", "form", ValueClass{Kind: KindEnum, Allowed: []string{"off", "on"}}},
		{"autocomplete", "input", ValueClass{Kind: KindSST}},
		{"width", "iframe", ValueClass{Kind: KindNumber}},
		{"type", "script", ValueClass{Kind: KindEnum, Allowed: []string{"importmap", "module", "speculationrules"}, AllowCustom: true}},
		{"datetime", "time", ValueClass{Kind: KindString}},
		{"step", "input", ValueClass{Kind: KindString}},
		{"min", "meter", ValueClass{Kind: KindFloat}},
	} {
		if got := class(tc.name, tc.el); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s/%s = %+v, want %+v", tc.el, tc.name, got, tc.want)
		}
	}
	if contains(idx.Elements["embed"].Attributes, "any") {
		t.Errorf("embed attributes keep the \"any\" marker: %v", idx.Elements["embed"].Attributes)
	}
	if contains(idx.Elements["dialog"].Attributes, "closedby") {
		t.Log("upstream now lists closedby in dialog's Attributes column; the builder's refinement loop is no longer needed for it")
	}
}

func TestParseIndicesRejectsOtherPages(t *testing.T) {
	if _, err := ParseIndices(strings.NewReader("<html><body><p>not the index</p></body></html>")); err == nil {
		t.Error("expected an error for a page without the index tables")
	}
}
