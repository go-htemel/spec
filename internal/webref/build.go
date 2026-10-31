package webref

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-htemel/spec"
)

// Config describes how one spec document is assembled.
type Config struct {
	Name          string   // key for overlay/<Name>.json, specs/<Name>.json and specs/candidates/<Name>.json
	SpecName      string   // value of the document's "name" field (kept identical to the legacy files)
	ElementFiles  []string // webref element files, in priority order (the first definition of a tag wins)
	IDLFiles      []string // IDL files whose members may become attributes
	BaseInterface string   // interface holding the global attributes; inheritance stops here
	BCD           string   // browser-compat-data namespace
	// SVGConventions: SVGAnimated* members are content attributes, names keep their case.
	SVGConventions bool
	// SharedMixins: mixins whose members count even though they are declared in another spec's IDL
	// (HTMLOrSVGOrMathMLElement lives in html.idl).
	SharedMixins []string
	// PartialIsObsolete: members of partial interfaces are obsolete features (true for html.idl,
	// which only uses partial HTML*Element interfaces in its "obsolete features" section).
	PartialIsObsolete bool
	UseWHATWG         bool
}

// Configs lists the documents the generator can build.
var Configs = []Config{
	{
		Name: "html", SpecName: "HTML",
		ElementFiles:      []string{"html"},
		IDLFiles:          []string{"html.idl"},
		BaseInterface:     "HTMLElement",
		BCD:               "html",
		PartialIsObsolete: true,
		UseWHATWG:         true,
	},
	{
		Name: "svg", SpecName: "svg",
		ElementFiles:   []string{"SVG2", "filter-effects-1", "css-masking-1", "svg-animations", "svg-paths"},
		IDLFiles:       []string{"SVG.idl", "filter-effects.idl", "css-masking.idl", "svg-animations.idl", "svg-paths.idl"},
		BaseInterface:  "SVGElement",
		BCD:            "svg",
		SVGConventions: true,
		SharedMixins:   []string{"HTMLOrSVGOrMathMLElement"},
	},
	{
		Name: "mathml", SpecName: "mathml",
		ElementFiles:  []string{"mathml-core"},
		IDLFiles:      []string{"mathml-core.idl"},
		BaseInterface: "MathMLElement",
		BCD:           "mathml",
		SharedMixins:  []string{"HTMLOrSVGOrMathMLElement"},
	},
}

// ConfigByName returns the named config.
func ConfigByName(name string) (Config, bool) {
	for _, c := range Configs {
		if c.Name == name {
			return c, true
		}
	}

	return Config{}, false
}

// Options are generation switches that do not belong in committed data.
type Options struct {
	IncludeObsolete bool // keep obsolete elements and attributes (marked deprecated)
}

// Type-source labels recorded in the provenance file. Strong sources describe the value syntax;
// weak ones only establish that the attribute exists.
const (
	srcIDL           = "idl"             // reflected IDL type ([Reflect], [ReflectURL], ...)
	srcIDLSetter     = "idl-setter"      // [ReflectSetter]: reflected on set only; DOMString ones are often enumerated
	srcIDLUnreflect  = "idl-unreflected" // IDL attribute with custom getter; type says little about the content attribute
	srcSVGEnum       = "idl-svg-enum"    // SVGAnimatedEnumeration: keywords not recoverable from IDL
	srcDefault       = "default"         // no source says anything about the type; String assumed
	srcWHATWG        = "whatwg"
	srcOverlay       = "overlay"
	srcOverlayPend   = "overlay-pending"
	srcBCDValues     = "bcd-values"
	srcBCD           = "bcd"
	srcIDLExist      = "idl"
	srcIDLEventExist = "idl-event-handler"
)

// WeakType reports whether a type source only guesses at the value syntax.
func WeakType(s string) bool {
	switch s {
	case srcIDLSetter, srcIDLUnreflect, srcSVGEnum, srcDefault:
		return true
	}

	return false
}

type attr struct {
	Name        string
	Kind        string
	URL         bool
	Allowed     []string
	AllowEmpty  bool
	AllowCustom bool
	Deprecated  bool
	Obsolete    bool
	Source      string // what established that the attribute exists
	TypeSource  string // what determined Kind
	Reason      string // overlay reason, if any
}

func (a *attr) same(b *attr) bool {
	return a.Kind == b.Kind && a.URL == b.URL && a.AllowEmpty == b.AllowEmpty && a.AllowCustom == b.AllowCustom &&
		strings.Join(a.Allowed, "\x00") == strings.Join(b.Allowed, "\x00")
}

// AttrProvenance explains one generated attribute.
type AttrProvenance struct {
	Kind       string `json:"kind"`
	Source     string `json:"source"`
	TypeSource string `json:"type_source"`
	Reason     string `json:"reason,omitempty"`
}

// ElementProvenance explains one generated element.
type ElementProvenance struct {
	Source   string `json:"source"`
	FlagsSrc string `json:"flags_source"`
}

// Provenance is written next to each candidate document.
type Provenance struct {
	Spec       string                       `json:"spec"`
	WHATWG     bool                         `json:"whatwg_index_used"`
	Elements   map[string]ElementProvenance `json:"elements"`
	Attributes map[string]AttrProvenance    `json:"attributes"` // "<tag>/<attr>", "@global/<attr>"
	Skipped    map[string]string            `json:"skipped,omitempty"`
	Warnings   []string                     `json:"warnings,omitempty"`
}

// Build assembles one spec document.
func Build(cfg Config, src *Sources, opt Options) (*spec.Spec, *Provenance, error) {
	ov := src.Overlays[cfg.Name]
	if ov == nil {
		ov = &Overlay{}
	}
	b := &builder{cfg: cfg, src: src, ov: ov, opt: opt,
		bcd: src.BCD[cfg.BCD],
		prov: &Provenance{
			Spec:       cfg.Name,
			Elements:   map[string]ElementProvenance{},
			Attributes: map[string]AttrProvenance{},
			Skipped:    map[string]string{},
		},
	}
	if b.bcd == nil {
		b.bcd = &BCDNamespace{}
	}
	if cfg.UseWHATWG && src.WHATWG != nil {
		b.whatwg = src.WHATWG
		b.prov.WHATWG = true
	}

	return b.build()
}

type builder struct {
	cfg    Config
	src    *Sources
	ov     *Overlay
	opt    Options
	bcd    *BCDNamespace
	whatwg *WHATWGIndex
	prov   *Provenance
}

type elementDef struct {
	tag, iface string
	obsolete   bool
	file       string
}

func (b *builder) build() (*spec.Spec, *Provenance, error) {
	var defs []elementDef
	seen := map[string]bool{}
	for _, f := range b.cfg.ElementFiles {
		ef := b.src.Elements[f]
		if ef == nil {
			return nil, nil, fmt.Errorf("%s: missing webref element file %s.json", b.cfg.Name, f)
		}
		for _, e := range ef.Elements {
			if seen[e.Name] {
				continue
			}
			seen[e.Name] = true
			if e.Obsolete && !b.opt.IncludeObsolete {
				b.prov.Skipped["<"+e.Name+">"] = "obsolete element (webref)"
				continue
			}
			defs = append(defs, elementDef{tag: e.Name, iface: e.Interface, obsolete: e.Obsolete, file: f})
		}
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].tag < defs[j].tag })

	globals := b.globals()
	out := &spec.Spec{Name: b.cfg.SpecName}
	for _, name := range sortedKeys(globals) {
		a := globals[name]
		out.Attributes = append(out.Attributes, b.toSpec("@global", a, ""))
	}

	for _, d := range defs {
		attrs := b.elementAttrs(d)
		e := &spec.Element{
			Tag:        d.tag,
			Interface:  d.iface,
			Deprecated: d.obsolete,
		}
		if l := b.legacyElement(d.tag); l != nil {
			e.Description = l.Description
		}
		// Void is the HTML syntax's void element list, which the overlay carries: no index table
		// states it. The index's Children column only says whether the content model is empty, which
		// is also true of iframe, template and selectedcontent, so it can only cross-check one way.
		e.Void = contains(b.ov.Void, d.tag)
		if b.whatwg != nil {
			if w, ok := b.whatwg.Elements[d.tag]; ok {
				e.Categories = w.Categories
				if e.Description == "" {
					e.Description = w.Description
				}
				if e.Void && !w.EmptyContentModel() {
					b.warn("<%s>: in the overlay void list but the WHATWG index gives it children %q", d.tag, w.Children)
				}
			}
		}
		if len(b.ov.Text) > 0 {
			e.Text = contains(b.ov.Text, d.tag)
		} else {
			e.Text = !e.Void && !contains(b.ov.NoText, d.tag)
		}
		e.RawText = contains(b.ov.RawText, d.tag)
		b.prov.Elements[d.tag] = ElementProvenance{Source: "webref:" + d.file, FlagsSrc: "overlay"}

		for _, name := range sortedKeys(attrs) {
			a := attrs[name]
			if g, ok := globals[name]; ok && (a.same(g) || WeakType(a.TypeSource) && a.Source == srcBCD) {
				continue // the global already provides it
			}
			e.Attributes = append(e.Attributes, b.toSpec(d.tag, a, d.tag))
		}
		out.Elements = append(out.Elements, e)
	}

	for _, tag := range append(append(append(append([]string{}, b.ov.Void...), b.ov.NoText...), b.ov.Text...), b.ov.RawText...) {
		if !seen[tag] {
			b.warn("overlay names unknown element <%s>", tag)
		}
	}
	for _, scope := range b.ov.Scopes() {
		if scope != "@global" && scope != "*" && !seen[scope] {
			b.warn("overlay has attributes for unknown element <%s>", scope)
		}
	}
	sort.Strings(b.prov.Warnings)

	if err := CheckNames(out); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", b.cfg.Name, err)
	}

	return out, b.prov, nil
}

func (b *builder) warn(format string, args ...any) {
	b.prov.Warnings = append(b.prov.Warnings, fmt.Sprintf(format, args...))
}

// globals collects the attributes every element of the spec accepts.
func (b *builder) globals() map[string]*attr {
	attrs := map[string]*attr{}
	confirm := func(name string) bool {
		_, ok := b.bcd.GlobalAttributes[name]
		_, ok2 := b.ov.For("@global")[name]
		return ok || ok2 || b.whatwgGlobal(name)
	}
	b.addIDL(attrs, []string{b.cfg.BaseInterface}, confirm)

	for _, key := range sortedKeys(b.bcd.GlobalAttributes) {
		st := b.bcd.GlobalAttributes[key]
		if key == "data_attributes" {
			attrs["data"] = &attr{Name: "data", Kind: KindPrefixedCustom, Source: srcBCD, TypeSource: srcBCD}
			continue
		}
		b.addBCD(attrs, "@global", key, st)
	}

	if b.whatwg != nil {
		for _, wa := range b.whatwg.Attributes {
			if wa.Global {
				b.applyWHATWG(attrs, wa.Name, wa, true)
			}
		}
	}

	b.applyOverlay(attrs, "*", false)
	b.applyOverlay(attrs, "@global", true)
	b.dropObsolete(attrs, "@global")

	return attrs
}

func (b *builder) whatwgGlobal(name string) bool {
	if b.whatwg == nil {
		return false
	}
	for _, wa := range b.whatwg.Attributes {
		if wa.Global && wa.Name == name {
			return true
		}
	}

	return false
}

func (b *builder) elementAttrs(d elementDef) map[string]*attr {
	attrs := map[string]*attr{}
	feature := b.bcd.Elements[d.tag]
	confirm := func(name string) bool {
		if feature != nil {
			if _, ok := feature.Attributes[name]; ok {
				return true
			}
		}
		if _, ok := b.ov.For(d.tag)[name]; ok {
			return true
		}
		if b.whatwg != nil {
			return contains(b.whatwg.Elements[d.tag].Attributes, name)
		}
		return false
	}
	if d.iface != "" {
		b.addIDL(attrs, b.src.IDL.Chain(d.iface, b.cfg.BaseInterface, "Element"), confirm)
	}

	if feature != nil {
		for _, key := range sortedKeys(feature.Attributes) {
			b.addBCD(attrs, d.tag, key, feature.Attributes[key])
		}
		for _, tv := range b.ov.BCDValueKeys {
			tag, name, _ := strings.Cut(tv, "/")
			if tag != d.tag {
				continue
			}
			var values []string
			for _, key := range sortedKeys(feature.Attributes) {
				if v, ok := strings.CutPrefix(key, name+"_"); ok && feature.Attributes[key].StandardTrack {
					values = append(values, v)
				}
			}
			if len(values) > 0 {
				attrs[name] = &attr{Name: name, Kind: KindEnum, Allowed: values, Source: srcBCD, TypeSource: srcBCDValues}
			}
		}
	}

	if b.whatwg != nil {
		if w, ok := b.whatwg.Elements[d.tag]; ok {
			for _, name := range w.Attributes {
				found := false
				for _, wa := range b.whatwg.Attributes {
					if wa.Name == name && contains(wa.Elements, d.tag) {
						b.applyWHATWG(attrs, name, wa, true)
						found = true
					}
				}
				if !found && attrs[name] == nil {
					attrs[name] = &attr{Name: name, Kind: KindString, Source: srcWHATWG, TypeSource: srcDefault}
				}
			}
		}
		// The elements table's Attributes column lags behind the attributes table (dialog's omits
		// closedby), so rows naming this element refine attributes another source already found. They
		// do not add new ones: the attributes table also lists picture for width/height, which the
		// elements table rightly does not.
		for _, wa := range b.whatwg.Attributes {
			if contains(wa.Elements, d.tag) && !contains(b.whatwg.Elements[d.tag].Attributes, wa.Name) {
				b.applyWHATWG(attrs, wa.Name, wa, false)
			}
		}
	}

	b.applyOverlay(attrs, "*", false)
	b.applyOverlay(attrs, d.tag, true)
	b.dropObsolete(attrs, d.tag)

	return attrs
}

func (b *builder) dropObsolete(attrs map[string]*attr, scope string) {
	for name, a := range attrs {
		if a.Obsolete {
			id := goIdent(name)
			if b.opt.IncludeObsolete && !contains(elementMethods, id) && !contains(containerMethods, id) {
				a.Deprecated = true
				continue
			}
			b.prov.Skipped[scope+"/"+name] = "obsolete attribute"
			delete(attrs, name)
		}
	}
}

func isEventHandler(t string) bool {
	return t == "EventHandler" || t == "OnErrorEventHandler" || t == "OnBeforeUnloadEventHandler"
}

// addIDL adds the content attributes declared by the given interfaces (and their mixins).
// confirm reports whether another source lists a content attribute, which is required for IDL
// attributes that do not reflect one.
func (b *builder) addIDL(attrs map[string]*attr, chain []string, confirm func(string) bool) {
	for _, iface := range chain {
		for _, ia := range b.src.IDL.Own(iface) {
			if !contains(b.cfg.IDLFiles, ia.File) && !contains(b.cfg.SharedMixins, ia.Owner) {
				continue
			}
			a := b.fromIDL(ia, confirm)
			if a == nil {
				continue
			}
			if b.cfg.PartialIsObsolete && ia.Partial {
				a.Obsolete, a.Deprecated = true, true
			}
			if prev, ok := attrs[a.Name]; ok {
				// e.g. rel (DOMString) and relList (DOMTokenList, [Reflect="rel"]): keep the more specific.
				if kindRank(a) <= kindRank(prev) {
					continue
				}
			}
			attrs[a.Name] = a
		}
	}
}

func kindRank(a *attr) int {
	switch {
	case a.Kind == KindString && !a.URL:
		return 0
	case a.Kind == KindString:
		return 1
	}

	return 2
}

func (b *builder) fromIDL(ia IDLAttribute, confirm func(string) bool) *attr {
	bt := ia.BaseType()
	if isEventHandler(bt) {
		if !contains(b.ov.EventHandlerMixins, ia.Owner) {
			return nil
		}
		return &attr{Name: strings.ToLower(ia.Name), Kind: KindString, Source: srcIDLEventExist, TypeSource: srcIDL}
	}

	if content, ok := ia.ReflectedName(); ok {
		a := &attr{Name: content, Source: srcIDLExist, TypeSource: srcIDL}
		if b.cfg.SVGConventions {
			a.Name = b.svgName(ia.Name, content)
		}
		if _, ok := ia.ExtAttrs["ReflectSetter"]; ok {
			a.TypeSource = srcIDLSetter
		}
		a.Kind, a.URL = idlKind(bt)
		// Only [ReflectURL] and USVString setters (href, action, formAction) hold a single URL;
		// plain [Reflect] USVString members are URL lists (ping, srcset, imagesrcset).
		_, reflURL := ia.ExtAttrs["ReflectURL"]
		_, setter := ia.ExtAttrs["ReflectSetter"]
		a.URL = reflURL || (a.URL && setter) || (b.cfg.SVGConventions && a.URL && a.Name == "href")
		if a.TypeSource == srcIDLSetter && (a.Kind != KindString || a.URL) {
			a.TypeSource = srcIDL // only DOMString setters hide enumerated attributes
		}
		return a
	}

	if b.cfg.SVGConventions && strings.HasPrefix(bt, "SVGAnimated") {
		a := &attr{Name: b.svgName(ia.Name, ""), Source: srcIDLExist, TypeSource: srcIDL}
		switch bt {
		case "SVGAnimatedBoolean":
			a.Kind, a.Allowed = KindEnum, []string{"false", "true"}
		case "SVGAnimatedInteger":
			a.Kind = KindNumber
		case "SVGAnimatedNumber":
			a.Kind = KindFloat
		case "SVGAnimatedEnumeration":
			a.Kind, a.TypeSource = KindString, srcSVGEnum
		case "SVGAnimatedString":
			a.Kind, a.URL = KindString, a.Name == "href"
		default: // Length, LengthList, NumberList, Angle, Rect, TransformList, PreserveAspectRatio
			a.Kind = KindString
		}
		return a
	}

	// Not declared as reflecting. Writable [CEReactions] attributes (and SVG's plain attributes)
	// usually still have a content attribute of the same name, but so do none of innerText, text,
	// ... so another source must list it.
	if ia.ReadOnly && !(b.cfg.SVGConventions && strings.HasPrefix(bt, "SVG")) {
		return nil
	}
	for _, name := range []string{strings.ToLower(ia.Name), ia.Name} {
		if confirm(name) {
			kind, url := idlKind(bt)
			if kind == KindBool {
				kind = KindString // e.g. translate/draggable/spellcheck: boolean IDL, keyword content attribute
			}
			return &attr{Name: name, Kind: kind, URL: url, Source: srcIDLExist, TypeSource: srcIDLUnreflect}
		}
	}

	return nil
}

func (b *builder) svgName(idlName, reflected string) string {
	if r, ok := b.ov.IDLRenames[idlName]; ok {
		return r
	}
	if reflected != "" {
		return reflected // [Reflect] members follow HTML naming (crossorigin, download, ...)
	}

	return idlName
}

func idlKind(t string) (string, bool) {
	if strings.HasPrefix(t, "(") {
		// Unions with Trusted Types: (TrustedScriptURL or USVString) is a URL, (TrustedHTML or
		// DOMString) a string. Other unions say nothing reliable.
		if strings.Contains(t, "TrustedScriptURL") {
			return KindString, true
		}
		return KindString, false
	}
	switch t {
	case "boolean":
		return KindBool, false
	case "long", "unsigned long", "short", "unsigned short", "long long", "unsigned long long":
		return KindNumber, false
	case "double", "float":
		return KindFloat, false
	case "USVString":
		return KindString, true
	case "DOMTokenList":
		return KindSST, false
	}

	return KindString, false
}

// bcdName maps a browser-compat-data key to an attribute name, or "" for sub-features that are not
// attributes (href_sms, data_uri, ...). BCD spells namespaced attributes with an underscore.
func bcdName(key string) string {
	switch {
	case strings.HasPrefix(key, "xlink_"):
		return "xlink:" + strings.TrimPrefix(key, "xlink_")
	case strings.HasPrefix(key, "xml_"):
		return "xml:" + strings.TrimPrefix(key, "xml_")
	case strings.Contains(key, "_"):
		return ""
	}

	return key
}

func (b *builder) addBCD(attrs map[string]*attr, scope, key string, st *BCDStatus) {
	if contains(b.ov.IgnoreBCD, key) {
		return
	}
	name := bcdName(key)
	if name == "" {
		return
	}
	if a, ok := attrs[name]; ok {
		a.Deprecated = a.Deprecated || st.Deprecated
		a.Obsolete = a.Obsolete || st.Obsolete()
		return
	}
	if _, inOverlay := b.ov.For(scope)[name]; !inOverlay {
		switch {
		case !st.StandardTrack:
			b.prov.Skipped[scope+"/"+name] = "browser-compat-data: not standard track"
			return
		case st.Experimental:
			b.prov.Skipped[scope+"/"+name] = "browser-compat-data: experimental and not in the spec's IDL"
			return
		}
	}
	attrs[name] = &attr{Name: name, Kind: KindString, Source: srcBCD, TypeSource: srcDefault,
		Deprecated: st.Deprecated, Obsolete: st.Obsolete()}
}

func (b *builder) applyWHATWG(attrs map[string]*attr, name string, wa WHATWGAttribute, add bool) {
	vc := ClassifyValue(wa.Value)
	a, ok := attrs[name]
	if !ok {
		if !add {
			return
		}
		a = &attr{Name: name, Source: srcWHATWG, TypeSource: srcDefault, Kind: KindString}
		attrs[name] = a
	}
	// A reflected IDL type is at least as precise as "Text"; anything more specific from the
	// index (enumerated keywords, URL, token set) wins.
	if !WeakType(a.TypeSource) && a.TypeSource != srcBCD && vc.Kind == KindString && !vc.URL {
		return
	}
	a.Kind, a.URL, a.Allowed, a.AllowEmpty, a.AllowCustom = vc.Kind, vc.URL, vc.Allowed, vc.AllowEmpty, vc.AllowCustom
	a.TypeSource = srcWHATWG
}

func (b *builder) applyOverlay(attrs map[string]*attr, scope string, add bool) {
	for _, name := range sortedKeys(b.ov.For(scope)) {
		o := b.ov.For(scope)[name]
		a, ok := attrs[name]
		if o.Remove {
			if ok {
				delete(attrs, name)
				b.prov.Skipped[scope+"/"+name] = "overlay: " + o.Reason
			}
			continue
		}
		if !ok {
			if !add {
				continue
			}
			a = &attr{Name: name, Kind: KindString, Source: srcOverlay, TypeSource: srcDefault}
			attrs[name] = a
		}
		if o.Type != "" {
			a.Kind, a.URL = o.Type, false
			if o.Type == KindURL {
				a.Kind, a.URL = KindString, true
			}
			a.Allowed = append([]string(nil), o.Allowed...)
			sort.Strings(a.Allowed)
			a.AllowEmpty, a.AllowCustom = o.AllowEmpty, o.AllowCustom
			a.TypeSource = srcOverlay
			if o.Pending != "" {
				a.TypeSource = srcOverlayPend
			}
		}
		if o.Deprecated != nil {
			a.Deprecated = *o.Deprecated
		}
		if a.Reason == "" || scope != "*" {
			a.Reason = o.Reason
		}
	}
}

func (b *builder) legacyElement(tag string) *spec.Element {
	if l := b.src.Legacy[b.cfg.Name]; l != nil {
		for _, e := range l.Elements {
			if e.Tag == tag {
				return e
			}
		}
	}

	return nil
}

// legacyDescription finds a description for an attribute in the previous document: the element's
// own attribute first, then a global of the same name.
func (b *builder) legacyDescription(tag, name string) string {
	l := b.src.Legacy[b.cfg.Name]
	if l == nil {
		return ""
	}
	var lists [][]spec.Attribute
	if e := b.legacyElement(tag); e != nil {
		lists = append(lists, e.Attributes)
	}
	lists = append(lists, l.Attributes)
	for _, list := range lists {
		for _, a := range list {
			if a.GetName() == name {
				return attrDescription(a)
			}
		}
	}
	if b.whatwg != nil {
		for _, wa := range b.whatwg.Attributes {
			if wa.Name == name && (contains(wa.Elements, tag) || wa.Global) {
				return wa.Description
			}
		}
	}

	return ""
}

func attrDescription(a spec.Attribute) string {
	switch a := a.(type) {
	case *spec.AttributeTypeString:
		return a.Description
	case *spec.AttributeTypeChar:
		return a.Description
	case *spec.AttributeTypeNumber:
		return a.Description
	case *spec.AttributeTypeFloat:
		return a.Description
	case *spec.AttributeTypeBool:
		return a.Description
	case *spec.AttributeTypeEnum:
		return a.Description
	case *spec.AttributeTypeSST:
		return a.Description
	case *spec.AttributeTypePrefixedCustom:
		return a.Description
	}

	return ""
}

func (b *builder) toSpec(scope string, a *attr, tag string) spec.Attribute {
	b.prov.Attributes[scope+"/"+a.Name] = AttrProvenance{Kind: kindLabel(a), Source: a.Source, TypeSource: a.TypeSource, Reason: a.Reason}
	desc := b.legacyDescription(tag, a.Name)

	return a.toSpec(desc)
}

func kindLabel(a *attr) string {
	if a.Kind == KindString && a.URL {
		return KindURL
	}

	return a.Kind
}

func (a *attr) toSpec(desc string) spec.Attribute {
	switch a.Kind {
	case KindChar:
		return &spec.AttributeTypeChar{Name: a.Name, Description: desc, Deprecated: a.Deprecated}
	case KindNumber:
		return &spec.AttributeTypeNumber{Name: a.Name, Description: desc, Deprecated: a.Deprecated}
	case KindFloat:
		return &spec.AttributeTypeFloat{Name: a.Name, Description: desc, Deprecated: a.Deprecated}
	case KindBool:
		return &spec.AttributeTypeBool{Name: a.Name, Description: desc, Deprecated: a.Deprecated}
	case KindSST:
		return &spec.AttributeTypeSST{Name: a.Name, Description: desc, Deprecated: a.Deprecated}
	case KindPrefixedCustom:
		return &spec.AttributeTypePrefixedCustom{Name: a.Name, Description: desc, Deprecated: a.Deprecated}
	case KindEnum:
		allowed := map[string]struct{}{}
		for _, v := range a.Allowed {
			allowed[v] = struct{}{}
		}
		return &spec.AttributeTypeEnum{Name: a.Name, Description: desc, Allowed: allowed,
			AllowEmpty: a.AllowEmpty, AllowCustom: a.AllowCustom, Deprecated: a.Deprecated}
	}

	return &spec.AttributeTypeString{Name: a.Name, Description: desc, URL: a.URL, Deprecated: a.Deprecated}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)

	return out
}
