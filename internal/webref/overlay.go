package webref

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Attribute kinds, named after the spec.AttributeType* types (URL is a String with URL set).
const (
	KindString         = "String"
	KindURL            = "URL"
	KindChar           = "Char"
	KindNumber         = "Number"
	KindFloat          = "Float"
	KindBool           = "Bool"
	KindSST            = "SST"
	KindEnum           = "Enum"
	KindPrefixedCustom = "PrefixedCustom"
)

var validKinds = map[string]bool{
	KindString: true, KindURL: true, KindChar: true, KindNumber: true, KindFloat: true,
	KindBool: true, KindSST: true, KindEnum: true, KindPrefixedCustom: true,
}

// AttrOverride is one overlay entry. Every entry must carry a Reason, so the file doubles as the
// list of judgment calls the generator makes.
type AttrOverride struct {
	Type        string   `json:"type,omitempty"`
	Allowed     []string `json:"allowed,omitempty"`
	AllowEmpty  bool     `json:"allow_empty,omitempty"`
	AllowCustom bool     `json:"allow_custom,omitempty"`
	Deprecated  *bool    `json:"deprecated,omitempty"`
	Remove      bool     `json:"remove,omitempty"`
	// Pending names the machine-readable source that would make the entry unnecessary once it is
	// vendored ("whatwg-index" for the HTML attribute index, "webref-css" for CSS value syntax of
	// presentation attributes, ...). The coverage report lists these separately from genuine
	// judgment calls.
	Pending string `json:"pending,omitempty"`
	Reason  string `json:"reason"`
}

// ContentModelOverride replaces the content model the WHATWG index gives one element. Like
// attribute overrides it must carry a Reason; use it only where the index is wrong or unparseable.
type ContentModelOverride struct {
	Kind       string   `json:"kind"`
	Categories []string `json:"categories,omitempty"`
	Elements   []string `json:"elements,omitempty"`
	Text       bool     `json:"text,omitempty"`
	// Note replaces the index's prose in the generated model; empty keeps the index's prose.
	Note   string `json:"note,omitempty"`
	Reason string `json:"reason"`
}

// Overlay holds the hand-maintained part of one spec's inputs.
type Overlay struct {
	Comment string `json:"comment,omitempty"`

	// Element flags. Void is the HTML syntax's void element list, which no vendored table states
	// (the WHATWG index's Children column is "empty" for iframe and template too). NoText is a
	// stand-in for that Children column; RawText is not in any table.
	Void   []string `json:"void,omitempty"`
	NoText []string `json:"no_text,omitempty"`
	// Text, when set, is an allow-list instead: only these elements accept text children
	// (used for SVG and MathML, where text content is the exception).
	Text    []string `json:"text,omitempty"`
	RawText []string `json:"raw_text,omitempty"`

	// EventHandlerMixins lists IDL mixins whose EventHandler attributes become content attributes
	// (e.g. WindowEventHandlers on body). GlobalEventHandlers is deliberately not listed.
	EventHandlerMixins []string `json:"event_handler_mixins,omitempty"`

	// IDLRenames maps IDL attribute names to content attribute names where they differ and no
	// [Reflect=name] says so (SVG: in1 -> in, stdDeviationX -> stdDeviation, ...).
	IDLRenames map[string]string `json:"idl_renames,omitempty"`

	// IgnoreBCD lists browser-compat-data sub-feature keys that are not attributes.
	IgnoreBCD []string `json:"ignore_bcd,omitempty"`

	// BCDValueKeys maps "<tag>/<attr>" to an attribute whose keywords browser-compat-data records
	// as sibling keys "<attr>_<keyword>" (input's type_button, type_checkbox, ...). The attribute
	// becomes an Enum of those keywords.
	BCDValueKeys []string `json:"bcd_value_keys,omitempty"`

	// Attributes is keyed by element tag (or several, comma-separated), "@global" for global
	// attributes, or "*" for an entry that applies to every element that already has an attribute
	// of that name (it never adds one). Element-specific entries win over "*".
	Attributes map[string]map[string]AttrOverride `json:"attributes,omitempty"`

	// ContentModel overrides the content model derived from the WHATWG index, keyed by tag.
	ContentModel map[string]ContentModelOverride `json:"content_model,omitempty"`

	scoped map[string]map[string]AttrOverride // Attributes with comma-separated keys expanded
}

// For returns the overrides for one scope (tag, "@global" or "*").
func (o *Overlay) For(scope string) map[string]AttrOverride {
	return o.scoped[scope]
}

// Scopes returns every scope the overlay mentions, sorted.
func (o *Overlay) Scopes() []string {
	return sortedKeys(o.scoped)
}

// ParseOverlay decodes and validates an overlay file. Unknown keys are rejected so typos surface.
func ParseOverlay(b []byte) (*Overlay, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	o := &Overlay{}
	if err := dec.Decode(o); err != nil {
		return nil, err
	}
	o.scoped = map[string]map[string]AttrOverride{}
	for _, key := range sortedKeys(o.Attributes) {
		for _, scope := range strings.Split(key, ",") {
			scope = strings.TrimSpace(scope)
			if o.scoped[scope] == nil {
				o.scoped[scope] = map[string]AttrOverride{}
			}
			for name, ov := range o.Attributes[key] {
				if _, dup := o.scoped[scope][name]; dup {
					return nil, fmt.Errorf("%s/%s: defined more than once", scope, name)
				}
				o.scoped[scope][name] = ov
			}
		}
	}
	for _, scope := range o.Scopes() {
		for _, name := range sortedKeys(o.scoped[scope]) {
			ov := o.scoped[scope][name]
			where := scope + "/" + name
			if ov.Reason == "" {
				return nil, fmt.Errorf("%s: overlay entry needs a reason", where)
			}
			if ov.Remove {
				continue
			}
			if ov.Type != "" && !validKinds[ov.Type] {
				return nil, fmt.Errorf("%s: unknown type %q", where, ov.Type)
			}
			if ov.Type != KindEnum && (len(ov.Allowed) > 0 || ov.AllowEmpty || ov.AllowCustom) {
				return nil, fmt.Errorf("%s: allowed/allow_empty/allow_custom need type Enum", where)
			}
			if ov.Type == KindEnum && len(ov.Allowed) == 0 && !ov.AllowEmpty && !ov.AllowCustom {
				return nil, fmt.Errorf("%s: Enum without any allowed value", where)
			}
		}
	}

	for _, tag := range sortedKeys(o.ContentModel) {
		cm := o.ContentModel[tag]
		where := "content_model/" + tag
		if cm.Reason == "" {
			return nil, fmt.Errorf("%s: overlay entry needs a reason", where)
		}
		if !contentKinds[cm.Kind] {
			return nil, fmt.Errorf("%s: unknown kind %q", where, cm.Kind)
		}
		for _, c := range cm.Categories {
			if !categoryVocab[c] {
				return nil, fmt.Errorf("%s: unknown category %q", where, c)
			}
		}
	}

	return o, nil
}
