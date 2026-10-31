package webref

import (
	"reflect"
	"testing"
)

const testIDL = `
// comment with interface Bogus { attribute long nope; };
[Exposed=Window]
interface HTMLFooElement : HTMLBarElement {
  [HTMLConstructor] constructor();
  [CEReactions, Reflect] attribute DOMString target;
  [CEReactions, Reflect="http-equiv"] attribute DOMString httpEquiv;
  [CEReactions, ReflectURL] attribute USVString src;
  [CEReactions, Reflect, ReflectDefault=1, ReflectRange=(1, 1000)] attribute unsigned long span;
  [SameObject, PutForwards=value, Reflect="rel"] readonly attribute DOMTokenList relList;
  [CEReactions] attribute [LegacyNullToEmptyString] DOMString value;
  /* block
     comment */
  static attribute long ignored;
  undefined click(optional FocusOptions options = {});
  readonly attribute (boolean or DOMString)? mixed;
};
interface HTMLBarElement : HTMLElement {
  [CEReactions, Reflect] attribute boolean disabled;
};
partial interface HTMLFooElement {
  [CEReactions, Reflect] attribute DOMString align;
};
interface mixin Linky {
  [CEReactions, ReflectSetter] stringifier attribute USVString href;
};
HTMLFooElement includes Linky;
dictionary D { long x = 1; };
enum E { "a", "b" };
`

func TestIDLParse(t *testing.T) {
	idl := NewIDL()
	if err := idl.Parse("test.idl", testIDL); err != nil {
		t.Fatal(err)
	}
	if _, ok := idl.Interfaces["Bogus"]; ok {
		t.Error("parsed an interface from inside a comment")
	}
	if got := idl.Chain("HTMLFooElement", "HTMLElement"); !reflect.DeepEqual(got, []string{"HTMLFooElement", "HTMLBarElement"}) {
		t.Errorf("Chain = %v", got)
	}

	type row struct {
		name, typ, content  string
		reflected, ro, part bool
	}
	var got []row
	for _, a := range idl.Own("HTMLFooElement") {
		c, ok := a.ReflectedName()
		got = append(got, row{a.Name, a.BaseType(), c, ok, a.ReadOnly, a.Partial})
	}
	want := []row{
		{"target", "DOMString", "target", true, false, false},
		{"httpEquiv", "DOMString", "http-equiv", true, false, false},
		{"src", "USVString", "src", true, false, false},
		{"span", "unsigned long", "span", true, false, false},
		{"relList", "DOMTokenList", "rel", true, true, false},
		{"value", "DOMString", "", false, false, false},
		{"mixed", "(boolean or DOMString)", "", false, true, false},
		{"align", "DOMString", "align", true, false, true},
		{"href", "USVString", "href", true, false, false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("attributes:\ngot  %+v\nwant %+v", got, want)
	}
}
