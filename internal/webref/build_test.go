package webref

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/go-htemel/spec"
)

const dataDir = "../../data"

func loadTestSources(t *testing.T) *Sources {
	t.Helper()
	src, err := LoadSources(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range Configs {
		if err := src.LoadLegacy(c.Name, filepath.Join("../../specs", c.Name+".json")); err != nil {
			t.Fatal(err)
		}
	}

	return src
}

func buildAll(t *testing.T, src *Sources) map[string]*spec.Spec {
	t.Helper()
	out := map[string]*spec.Spec{}
	for _, c := range Configs {
		sp, _, err := Build(c, src, Options{})
		if err != nil {
			t.Fatal(err)
		}
		out[c.Name] = sp
	}

	return out
}

func find(sp *spec.Spec, tag, attr string) spec.Attribute {
	list := sp.Attributes
	if tag != "" {
		list = nil
		for _, e := range sp.Elements {
			if e.Tag == tag {
				list = e.Attributes
			}
		}
	}
	for _, a := range list {
		if a.GetName() == attr {
			return a
		}
	}

	return nil
}

func element(sp *spec.Spec, tag string) *spec.Element {
	for _, e := range sp.Elements {
		if e.Tag == tag {
			return e
		}
	}

	return nil
}

// TestCandidatesUpToDate regenerates every document from the committed data and compares it with
// specs/candidates, so a data or overlay change without regenerated candidates fails CI.
func TestCandidatesUpToDate(t *testing.T) {
	src := loadTestSources(t)
	for _, c := range Configs {
		sp, prov, err := Build(c, src, Options{})
		if err != nil {
			t.Fatal(err)
		}
		for path, v := range map[string]any{
			filepath.Join("../../specs/candidates", c.Name+".json"):            sp,
			filepath.Join("../../specs/candidates", c.Name+".provenance.json"): prov,
		} {
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(append(got, '\n'), want) {
				t.Errorf("%s is stale; run `go run ./cmd/specgen2 generate`", path)
			}
		}
	}
}

func TestBuildDeterministic(t *testing.T) {
	a, _ := json.Marshal(buildAll(t, loadTestSources(t)))
	b, _ := json.Marshal(buildAll(t, loadTestSources(t)))
	if !bytes.Equal(a, b) {
		t.Error("two builds from the same data differ")
	}
}

func TestBuildSorted(t *testing.T) {
	for name, sp := range buildAll(t, loadTestSources(t)) {
		for i := 1; i < len(sp.Elements); i++ {
			if sp.Elements[i-1].Tag >= sp.Elements[i].Tag {
				t.Errorf("%s: elements not sorted at %s", name, sp.Elements[i].Tag)
			}
		}
		lists := [][]spec.Attribute{sp.Attributes}
		for _, e := range sp.Elements {
			lists = append(lists, e.Attributes)
		}
		for _, l := range lists {
			for i := 1; i < len(l); i++ {
				if l[i-1].GetName() >= l[i].GetName() {
					t.Errorf("%s: attributes not sorted at %s", name, l[i].GetName())
				}
			}
		}
	}
}

// TestBuildFacts pins behaviour that motivated the pipeline: each case is a known error or gap of
// the hand-written generator, or a rule of the new one.
func TestBuildFacts(t *testing.T) {
	all := buildAll(t, loadTestSources(t))
	html, svg, mathml := all["html"], all["svg"], all["mathml"]

	type check struct {
		sp         *spec.Spec
		tag, attr  string
		want       string // type label as in the coverage report, "URL" for URL strings, "-" for absent
		deprecated bool
	}
	for _, c := range []check{
		{html, "textarea", "rows", "Number", false},                   // was String
		{html, "a", "download", "Enum{}+empty+custom", false},         // was Bool
		{html, "a", "href", "URL", false},                             // [ReflectSetter] USVString
		{html, "img", "src", "URL", false},                            // [ReflectURL]
		{html, "script", "src", "URL", false},                         // (TrustedScriptURL or USVString)
		{html, "img", "srcset", "String", false},                      // plain [Reflect] USVString is a list, not a URL
		{html, "a", "ping", "SST", false},                             // URL list
		{html, "a", "rel", "SST", false},                              // DOMTokenList relList wins over DOMString rel
		{html, "td", "colspan", "Number", false},                      // legacy had a non-existent "span"
		{html, "td", "span", "-", false},                              //
		{html, "img", "align", "-", false},                            // obsolete attributes are dropped
		{html, "audio", "playsinline", "-", false},                    // video only
		{html, "input", "max", "String", false},                       // dates too, not Number
		{html, "select", "autocomplete", "SST", false},                // consistent autofill tokens
		{html, "form", "autocomplete", "Enum{off,on}", false},         // element entry beats "*"
		{html, "template", "shadowrootdelegatesfocus", "Bool", false}, // legacy misspelled it
		{html, "body", "onload", "-", false},                          // GlobalEventHandlers not included
		{html, "body", "onpopstate", "String", false},                 // WindowEventHandlers are
		{html, "", "class", "SST", false},
		{html, "", "headingoffset", "Number", false},
		{svg, "rect", "pathLength", "Float", false},                           // SVGAnimatedNumber
		{svg, "feConvolveMatrix", "preserveAlpha", "Enum{false,true}", false}, // SVG booleans are keywords
		{svg, "feGaussianBlur", "stdDeviation", "String", false},              // X/Y IDL members collapsed
		{svg, "feGaussianBlur", "stdDeviationX", "-", false},
		{svg, "feImage", "crossorigin", "Enum{anonymous,use-credentials}+empty", false},
		{svg, "a", "xlink:href", "URL", true},
		{svg, "animate", "fill", "Enum{freeze,remove}", false},
		{svg, "", "class", "SST", false},
		{svg, "", "tabindex", "Number", false}, // shared HTMLOrSVGOrMathMLElement mixin
		{mathml, "mo", "stretchy", "Enum{false,true}", false},
		{mathml, "", "mathcolor", "String", true},
	} {
		a := find(c.sp, c.tag, c.attr)
		got := "-"
		if a != nil {
			got = typeLabel(a)
			if s, ok := a.(*spec.AttributeTypeString); ok && s.URL {
				got = "URL"
			}
		}
		if got != c.want {
			t.Errorf("%s <%s> %s: got %s, want %s", c.sp.Name, c.tag, c.attr, got, c.want)
		}
		if a != nil && a.IsDeprecated() != c.deprecated {
			t.Errorf("%s <%s> %s: deprecated = %v", c.sp.Name, c.tag, c.attr, a.IsDeprecated())
		}
	}

	if e := element(html, "script"); e == nil || !e.RawText || !e.Text || e.Interface != "HTMLScriptElement" {
		t.Errorf("script = %+v", e)
	}
	if e := element(html, "br"); e == nil || !e.Void || e.Text {
		t.Errorf("br = %+v", e)
	}
	if element(html, "marquee") != nil || element(html, "sup") == nil {
		t.Error("obsolete elements must be dropped and sup present")
	}
	if element(svg, "feGaussianBlur") == nil || len(svg.Elements) != 63 || len(mathml.Elements) != 31 {
		t.Errorf("element counts: svg %d mathml %d", len(svg.Elements), len(mathml.Elements))
	}
	if typ := find(html, "input", "type"); typ == nil || !strings.Contains(typeLabel(typ), "datetime-local") {
		t.Errorf("input type from browser-compat-data keys: %v", typ)
	}
}

func TestBuildIncludeObsolete(t *testing.T) {
	src := loadTestSources(t)
	cfg, _ := ConfigByName("html")
	sp, _, err := Build(cfg, src, Options{IncludeObsolete: true})
	if err != nil {
		t.Fatal(err)
	}
	if e := element(sp, "marquee"); e == nil || !e.Deprecated {
		t.Errorf("marquee = %+v", e)
	}
	if a := find(sp, "img", "align"); a == nil || !a.IsDeprecated() {
		t.Errorf("img align = %v", a)
	}
}

// TestBuildWithWHATWG exercises the index path with the synthetic fixture: void flags and
// categories come from the index, and index value types refine weak IDL types.
func TestBuildWithWHATWG(t *testing.T) {
	src := loadTestSources(t)
	f, err := os.Open("testdata/indices_synthetic.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if src.WHATWG, err = ParseIndices(f); err != nil {
		t.Fatal(err)
	}
	// Drop the overlay's stand-ins so the index is what decides.
	ov := *src.Overlays["html"]
	ov.Void = nil
	src.Overlays["html"] = &ov

	cfg, _ := ConfigByName("html")
	sp, prov, err := Build(cfg, src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !prov.WHATWG {
		t.Error("provenance does not record the index")
	}
	br, a := element(sp, "br"), element(sp, "a")
	if !br.Void || a.Void {
		t.Errorf("void from index: br=%v a=%v", br.Void, a.Void)
	}
	if !reflect.DeepEqual(a.Categories, []string{"flow", "interactive", "palpable", "phrasing"}) {
		t.Errorf("a categories = %v", a.Categories)
	}
	if p := prov.Elements["br"]; p.FlagsSrc != "whatwg" {
		t.Errorf("br flags source = %q", p.FlagsSrc)
	}
	// target is a reflected DOMString; the index's free-text value must not override it.
	if p := prov.Attributes["a/target"]; p.TypeSource != srcIDL {
		t.Errorf("a/target type source = %q", p.TypeSource)
	}
	// Overlay still wins over the index (download: index says Text).
	if got := typeLabel(find(sp, "a", "download")); got != "Enum{}+empty+custom" {
		t.Errorf("a download = %s", got)
	}
}

func TestOverlayValidation(t *testing.T) {
	for _, tc := range []struct{ doc, want string }{
		{`{"attributes":{"a":{"x":{"type":"String"}}}}`, "needs a reason"},
		{`{"attributes":{"a":{"x":{"type":"Strng","reason":"r"}}}}`, "unknown type"},
		{`{"attributes":{"a":{"x":{"type":"String","allowed":["q"],"reason":"r"}}}}`, "need type Enum"},
		{`{"attributes":{"a":{"x":{"type":"Enum","reason":"r"}}}}`, "without any allowed"},
		{`{"attributes":{"a,b":{"x":{"type":"Bool","reason":"r"}},"b":{"x":{"type":"Bool","reason":"r"}}}}`, "more than once"},
		{`{"voids":["br"]}`, "unknown field"},
	} {
		_, err := ParseOverlay([]byte(tc.doc))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want error containing %q", tc.doc, err, tc.want)
		}
	}
}

func TestCheckNamesCollisions(t *testing.T) {
	sp := &spec.Spec{Elements: []*spec.Element{
		{Tag: "ol", Attributes: []spec.Attribute{&spec.AttributeTypeEnum{Name: "type", Allowed: map[string]struct{}{"a": {}, "A": {}}}}},
		{Tag: "x", Attributes: []spec.Attribute{&spec.AttributeTypeString{Name: "stroke-width"}, &spec.AttributeTypeString{Name: "strokeWidth"}}},
	}}
	err := CheckNames(sp)
	if err == nil || !strings.Contains(err.Error(), "OlTypeA") || !strings.Contains(err.Error(), "StrokeWidth") {
		t.Errorf("CheckNames = %v", err)
	}
}

func TestCompare(t *testing.T) {
	oldSp := &spec.Spec{
		Attributes: []spec.Attribute{&spec.AttributeTypeString{Name: "id"}},
		Elements: []*spec.Element{
			{Tag: "a", Attributes: []spec.Attribute{&spec.AttributeTypeBool{Name: "download"}, &spec.AttributeTypeString{Name: "gone"}}},
			{Tag: "old"},
		},
	}
	newSp := &spec.Spec{
		Attributes: []spec.Attribute{&spec.AttributeTypeString{Name: "id"}, &spec.AttributeTypeSST{Name: "class"}},
		Elements: []*spec.Element{
			{Tag: "a", Attributes: []spec.Attribute{&spec.AttributeTypeString{Name: "download"}, &spec.AttributeTypeString{Name: "href", URL: true}}},
			{Tag: "new"},
		},
	}
	prov := &Provenance{Attributes: map[string]AttrProvenance{
		"a/download": {Source: "idl", TypeSource: srcIDL},
		"a/href":     {Source: "idl", TypeSource: srcIDL},
	}}
	d := Compare(oldSp, newSp, prov)
	if !reflect.DeepEqual(d.OnlyOldElements, []string{"old"}) || !reflect.DeepEqual(d.OnlyNewElements, []string{"new"}) {
		t.Errorf("elements: %v %v", d.OnlyOldElements, d.OnlyNewElements)
	}
	c := d.Count()
	if c.TypesN != 1 || c.Types[ClassEvidence] != 1 || c.OnlyOldN != 1 || c.OnlyNewN != 2 || d.URLFlagged != 1 {
		t.Errorf("counts = %+v url=%d", c, d.URLFlagged)
	}
	if md := d.Markdown("old", "new"); !strings.Contains(md, "`download` | Bool | String | evidence") {
		t.Errorf("markdown:\n%s", md)
	}
}
