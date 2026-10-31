package spec

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Spec defines the spec document that all found elements and their attributes are parsed into.
type Spec struct {
	Name       string      `json:"name"`
	Elements   []*Element  `json:"elements"`
	Attributes []Attribute `json:"attributes,omitempty"`
}

func attrUnmarshal(in []json.RawMessage) ([]Attribute, error) {
	out := make([]Attribute, 0)

	var tmpAttr struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		AttributeType string `json:"attribute_type"`
	}

	for _, attr := range in {
		if err := json.Unmarshal(attr, &tmpAttr); err != nil {
			return nil, err
		}

		switch tmpAttr.AttributeType {
		case "AttributeTypeString":
			a := &AttributeTypeString{}
			if err := json.Unmarshal(attr, &a); err != nil {
				return nil, err
			}
			out = append(out, a)
		case "AttributeTypeChar":
			a := &AttributeTypeChar{}
			if err := json.Unmarshal(attr, &a); err != nil {
				return nil, err
			}
			out = append(out, a)
		case "AttributeTypeNumber":
			a := &AttributeTypeNumber{}
			if err := json.Unmarshal(attr, &a); err != nil {
				return nil, err
			}
			out = append(out, a)
		case "AttributeTypeFloat":
			a := &AttributeTypeFloat{}
			if err := json.Unmarshal(attr, &a); err != nil {
				return nil, err
			}
			out = append(out, a)
		case "AttributeTypeBool":
			a := &AttributeTypeBool{}
			if err := json.Unmarshal(attr, &a); err != nil {
				return nil, err
			}
			out = append(out, a)
		case "AttributeTypeEnum":
			a := &AttributeTypeEnum{}
			if err := json.Unmarshal(attr, &a); err != nil {
				return nil, err
			}
			out = append(out, a)
		case "AttributeTypeSST":
			a := &AttributeTypeSST{}
			if err := json.Unmarshal(attr, &a); err != nil {
				return nil, err
			}
			out = append(out, a)
		case "AttributeTypePrefixedCustom":
			a := &AttributeTypePrefixedCustom{}
			if err := json.Unmarshal(attr, &a); err != nil {
				return nil, err
			}
			out = append(out, a)
		default:
			return nil, fmt.Errorf("unknown attribute_type %q for attribute %q", tmpAttr.AttributeType, tmpAttr.Name)
		}
	}

	return out, nil
}

// UnmarshalJSON handles converting the marshaled json back into a Spec struct.
func (sp *Spec) UnmarshalJSON(b []byte) error {
	var tmp struct {
		Name       string            `json:"name"`
		Elements   []*Element        `json:"elements"`
		Attributes []json.RawMessage `json:"attributes,omitempty"`
	}

	if err := json.Unmarshal(b, &tmp); err != nil {
		return err
	}

	sp.Name = tmp.Name
	sp.Elements = tmp.Elements
	attrs, err := attrUnmarshal(tmp.Attributes)
	if err != nil {
		return err
	}
	sp.Attributes = attrs

	return nil
}

// Element represents an element ia specifications such as HTML or SVG.
// An element has attributes that are relative only to itself but also inherits any global attributes defined by the spec.
type Element struct {
	Tag         string      `json:"tag"`
	Description string      `json:"description,omitempty"`
	Attributes  []Attribute `json:"attributes,omitempty"`

	// A Void element has no children
	Void bool `json:"void,omitempty"`
	Text bool `json:"text,omitempty"`

	// The fields below were added after v0.2.0. They are all omitempty, so documents that do not use
	// them marshal exactly as before, and consumers built against older versions of this package
	// ignore them (encoding/json skips unknown object keys).

	// RawText marks elements whose text content is raw text in the HTML syntax (script, style): it
	// is not entity-decoded by the parser, so it must not be HTML-escaped when rendered.
	RawText bool `json:"raw_text,omitempty"`
	// Deprecated marks elements that are obsolete or deprecated in the source specification.
	Deprecated bool `json:"deprecated,omitempty"`
	// Interface is the DOM interface implemented by the element (e.g. HTMLAnchorElement).
	Interface string `json:"interface,omitempty"`
	// Categories lists the content categories of the element (e.g. "flow", "phrasing",
	// "interactive"), as given by the specification's element index, sorted.
	Categories []string `json:"categories,omitempty"`
}

// UnmarshalJSON handles converting the marshaled json back into an Element struct.
func (e *Element) UnmarshalJSON(b []byte) error {
	var tmp struct {
		Tag         string            `json:"tag"`
		Description string            `json:"description,omitempty"`
		Attributes  []json.RawMessage `json:"attributes,omitempty"`
		Void        bool              `json:"void,omitempty"`
		Text        bool              `json:"text,omitempty"`
		RawText     bool              `json:"raw_text,omitempty"`
		Deprecated  bool              `json:"deprecated,omitempty"`
		Interface   string            `json:"interface,omitempty"`
		Categories  []string          `json:"categories,omitempty"`
	}

	if err := json.Unmarshal(b, &tmp); err != nil {
		return err
	}

	e.Tag = tmp.Tag
	e.Description = tmp.Description
	e.Void = tmp.Void
	e.Text = tmp.Text
	e.RawText = tmp.RawText
	e.Deprecated = tmp.Deprecated
	e.Interface = tmp.Interface
	e.Categories = tmp.Categories
	attrs, err := attrUnmarshal(tmp.Attributes)
	if err != nil {
		return err
	}
	e.Attributes = attrs

	return nil
}

// Attribute defines the interface that all attributes must conform to.
// The unexported isAttr method means only this package can implement it, so methods can be added
// without breaking other modules.
type Attribute interface {
	isAttr()
	GetName() string
	// IsDeprecated reports whether the attribute is deprecated or obsolete in its specification.
	IsDeprecated() bool
}

// simpleAttrJSON is the wire form shared by every attribute type without extra fields. Field order
// matches the historical output so existing documents round-trip byte for byte; the newer fields
// are omitempty.
type simpleAttrJSON struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	URL           bool   `json:"url,omitempty"`
	Deprecated    bool   `json:"deprecated,omitempty"`
	AttributeType string `json:"attribute_type"`
}

// AttributeTypeString allows for setting string values on an attribute.
type AttributeTypeString struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// URL marks string attributes whose value is a URL (href, src, action, ...), so generators can
	// apply URL-specific handling such as scheme filtering. Added after v0.2.0.
	URL bool `json:"url,omitempty"`
	// Deprecated marks the attribute as deprecated or obsolete. Added after v0.2.0.
	Deprecated bool `json:"deprecated,omitempty"`
}

func (a AttributeTypeString) isAttr() {}

func (a AttributeTypeString) GetName() string {
	return a.Name
}

func (a AttributeTypeString) IsDeprecated() bool {
	return a.Deprecated
}

func (a AttributeTypeString) MarshalJSON() ([]byte, error) {
	return json.Marshal(&simpleAttrJSON{
		Name:          a.Name,
		Description:   a.Description,
		URL:           a.URL,
		Deprecated:    a.Deprecated,
		AttributeType: "AttributeTypeString",
	})
}

// AttributeTypeChar allows for setting a single char or rune value on an attribute.
type AttributeTypeChar struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Deprecated marks the attribute as deprecated or obsolete. Added after v0.2.0.
	Deprecated bool `json:"deprecated,omitempty"`
}

func (a AttributeTypeChar) isAttr() {}

func (a AttributeTypeChar) GetName() string {
	return a.Name
}

func (a AttributeTypeChar) IsDeprecated() bool {
	return a.Deprecated
}

func (a AttributeTypeChar) MarshalJSON() ([]byte, error) {
	return json.Marshal(&simpleAttrJSON{
		Name:          a.Name,
		Description:   a.Description,
		Deprecated:    a.Deprecated,
		AttributeType: "AttributeTypeChar",
	})
}

// AttributeTypeNumber allows for setting integer values on an attribute.
type AttributeTypeNumber struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Deprecated marks the attribute as deprecated or obsolete. Added after v0.2.0.
	Deprecated bool `json:"deprecated,omitempty"`
}

func (a AttributeTypeNumber) isAttr() {}

func (a AttributeTypeNumber) GetName() string {
	return a.Name
}

func (a AttributeTypeNumber) IsDeprecated() bool {
	return a.Deprecated
}

func (a AttributeTypeNumber) MarshalJSON() ([]byte, error) {
	return json.Marshal(&simpleAttrJSON{
		Name:          a.Name,
		Description:   a.Description,
		Deprecated:    a.Deprecated,
		AttributeType: "AttributeTypeNumber",
	})
}

// AttributeTypeFloat allows for setting float values on an attribute.
type AttributeTypeFloat struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Deprecated marks the attribute as deprecated or obsolete. Added after v0.2.0.
	Deprecated bool `json:"deprecated,omitempty"`
}

func (a AttributeTypeFloat) isAttr() {}

func (a AttributeTypeFloat) GetName() string {
	return a.Name
}

func (a AttributeTypeFloat) IsDeprecated() bool {
	return a.Deprecated
}

func (a AttributeTypeFloat) MarshalJSON() ([]byte, error) {
	return json.Marshal(&simpleAttrJSON{
		Name:          a.Name,
		Description:   a.Description,
		Deprecated:    a.Deprecated,
		AttributeType: "AttributeTypeFloat",
	})
}

// AttributeTypeBool allows for setting boolean values on an attribute.
type AttributeTypeBool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Deprecated marks the attribute as deprecated or obsolete. Added after v0.2.0.
	Deprecated bool `json:"deprecated,omitempty"`
}

func (a AttributeTypeBool) isAttr() {}

func (a AttributeTypeBool) GetName() string {
	return a.Name
}

func (a AttributeTypeBool) IsDeprecated() bool {
	return a.Deprecated
}

func (a AttributeTypeBool) MarshalJSON() ([]byte, error) {
	return json.Marshal(&simpleAttrJSON{
		Name:          a.Name,
		Description:   a.Description,
		Deprecated:    a.Deprecated,
		AttributeType: "AttributeTypeBool",
	})
}

// AttributeTypeEnum allows for setting enumerated values on an attribute.
// AllowEmpty field accounts for if spec allows for attributes to be empty.
// AllowCustom field accounts for if the spec allows for a set of specific enums but may also allow custom values.
// Allow field should contain a list of allowed enum values as defined by the spec.
type AttributeTypeEnum struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Allowed     map[string]struct{} `json:"allowed"`
	AllowCustom bool                `json:"allow_custom"`
	AllowEmpty  bool                `json:"allow_empty"`
	// Deprecated marks the attribute as deprecated or obsolete. Added after v0.2.0.
	Deprecated bool `json:"deprecated,omitempty"`
}

func (a AttributeTypeEnum) isAttr() {}

func (a AttributeTypeEnum) GetName() string {
	return a.Name
}

func (a AttributeTypeEnum) IsDeprecated() bool {
	return a.Deprecated
}

// AllowedSorted returns the allowed enum values in sorted order, giving callers
// a deterministic alternative to iterating the Allowed map.
func (a AttributeTypeEnum) AllowedSorted() []string {
	out := make([]string, 0, len(a.Allowed))
	for k := range a.Allowed {
		out = append(out, k)
	}
	sort.Strings(out)

	return out
}

// MarshalJSON emits the allowed values as an object with keys in sorted order,
// so the output is deterministic. (encoding/json already sorts map keys, but this
// is made explicit and kept independent of the map type's encoding.)
func (a AttributeTypeEnum) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Name          string              `json:"name"`
		Description   string              `json:"description,omitempty"`
		Allowed       map[string]struct{} `json:"allowed"`
		AllowEmpty    bool                `json:"allow_empty"`
		AllowCustom   bool                `json:"allow_custom"`
		Deprecated    bool                `json:"deprecated,omitempty"`
		AttributeType string              `json:"attribute_type"`
	}{
		Name:          a.Name,
		Description:   a.Description,
		Allowed:       a.Allowed,
		AllowEmpty:    a.AllowEmpty,
		AllowCustom:   a.AllowCustom,
		Deprecated:    a.Deprecated,
		AttributeType: "AttributeTypeEnum",
	})
}

// AttributeTypeSST allows for setting space-separated tokens values on an attribute.
type AttributeTypeSST struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Deprecated marks the attribute as deprecated or obsolete. Added after v0.2.0.
	Deprecated bool `json:"deprecated,omitempty"`
}

func (a AttributeTypeSST) isAttr() {}

func (a AttributeTypeSST) GetName() string {
	return a.Name
}

func (a AttributeTypeSST) IsDeprecated() bool {
	return a.Deprecated
}

func (a AttributeTypeSST) MarshalJSON() ([]byte, error) {
	return json.Marshal(&simpleAttrJSON{
		Name:          a.Name,
		Description:   a.Description,
		Deprecated:    a.Deprecated,
		AttributeType: "AttributeTypeSST",
	})
}

// AttributeTypePrefixedCustom provides support for attributes like `data-<user defined>`
type AttributeTypePrefixedCustom struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Deprecated marks the attribute as deprecated or obsolete. Added after v0.2.0.
	Deprecated bool `json:"deprecated,omitempty"`
}

func (a AttributeTypePrefixedCustom) isAttr() {}

func (a AttributeTypePrefixedCustom) GetName() string {
	return a.Name
}

func (a AttributeTypePrefixedCustom) IsDeprecated() bool {
	return a.Deprecated
}

func (a AttributeTypePrefixedCustom) MarshalJSON() ([]byte, error) {
	return json.Marshal(&simpleAttrJSON{
		Name:          a.Name,
		Description:   a.Description,
		Deprecated:    a.Deprecated,
		AttributeType: "AttributeTypePrefixedCustom",
	})
}
