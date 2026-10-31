package webref

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/go-htemel/spec"
)

// goIdent mirrors htemel's internal/generators.goIdent: every run of non-letter, non-digit
// characters is a word break, the first letter of each word is upper-cased, interior capitals are
// kept, and a leading digit gets a "V" prefix.
func goIdent(s string) string {
	var b strings.Builder
	upper := true
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			upper = true
			continue
		}
		if upper {
			r = unicode.ToUpper(r)
			upper = false
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" || unicode.IsDigit(rune(out[0])) {
		out = "V" + out
	}

	return out
}

var (
	elementMethods   = []string{"With", "AddClass", "ToggleClass", "Attr", "AttrUnsafe", "BoolAttr", "Attrs", "SetAttr", "SetAttrUnsafe", "Render"}
	containerMethods = []string{"Children", "Text", "Textf", "If", "IfElse", "IfThen"}
)

// CheckNames reports Go identifiers or file names that htemel's generator would produce more than
// once for sp (it mirrors internal/generators.CheckNames there, including the merge of global
// attributes into every element), so a candidate document is known to be generatable before it
// is handed over. Duplicate attribute names on one element are also reported.
func CheckNames(sp *spec.Spec) error {
	var problems []string
	pkg := map[string]string{}
	files := map[string]string{}
	declare := func(name, what string) {
		if prev, ok := pkg[name]; ok {
			problems = append(problems, fmt.Sprintf("package-level %s is declared by both %s and %s", name, prev, what))
			return
		}
		pkg[name] = what
	}

	for _, e := range sp.Elements {
		attrs := append([]spec.Attribute(nil), e.Attributes...)
		own := map[string]bool{}
		for _, a := range e.Attributes {
			if own[a.GetName()] {
				problems = append(problems, fmt.Sprintf("<%s>: attribute %q listed twice", e.Tag, a.GetName()))
			}
			own[a.GetName()] = true
		}
		for _, g := range sp.Attributes {
			if !own[g.GetName()] {
				attrs = append(attrs, g)
			}
		}

		tag := goIdent(e.Tag)
		where := "<" + e.Tag + ">"
		file := strings.ToLower(e.Tag) + ".go"
		if prev, ok := files[file]; ok {
			problems = append(problems, fmt.Sprintf("file %s is produced by both <%s> and %s", file, prev, where))
		} else {
			files[file] = e.Tag
		}
		declare(tag, where)
		declare(tag+"If", where)
		declare(tag+"Element", where)
		if !e.Void {
			declare(tag+"Ternary", where)
		}

		methods := map[string]string{}
		method := func(name, what string) {
			if prev, ok := methods[name]; ok {
				problems = append(problems, fmt.Sprintf("%s: method %s is declared by both %s and %s", where, name, prev, what))
				return
			}
			methods[name] = what
		}
		for _, m := range elementMethods {
			method(m, "the built-in "+m)
		}
		if !e.Void {
			for _, m := range containerMethods {
				method(m, "the built-in "+m)
			}
		}
		for _, attr := range attrs {
			name := attr.GetName()
			what := fmt.Sprintf("attribute %q", name)
			method(goIdent(name), what)
			switch a := attr.(type) {
			case *spec.AttributeTypePrefixedCustom:
				method(goIdent(name)+"Unsafe", what)
			case *spec.AttributeTypeEnum:
				typeName := tag + goIdent(name)
				ew := where + " " + what
				declare(typeName, ew)
				for _, v := range a.AllowedSorted() {
					declare(typeName+goIdent(v), fmt.Sprintf("%s value %q", ew, v))
				}
				if a.AllowEmpty {
					declare(typeName+"Empty", ew+" (empty value)")
				}
				if a.AllowCustom {
					declare(typeName+"Custom", ew+" (custom value)")
				}
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("generated names collide:\n  %s", strings.Join(problems, "\n  "))
	}

	return nil
}
