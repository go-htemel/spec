package spec

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSpecRoundTrip(t *testing.T) {
	attrs := []Attribute{
		&AttributeTypeString{Name: "s", Description: "d"},
		&AttributeTypeChar{Name: "c", Description: "d"},
		&AttributeTypeNumber{Name: "n", Description: "d"},
		&AttributeTypeFloat{Name: "f", Description: "d"},
		&AttributeTypeBool{Name: "b", Description: "d"},
		&AttributeTypeEnum{Name: "e", Description: "d", AllowCustom: true, AllowEmpty: true,
			Allowed: map[string]struct{}{"z": {}, "a": {}, "m": {}}},
		&AttributeTypeSST{Name: "sst", Description: "d"},
		&AttributeTypePrefixedCustom{Name: "data", Description: "d"},
	}
	in := &Spec{
		Name:       "T",
		Attributes: attrs,
		Elements:   []*Element{{Tag: "x", Description: "d", Text: true, Attributes: attrs}},
	}

	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	out := &Spec{}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip mismatch:\nin:  %+v\nout: %+v", in, out)
	}
}

func TestUnknownAttributeType(t *testing.T) {
	err := json.Unmarshal([]byte(`{"name":"T","attributes":[{"name":"x","attribute_type":"Bogus"}]}`), &Spec{})
	if err == nil || !strings.Contains(err.Error(), "Bogus") {
		t.Errorf("expected error naming Bogus, got %v", err)
	}
}

func TestEnumDeterministic(t *testing.T) {
	e := AttributeTypeEnum{Name: "e", Allowed: map[string]struct{}{"c": {}, "a": {}, "b": {}}}
	if got := e.AllowedSorted(); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("AllowedSorted = %v", got)
	}
	b, _ := json.Marshal(e)
	if !strings.Contains(string(b), `"allowed":{"a":{},"b":{},"c":{}}`) {
		t.Errorf("unsorted output: %s", b)
	}
}

// TestCommittedSpecsRoundTrip checks that the committed spec documents load and re-marshal without
// loss, i.e. that the fields added after v0.2.0 do not change existing output. specs/html.json was
// written by json.MarshalIndent and must match byte for byte; the hand-written specs/svg.json spells
// out some zero values ("void": false), so it is compared after decoding.
func TestCommittedSpecsRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		path  string
		exact bool
	}{{"specs/html.json", true}, {"specs/svg.json", false}} {
		t.Run(tc.path, func(t *testing.T) {
			want, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			sp := &Spec{}
			if err := json.Unmarshal(want, sp); err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(sp, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if tc.exact && !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
				t.Errorf("%s does not round-trip byte for byte", tc.path)
			}
			again := &Spec{}
			if err := json.Unmarshal(got, again); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(sp, again) {
				t.Errorf("%s changes after a marshal/unmarshal cycle", tc.path)
			}
		})
	}
}

func TestNewFieldsRoundTrip(t *testing.T) {
	in := &Spec{
		Name: "T",
		Attributes: []Attribute{
			&AttributeTypeString{Name: "href", URL: true, Deprecated: true},
			&AttributeTypeBool{Name: "b", Deprecated: true},
			&AttributeTypeEnum{Name: "e", Allowed: map[string]struct{}{"x": {}}, Deprecated: true},
		},
		Elements: []*Element{{
			Tag: "script", Text: true, RawText: true, Deprecated: true,
			Attributes: []Attribute{&AttributeTypeString{Name: "src", URL: true}},
			Interface:  "HTMLScriptElement", Categories: []string{"flow", "metadata", "phrasing"},
		}},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{`"url":true`, `"deprecated":true`, `"raw_text":true`, `"interface":"HTMLScriptElement"`, `"categories":["flow","metadata","phrasing"]`} {
		if !strings.Contains(string(b), frag) {
			t.Errorf("missing %s in %s", frag, b)
		}
	}
	out := &Spec{}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip mismatch:\nin:  %+v\nout: %+v", in, out)
	}
	for _, a := range out.Attributes {
		if !a.IsDeprecated() {
			t.Errorf("%s: IsDeprecated() = false", a.GetName())
		}
	}
}

// TestOldConsumerCompat decodes a document using the new fields with a mirror of the v0.2.0 wire
// structs (no new fields, same discriminator switch) to show older consumers still load it.
func TestOldConsumerCompat(t *testing.T) {
	doc := `{"name":"T","elements":[{"tag":"a","raw_text":true,"deprecated":true,"interface":"X","categories":["flow"],
		"attributes":[{"name":"href","url":true,"deprecated":true,"attribute_type":"AttributeTypeString"}]}]}`
	var old struct {
		Name     string `json:"name"`
		Elements []struct {
			Tag        string `json:"tag"`
			Attributes []struct {
				Name          string `json:"name"`
				Description   string `json:"description"`
				AttributeType string `json:"attribute_type"`
			} `json:"attributes"`
		} `json:"elements"`
	}
	if err := json.Unmarshal([]byte(doc), &old); err != nil {
		t.Fatal(err)
	}
	if old.Elements[0].Attributes[0].AttributeType != "AttributeTypeString" {
		t.Errorf("discriminator changed: %+v", old)
	}
}

func TestContentModelRoundTrip(t *testing.T) {
	in := &Spec{Name: "T", Elements: []*Element{
		{Tag: "ul", Categories: []string{"flow", "palpable"}, ContentModel: &ContentModel{
			Kind: ContentKindChildren, Categories: []string{"script-supporting"}, Elements: []string{"li"},
			Note: "li; script-supporting elements"}},
		{Tag: "p", ContentModel: &ContentModel{Kind: ContentKindChildren, Categories: []string{"phrasing"}, Text: true}},
		{Tag: "br", Void: true, ContentModel: &ContentModel{Kind: ContentKindEmpty}},
		{Tag: "x"},
	}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{
		`"content_model":{"kind":"children","categories":["script-supporting"],"elements":["li"],"note":"li; script-supporting elements"}`,
		`"content_model":{"kind":"children","categories":["phrasing"],"text":true}`,
		`"content_model":{"kind":"empty"}`,
		`{"tag":"x"}`,
	} {
		if !strings.Contains(string(b), frag) {
			t.Errorf("missing %s in %s", frag, b)
		}
	}
	out := &Spec{}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatal(err)
	}
	for i, e := range out.Elements {
		if !reflect.DeepEqual(e.ContentModel, in.Elements[i].ContentModel) {
			t.Errorf("<%s>: content model %+v, want %+v", e.Tag, e.ContentModel, in.Elements[i].ContentModel)
		}
	}
	again, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, again) {
		t.Errorf("re-marshal differs:\n%s\n%s", b, again)
	}
}

// TestOldDocumentWithoutContentModel loads a document written before content_model existed.
func TestOldDocumentWithoutContentModel(t *testing.T) {
	doc := `{"name":"HTML","elements":[{"tag":"ul","categories":["flow","palpable"],"attributes":[{"name":"compact","description":"","attribute_type":"AttributeTypeBool"}]}]}`
	sp := &Spec{}
	if err := json.Unmarshal([]byte(doc), sp); err != nil {
		t.Fatal(err)
	}
	if e := sp.Elements[0]; e.ContentModel != nil || len(e.Attributes) != 1 {
		t.Errorf("old document decoded as %+v", e)
	}
}

func TestContentCategoriesVocabulary(t *testing.T) {
	if len(ContentCategories) != 16 {
		t.Errorf("ContentCategories has %d entries", len(ContentCategories))
	}
	for _, c := range ContentCategories {
		if c != strings.ToLower(c) || strings.Contains(c, " ") {
			t.Errorf("category %q is not lower-case and hyphenated", c)
		}
	}
}
