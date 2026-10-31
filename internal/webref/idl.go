// Package webref builds spec documents from machine-readable web platform data: the element lists
// and Web IDL published by w3c/webref, MDN's browser-compat-data, the WHATWG HTML index tables and a
// hand-maintained overlay for the judgment calls none of those sources can make.
package webref

import (
	"fmt"
	"sort"
	"strings"
)

// IDLAttribute is one `attribute` member of an interface or mixin.
type IDLAttribute struct {
	Name     string
	Type     string            // the IDL type as written, whitespace-normalised, e.g. "unsigned long" or "DOMString?"
	ReadOnly bool              // declared readonly
	ExtAttrs map[string]string // extended attributes; value is "" when the attribute has no argument
	Partial  bool              // declared in a partial interface
	File     string            // IDL file the member came from
	Owner    string            // interface or mixin that declares the member
}

// ReflectedName returns the content attribute the member reflects and whether it reflects one, using
// the HTML standard's [Reflect], [Reflect=name], [ReflectURL], [ReflectSetter], ... extended
// attributes. The content attribute name defaults to the lower-cased IDL name.
func (a IDLAttribute) ReflectedName() (string, bool) {
	if v, ok := a.ExtAttrs["Reflect"]; ok {
		if v != "" {
			return strings.Trim(v, `"`), true
		}
		return strings.ToLower(a.Name), true
	}
	for k := range a.ExtAttrs {
		if strings.HasPrefix(k, "Reflect") && k != "ReflectDefault" && k != "ReflectRange" {
			return strings.ToLower(a.Name), true
		}
	}

	return "", false
}

// BaseType returns the type without nullability, inline extended attributes or union parentheses
// collapsed, e.g. "[LegacyNullToEmptyString] DOMString" -> "DOMString", "DOMString?" -> "DOMString".
func (a IDLAttribute) BaseType() string {
	t := a.Type
	for strings.HasPrefix(t, "[") {
		if i := strings.Index(t, "]"); i >= 0 {
			t = strings.TrimSpace(t[i+1:])
		} else {
			break
		}
	}
	t = strings.TrimSuffix(t, "?")
	t = strings.TrimPrefix(t, "unrestricted ")

	return t
}

// IDLInterface collects an interface or interface mixin across all its (partial) definitions.
type IDLInterface struct {
	Name       string
	Mixin      bool
	Inherits   string
	Attributes []IDLAttribute
}

// IDL is a parsed set of IDL files. Only interfaces, interface mixins, partials of both and includes
// statements are kept; everything else (dictionaries, enums, callbacks, namespaces, operations,
// constants) is skipped. That is all the generator needs.
type IDL struct {
	Interfaces map[string]*IDLInterface
	Includes   map[string][]string // interface -> mixins it includes
}

// NewIDL returns an empty IDL set.
func NewIDL() *IDL {
	return &IDL{Interfaces: map[string]*IDLInterface{}, Includes: map[string][]string{}}
}

// Parse adds the definitions in src (the contents of file) to the set. The parser is deliberately
// tolerant: it splits the source into top-level definitions by brace and semicolon depth and only
// interprets the shapes listed on IDL; anything it does not recognise is ignored.
func (idl *IDL) Parse(file, src string) error {
	src = stripComments(src)
	defs, err := splitTopLevel(src)
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	for _, d := range defs {
		_, d.header = splitExtAttrs(d.header)
		f := strings.Fields(d.header)
		if len(f) == 0 {
			continue
		}
		if d.body == "" {
			// `A includes B`
			if len(f) == 3 && f[1] == "includes" {
				if !contains(idl.Includes[f[0]], f[2]) {
					idl.Includes[f[0]] = append(idl.Includes[f[0]], f[2])
				}
			}
			continue
		}
		partial := false
		if f[0] == "partial" {
			partial = true
			f = f[1:]
		}
		if len(f) < 2 || f[0] != "interface" {
			continue // dictionary, enum, namespace, callback interface, ...
		}
		f = f[1:]
		mixin := false
		if f[0] == "mixin" {
			mixin = true
			f = f[1:]
		}
		if len(f) == 0 {
			continue
		}
		header := strings.Join(f, " ")
		name, inherits := header, ""
		if i := strings.Index(header, ":"); i >= 0 {
			name, inherits = strings.TrimSpace(header[:i]), strings.TrimSpace(header[i+1:])
		}
		it := idl.Interfaces[name]
		if it == nil {
			it = &IDLInterface{Name: name, Mixin: mixin}
			idl.Interfaces[name] = it
		}
		if inherits != "" {
			it.Inherits = inherits
		}
		for _, m := range splitMembers(d.body) {
			if a, ok := parseAttribute(m); ok {
				a.Partial = partial
				a.File = file
				a.Owner = name
				it.Attributes = append(it.Attributes, a)
			}
		}
	}

	return nil
}

// Own returns the attributes declared directly on name and on the mixins it includes (not inherited
// ones), in declaration order.
func (idl *IDL) Own(name string) []IDLAttribute {
	var out []IDLAttribute
	if it := idl.Interfaces[name]; it != nil {
		out = append(out, it.Attributes...)
	}
	mixins := append([]string(nil), idl.Includes[name]...)
	sort.Strings(mixins)
	for _, m := range mixins {
		if it := idl.Interfaces[m]; it != nil {
			out = append(out, it.Attributes...)
		}
	}

	return out
}

// Chain returns name followed by its ancestors, stopping before (and excluding) any interface in
// stop, e.g. Chain("HTMLVideoElement", "HTMLElement") = [HTMLVideoElement HTMLMediaElement].
func (idl *IDL) Chain(name string, stop ...string) []string {
	var out []string
	seen := map[string]bool{}
	for name != "" && !contains(stop, name) && !seen[name] {
		seen[name] = true
		out = append(out, name)
		it := idl.Interfaces[name]
		if it == nil {
			break
		}
		name = it.Inherits
	}

	return out
}

type definition struct {
	header string // text before `{` (or the whole statement for bodiless definitions)
	body   string // text between the outer braces; "" for statements such as includes
}

func stripComments(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '"':
			j := strings.IndexByte(s[i+1:], '"')
			if j < 0 {
				b.WriteString(s[i:])
				return b.String()
			}
			b.WriteString(s[i : i+j+2])
			i += j + 1
		case strings.HasPrefix(s[i:], "//"):
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				return b.String()
			}
			i += j - 1
		case strings.HasPrefix(s[i:], "/*"):
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				return b.String()
			}
			b.WriteByte(' ')
			i += j + 3
		default:
			b.WriteByte(s[i])
		}
	}

	return b.String()
}

// splitTopLevel splits IDL source into definitions terminated by `;` at nesting depth zero.
func splitTopLevel(s string) ([]definition, error) {
	var out []definition
	depth, start, open := 0, 0, -1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '(', '[', '<':
			if s[i] == '{' && depth == 0 {
				open = i
			}
			depth++
		case '}', ')', ']', '>':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced %q at offset %d", s[i], i)
			}
		case ';':
			if depth != 0 {
				continue
			}
			stmt := s[start:i]
			d := definition{header: strings.TrimSpace(stmt)}
			if open >= start {
				closeIdx := strings.LastIndexByte(stmt, '}')
				if closeIdx < 0 {
					return nil, fmt.Errorf("definition at offset %d has no closing brace", start)
				}
				d.header = strings.TrimSpace(s[start:open])
				d.body = s[open+1 : start+closeIdx]
			}
			out = append(out, d)
			start, open = i+1, -1
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced brackets at end of input")
	}

	return out, nil
}

// splitMembers splits an interface body into member declarations.
func splitMembers(body string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '(', '[', '<', '{':
			depth++
		case ')', ']', '>', '}':
			depth--
		case ';':
			if depth == 0 {
				if m := strings.TrimSpace(body[start:i]); m != "" {
					out = append(out, m)
				}
				start = i + 1
			}
		}
	}

	return out
}

// splitExtAttrs removes a leading `[...]` extended attribute list and parses it.
func splitExtAttrs(s string) (map[string]string, string) {
	s = strings.TrimSpace(s)
	ext := map[string]string{}
	if !strings.HasPrefix(s, "[") {
		return ext, s
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
			if depth == 0 && s[i] == ']' {
				for _, part := range splitComma(s[1:i]) {
					k, v, _ := strings.Cut(part, "=")
					ext[strings.TrimSpace(k)] = strings.TrimSpace(v)
				}
				return ext, strings.TrimSpace(s[i+1:])
			}
		}
	}

	return ext, s
}

func splitComma(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '<':
			depth++
		case ')', ']', '>':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if rest := strings.TrimSpace(s[start:]); rest != "" {
		out = append(out, rest)
	}

	return out
}

// parseAttribute recognises `[ext] (static|stringifier|inherit)* readonly? attribute Type name`.
func parseAttribute(m string) (IDLAttribute, bool) {
	ext, rest := splitExtAttrs(m)
	f := strings.Fields(rest)
	ro := false
	for len(f) > 0 {
		switch f[0] {
		case "static":
			return IDLAttribute{}, false // static attributes are not element state
		case "stringifier", "inherit":
			f = f[1:]
			continue
		case "readonly":
			ro = true
			f = f[1:]
			continue
		}
		break
	}
	if len(f) < 3 || f[0] != "attribute" {
		return IDLAttribute{}, false
	}
	name := strings.TrimPrefix(f[len(f)-1], "_")

	return IDLAttribute{
		Name:     name,
		Type:     strings.Join(f[1:len(f)-1], " "),
		ReadOnly: ro,
		ExtAttrs: ext,
	}, true
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}

	return false
}
