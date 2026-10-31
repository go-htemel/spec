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
	if a.Void() || !idx.Elements["br"].Void() {
		t.Errorf("void: a=%v br=%v", a.Void(), idx.Elements["br"].Void())
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
		in, kind string
		url      bool
		allowed  []string
		empty    bool
	}{
		{"Boolean attribute", KindBool, false, nil, false},
		{"Valid URL potentially surrounded by spaces", KindString, true, nil, false},
		{`"until-found"; "hidden"; the empty string`, KindEnum, false, []string{"hidden", "until-found"}, true},
		{"Set of space-separated tokens", KindSST, false, nil, false},
		{"Set of space-separated tokens consisting of valid non-empty URLs", KindSST, false, nil, false},
		{"Valid non-negative integer", KindNumber, false, nil, false},
		{"Valid floating-point number*", KindFloat, false, nil, false},
		{"Valid list of floating-point numbers*", KindString, false, nil, false},
		{"Text*", KindString, false, nil, false},
	} {
		k, u, al, e := ClassifyValue(tc.in)
		if k != tc.kind || u != tc.url || !reflect.DeepEqual(al, tc.allowed) || e != tc.empty {
			t.Errorf("ClassifyValue(%q) = %s %v %v %v", tc.in, k, u, al, e)
		}
	}
}

func TestParseIndicesRejectsOtherPages(t *testing.T) {
	if _, err := ParseIndices(strings.NewReader("<html><body><p>not the index</p></body></html>")); err == nil {
		t.Error("expected an error for a page without the index tables")
	}
}
