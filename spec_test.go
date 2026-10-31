package spec

import (
	"encoding/json"
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
