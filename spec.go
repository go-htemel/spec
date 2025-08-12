package spec

import (
	"encoding/json"
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
		case "AttributeType":
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
}

// UnmarshalJSON handles converting the marshaled json back into an Element struct.
func (e *Element) UnmarshalJSON(b []byte) error {
	var tmp struct {
		Tag         string            `json:"tag"`
		Description string            `json:"description,omitempty"`
		Attributes  []json.RawMessage `json:"attributes,omitempty"`
		Void        bool              `json:"void,omitempty"`
	}

	if err := json.Unmarshal(b, &tmp); err != nil {
		return err
	}

	e.Tag = tmp.Tag
	e.Description = tmp.Description
	e.Void = tmp.Void
	attrs, err := attrUnmarshal(tmp.Attributes)
	if err != nil {
		return err
	}
	e.Attributes = attrs

	return nil
}

// Attribute defines the interface that all attributes must conform to.
type Attribute interface {
	isAttr()
	GetName() string
}

// AttributeTypeString allows for setting string values on an attribute.
type AttributeTypeString struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (a AttributeTypeString) isAttr() {}

func (a AttributeTypeString) GetName() string {
	return a.Name
}

func (a AttributeTypeString) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Name          string `json:"name"`
		Description   string `json:"description,omitempty"`
		AttributeType string `json:"attribute_type"`
	}{
		Name:          a.Name,
		Description:   a.Description,
		AttributeType: "AttributeTypeString",
	})
}

// AttributeTypeChar allows for setting a single char or rune value on an attribute.
type AttributeTypeChar struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (a AttributeTypeChar) isAttr() {}

func (a AttributeTypeChar) GetName() string {
	return a.Name
}

func (a AttributeTypeChar) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Name          string `json:"name"`
		Description   string `json:"description,omitempty"`
		AttributeType string `json:"attribute_type"`
	}{
		Name:          a.Name,
		Description:   a.Description,
		AttributeType: "AttributeTypeChar",
	})
}

// AttributeTypeNumber allows for setting integer values on an attribute.
type AttributeTypeNumber struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (a AttributeTypeNumber) isAttr() {}

func (a AttributeTypeNumber) GetName() string {
	return a.Name
}

func (a AttributeTypeNumber) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Name          string `json:"name"`
		Description   string `json:"description,omitempty"`
		AttributeType string `json:"attribute_type"`
	}{
		Name:          a.Name,
		Description:   a.Description,
		AttributeType: "AttributeTypeNumber",
	})
}

// AttributeTypeFloat allows for setting float values on an attribute.
type AttributeTypeFloat struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (a AttributeTypeFloat) isAttr() {}

func (a AttributeTypeFloat) GetName() string {
	return a.Name
}

func (a AttributeTypeFloat) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Name          string `json:"name"`
		Description   string `json:"description,omitempty"`
		AttributeType string `json:"attribute_type"`
	}{
		Name:          a.Name,
		Description:   a.Description,
		AttributeType: "AttributeTypeFloat",
	})
}

// AttributeTypeBool allows for setting boolean values on an attribute.
type AttributeTypeBool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (a AttributeTypeBool) isAttr() {}

func (a AttributeTypeBool) GetName() string {
	return a.Name
}

func (a AttributeTypeBool) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Name          string `json:"name"`
		Description   string `json:"description,omitempty"`
		AttributeType string `json:"attribute_type"`
	}{
		Name:          a.Name,
		Description:   a.Description,
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
}

func (a AttributeTypeEnum) isAttr() {}

func (a AttributeTypeEnum) GetName() string {
	return a.Name
}

func (a AttributeTypeEnum) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Name          string              `json:"name"`
		Description   string              `json:"description,omitempty"`
		Allowed       map[string]struct{} `json:"allowed"`
		AllowEmpty    bool                `json:"allow_empty"`
		AllowCustom   bool                `json:"allow_custom"`
		AttributeType string              `json:"attribute_type"`
	}{
		Name:          a.Name,
		Description:   a.Description,
		Allowed:       a.Allowed,
		AllowEmpty:    a.AllowEmpty,
		AllowCustom:   a.AllowCustom,
		AttributeType: "AttributeTypeEnum",
	})
}

// AttributeTypeSST allows for setting space-separated tokens values on an attribute.
type AttributeTypeSST struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (a AttributeTypeSST) isAttr() {}

func (a AttributeTypeSST) GetName() string {
	return a.Name
}

func (a AttributeTypeSST) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Name          string `json:"name"`
		Description   string `json:"description,omitempty"`
		AttributeType string `json:"attribute_type"`
	}{
		Name:          a.Name,
		Description:   a.Description,
		AttributeType: "AttributeTypeSST",
	})
}

// AttributeTypePrefixedCustom provides support for attributes like `data-<user defined>`
type AttributeTypePrefixedCustom struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (a AttributeTypePrefixedCustom) isAttr() {}

func (a AttributeTypePrefixedCustom) GetName() string {
	return a.Name
}

func (a AttributeTypePrefixedCustom) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Name          string `json:"name"`
		Description   string `json:"description,omitempty"`
		AttributeType string `json:"attribute_type"`
	}{
		Name:          a.Name,
		Description:   a.Description,
		AttributeType: "AttributeTypePrefixedCustom",
	})
}
