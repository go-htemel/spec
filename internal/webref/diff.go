package webref

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-htemel/spec"
)

// Classes of differences, used to separate what the new pipeline gets right from what it cannot
// know yet.
const (
	ClassEvidence = "evidence"      // new value backed by reflected IDL / SVG IDL / the WHATWG index
	ClassOverlay  = "overlay"       // new value is a recorded judgment call
	ClassPending  = "pending"       // overlay stand-in for data the WHATWG index will supply
	ClassWeak     = "weak"          // new type is a guess (String) — a gap the index (html) or overlay (svg) must fill
	ClassSkipped  = "skipped"       // dropped on purpose (obsolete, non-standard, overlay removal)
	ClassNoSource = "not-in-source" // old attribute that no source lists for the element
)

var classText = map[string]string{
	ClassEvidence: "new value backed by reflected IDL / SVG IDL types (pipeline evidence)",
	ClassOverlay:  "new value from an overlay judgment call",
	ClassPending:  "overlay stand-in for data a not-yet-vendored source would supply (WHATWG index for HTML, @webref/css for SVG presentation attributes)",
	ClassWeak:     "new type is a String fallback: no source states the value syntax (WHATWG index would fill for HTML)",
	ClassSkipped:  "dropped on purpose (obsolete, not standard-track, or overlay removal)",
	ClassNoSource: "no source lists it for this element",
}

// AttrDelta is one attribute-level difference.
type AttrDelta struct {
	Name   string
	Old    string // type label in the old document ("" when absent)
	New    string // type label in the new document ("" when absent)
	Class  string
	Detail string // provenance / reason
}

// ElementDiff lists the differences for one element (or the globals).
type ElementDiff struct {
	Tag     string
	OnlyOld []AttrDelta
	OnlyNew []AttrDelta
	Types   []AttrDelta
	Flags   []string
}

func (e ElementDiff) empty() bool {
	return len(e.OnlyOld) == 0 && len(e.OnlyNew) == 0 && len(e.Types) == 0 && len(e.Flags) == 0
}

// Diff is the comparison of two documents.
type Diff struct {
	OldElements, NewElements []string
	OnlyOldElements          []string
	OnlyNewElements          []string
	OnlyOldReasons           map[string]string
	Globals                  ElementDiff
	Elements                 []ElementDiff
	Compared                 int // attribute pairs present on both sides (element scope, effective)
	Agree                    int
	URLFlagged               int // URL-flagged string attributes in the new document
	Deprecated               int // deprecated attributes in the new document
	WHATWGUsed               bool
	OldGlobals, NewGlobals   int
	// TypeSources counts every attribute of the new document (globals and element-specific) by
	// how its type was determined: evidence, overlay, pending or weak.
	TypeSources map[string]int
}

// NamedDiff labels a Diff for the combined report.
type NamedDiff struct {
	Name, Old, New string
	Diff           *Diff
}

// Compare diffs old against new. Per element it compares the attributes each document would
// generate for that element (own attributes, falling back to a global of the same name), restricted
// to names that are element-specific on at least one side; global attribute lists are compared
// separately, so a global difference is reported once rather than once per element.
func Compare(oldSp, newSp *spec.Spec, prov *Provenance) *Diff {
	d := &Diff{OnlyOldReasons: map[string]string{}}
	if prov != nil {
		d.WHATWGUsed = prov.WHATWG
	}
	oldEl, newEl := elementMap(oldSp), elementMap(newSp)
	d.OldElements, d.NewElements = sortedKeys(oldEl), sortedKeys(newEl)
	for _, t := range d.OldElements {
		if newEl[t] == nil {
			d.OnlyOldElements = append(d.OnlyOldElements, t)
			if prov != nil {
				if r, ok := prov.Skipped["<"+t+">"]; ok {
					d.OnlyOldReasons[t] = r
				}
			}
		}
	}
	for _, t := range d.NewElements {
		if oldEl[t] == nil {
			d.OnlyNewElements = append(d.OnlyNewElements, t)
		}
	}

	oldG, newG := attrMap(oldSp.Attributes), attrMap(newSp.Attributes)
	d.OldGlobals, d.NewGlobals = len(oldG), len(newG)
	d.TypeSources = map[string]int{}
	if prov != nil {
		for _, p := range prov.Attributes {
			d.TypeSources[classify(p)]++
		}
	}
	d.Globals = compareSets("@global", oldG, newG, nil, nil, prov).ElementDiff

	for _, t := range d.OldElements {
		if newEl[t] == nil {
			continue
		}
		oe, ne := oldEl[t], newEl[t]
		ed := compareSets(t, attrMap(oe.Attributes), attrMap(ne.Attributes), oldG, newG, prov)
		flagSrc := ""
		if prov != nil {
			flagSrc = " (source: " + prov.Elements[t].FlagsSrc + ")"
		}
		if oe.Void != ne.Void {
			ed.Flags = append(ed.Flags, fmt.Sprintf("void %v -> %v%s", oe.Void, ne.Void, flagSrc))
		}
		if oe.Text != ne.Text {
			ed.Flags = append(ed.Flags, fmt.Sprintf("text %v -> %v%s", oe.Text, ne.Text, flagSrc))
		}
		if !ed.empty() {
			d.Elements = append(d.Elements, ed.ElementDiff)
		}
		d.Compared += ed.compared
		d.Agree += ed.agree
	}

	count := func(a spec.Attribute) {
		if s, ok := a.(*spec.AttributeTypeString); ok && s.URL {
			d.URLFlagged++
		}
		if a.IsDeprecated() {
			d.Deprecated++
		}
	}
	for _, a := range newSp.Attributes {
		count(a)
	}
	for _, e := range newSp.Elements {
		for _, a := range e.Attributes {
			count(a)
		}
	}

	return d
}

type elementDiffCount struct {
	ElementDiff
	compared, agree int
}

func compareSets(scope string, oldOwn, newOwn, oldG, newG map[string]spec.Attribute, prov *Provenance) elementDiffCount {
	out := elementDiffCount{ElementDiff: ElementDiff{Tag: scope}}
	names := map[string]bool{}
	for n := range oldOwn {
		names[n] = true
	}
	for n := range newOwn {
		names[n] = true
	}
	for _, n := range sortedKeys(names) {
		o, ok := oldOwn[n]
		if !ok {
			o = oldG[n]
		}
		nw, ok := newOwn[n]
		newScope := scope
		if !ok {
			nw = newG[n]
			newScope = "@global"
		}
		var p AttrProvenance
		var skipped string
		if prov != nil {
			p = prov.Attributes[newScope+"/"+n]
			skipped = prov.Skipped[scope+"/"+n]
		}
		switch {
		case o == nil && nw == nil:
		case o == nil:
			out.OnlyNew = append(out.OnlyNew, AttrDelta{Name: n, New: typeLabel(nw), Class: classify(p), Detail: detail(p)})
		case nw == nil:
			c, det := ClassNoSource, ""
			if skipped != "" {
				c, det = ClassSkipped, skipped
			}
			out.OnlyOld = append(out.OnlyOld, AttrDelta{Name: n, Old: typeLabel(o), Class: c, Detail: det})
		default:
			out.compared++
			if typeLabel(o) == typeLabel(nw) {
				out.agree++
				continue
			}
			out.Types = append(out.Types, AttrDelta{Name: n, Old: typeLabel(o), New: typeLabel(nw), Class: classify(p), Detail: detail(p)})
		}
	}

	return out
}

func classify(p AttrProvenance) string {
	switch {
	case p.TypeSource == srcOverlayPend:
		return ClassPending
	case p.TypeSource == srcOverlay:
		return ClassOverlay
	case WeakType(p.TypeSource) || p.TypeSource == "":
		return ClassWeak
	}

	return ClassEvidence
}

func detail(p AttrProvenance) string {
	if p.Source == "" {
		return ""
	}
	s := "exists: " + p.Source + "; type: " + p.TypeSource
	if p.Reason != "" {
		s += "; " + p.Reason
	}

	return s
}

// typeLabel renders an attribute's type for comparison. The URL flag is not part of the label: the
// old documents cannot express it, so it is counted separately rather than as a disagreement.
func typeLabel(a spec.Attribute) string {
	switch a := a.(type) {
	case *spec.AttributeTypeString:
		return "String"
	case *spec.AttributeTypeChar:
		return "Char"
	case *spec.AttributeTypeNumber:
		return "Number"
	case *spec.AttributeTypeFloat:
		return "Float"
	case *spec.AttributeTypeBool:
		return "Bool"
	case *spec.AttributeTypeSST:
		return "SST"
	case *spec.AttributeTypePrefixedCustom:
		return "PrefixedCustom"
	case *spec.AttributeTypeEnum:
		s := "Enum{" + strings.Join(a.AllowedSorted(), ",") + "}"
		if a.AllowEmpty {
			s += "+empty"
		}
		if a.AllowCustom {
			s += "+custom"
		}
		return s
	}

	return fmt.Sprintf("%T", a)
}

func elementMap(sp *spec.Spec) map[string]*spec.Element {
	m := map[string]*spec.Element{}
	for _, e := range sp.Elements {
		m[e.Tag] = e
	}

	return m
}

func attrMap(list []spec.Attribute) map[string]spec.Attribute {
	m := map[string]spec.Attribute{}
	for _, a := range list {
		m[a.GetName()] = a
	}

	return m
}

// Counts summarises a Diff by class.
type Counts struct {
	OnlyOld, OnlyNew, Types map[string]int
	OnlyOldN, OnlyNewN      int
	TypesN                  int
}

// Count tallies the element-scope and global differences by class.
func (d *Diff) Count() Counts {
	c := Counts{OnlyOld: map[string]int{}, OnlyNew: map[string]int{}, Types: map[string]int{}}
	for _, e := range append([]ElementDiff{d.Globals}, d.Elements...) {
		for _, x := range e.OnlyOld {
			c.OnlyOld[x.Class]++
			c.OnlyOldN++
		}
		for _, x := range e.OnlyNew {
			c.OnlyNew[x.Class]++
			c.OnlyNewN++
		}
		for _, x := range e.Types {
			c.Types[x.Class]++
			c.TypesN++
		}
	}

	return c
}

// Markdown renders the diff as a standalone report section.
func (d *Diff) Markdown(oldPath, newPath string) string {
	var b strings.Builder
	c := d.Count()
	fmt.Fprintf(&b, "Comparing `%s` (old) with `%s` (new). WHATWG index used: %v.\n\n", oldPath, newPath, d.WHATWGUsed)
	fmt.Fprintf(&b, "| | count |\n|---|---|\n")
	fmt.Fprintf(&b, "| elements old / new | %d / %d |\n", len(d.OldElements), len(d.NewElements))
	fmt.Fprintf(&b, "| elements only in old | %d |\n", len(d.OnlyOldElements))
	fmt.Fprintf(&b, "| elements only in new | %d |\n", len(d.OnlyNewElements))
	fmt.Fprintf(&b, "| global attributes old / new | %d / %d |\n", d.OldGlobals, d.NewGlobals)
	fmt.Fprintf(&b, "| element attributes on both sides (incl. globals) | %d (agree on type: %d) |\n", d.Compared, d.Agree)
	fmt.Fprintf(&b, "| attributes only in old (elements + globals) | %d |\n", c.OnlyOldN)
	fmt.Fprintf(&b, "| attributes only in new (elements + globals) | %d |\n", c.OnlyNewN)
	fmt.Fprintf(&b, "| type disagreements (elements + globals) | %d |\n", c.TypesN)
	fmt.Fprintf(&b, "| URL-flagged attributes in new | %d |\n", d.URLFlagged)
	fmt.Fprintf(&b, "| deprecated-flagged attributes in new | %d |\n", d.Deprecated)
	fmt.Fprintf(&b, "| all new attributes by type source: evidence / overlay / pending / weak | %d / %d / %d / %d |\n\n",
		d.TypeSources[ClassEvidence], d.TypeSources[ClassOverlay], d.TypeSources[ClassPending], d.TypeSources[ClassWeak])

	classes := []string{ClassEvidence, ClassOverlay, ClassPending, ClassWeak, ClassSkipped, ClassNoSource}
	fmt.Fprintf(&b, "By class:\n\n| class | meaning | only in new | type disagreements | only in old |\n|---|---|---|---|---|\n")
	for _, cl := range classes {
		if c.OnlyNew[cl]+c.Types[cl]+c.OnlyOld[cl] == 0 {
			continue
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d |\n", cl, classText[cl], c.OnlyNew[cl], c.Types[cl], c.OnlyOld[cl])
	}
	b.WriteString("\n")

	if len(d.OnlyOldElements) > 0 {
		b.WriteString("Elements only in old: ")
		var parts []string
		for _, t := range d.OnlyOldElements {
			if r := d.OnlyOldReasons[t]; r != "" {
				t += " (" + r + ")"
			}
			parts = append(parts, "`"+t+"`")
		}
		b.WriteString(strings.Join(parts, ", ") + "\n\n")
	}
	if len(d.OnlyNewElements) > 0 {
		b.WriteString("Elements only in new: " + codeList(d.OnlyNewElements) + "\n\n")
	}

	writeSection := func(e ElementDiff) {
		title := "`<" + e.Tag + ">`"
		if e.Tag == "@global" {
			title = "Global attributes"
		}
		fmt.Fprintf(&b, "#### %s\n\n", title)
		for _, f := range e.Flags {
			fmt.Fprintf(&b, "- flags: %s\n", f)
		}
		if len(e.Types) > 0 {
			b.WriteString("\n| type disagreement | old | new | class | provenance |\n|---|---|---|---|---|\n")
			for _, x := range e.Types {
				fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s |\n", x.Name, x.Old, x.New, x.Class, x.Detail)
			}
			b.WriteString("\n")
		}
		if len(e.OnlyNew) > 0 {
			b.WriteString("- only in new: " + deltaList(e.OnlyNew, false) + "\n")
		}
		if len(e.OnlyOld) > 0 {
			b.WriteString("- only in old: " + deltaList(e.OnlyOld, true) + "\n")
		}
		b.WriteString("\n")
	}
	if !d.Globals.empty() {
		writeSection(d.Globals)
	}
	for _, e := range d.Elements {
		writeSection(e)
	}

	return b.String()
}

func codeList(xs []string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = "`" + x + "`"
	}

	return strings.Join(parts, ", ")
}

func deltaList(xs []AttrDelta, old bool) string {
	byClass := map[string][]string{}
	for _, x := range xs {
		label := x.New
		if old {
			label = x.Old
		}
		s := "`" + x.Name + "` " + label
		if old && x.Detail != "" {
			s += " (" + x.Detail + ")"
		}
		byClass[x.Class] = append(byClass[x.Class], s)
	}
	var parts []string
	for _, cl := range sortedKeys(byClass) {
		parts = append(parts, "["+cl+"] "+strings.Join(byClass[cl], ", "))
	}

	return strings.Join(parts, "; ")
}

const (
	notesBegin = "<!-- notes:begin -->"
	notesEnd   = "<!-- notes:end -->"
)

// Report renders the combined coverage report. Hand-written notes between the notes markers of an
// existing report file are carried over by ReportPreservingNotes.
func Report(m *SourcesManifest, diffs []NamedDiff) string {
	return renderReport(m, diffs, "")
}

// ReportPreservingNotes is Report, keeping the hand-written notes block of the file at path.
func ReportPreservingNotes(path string, m *SourcesManifest, diffs []NamedDiff) string {
	notes := ""
	if b, err := os.ReadFile(path); err == nil {
		s := string(b)
		if i, j := strings.Index(s, notesBegin), strings.Index(s, notesEnd); i >= 0 && j > i {
			notes = s[i+len(notesBegin) : j]
		}
	}

	return renderReport(m, diffs, notes)
}

func renderReport(m *SourcesManifest, diffs []NamedDiff, notes string) string {
	var b strings.Builder
	b.WriteString("# webref pipeline coverage report\n\n")
	b.WriteString("Generated by `go run ./cmd/specgen2 report` from `specs/candidates/*.json` (built by `go run ./cmd/specgen2 generate`)\n")
	b.WriteString("and the committed `specs/*.json`. Everything outside the notes block is regenerated.\n\n")
	b.WriteString("## Inputs\n\n| source | version | integrity | retrieved | status |\n|---|---|---|---|---|\n")
	for _, s := range m.Sources {
		integ := s.Integrity
		if len(integ) > 24 {
			integ = integ[:24] + "…"
		}
		status := s.Status
		if status == "" {
			status = "vendored"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", s.Name, s.Version, integ, s.Retrieved, status)
	}
	b.WriteString("\n")
	b.WriteString("## Headline numbers\n\n| spec | elements old/new | only old | only new | attrs only old | attrs only new | type disagreements | of which evidence / overlay / pending / weak | all new attrs: evidence / overlay / pending / weak |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, nd := range diffs {
		d, c := nd.Diff, nd.Diff.Count()
		fmt.Fprintf(&b, "| %s | %d/%d | %d | %d | %d | %d | %d | %d / %d / %d / %d | %d / %d / %d / %d |\n", nd.Name,
			len(d.OldElements), len(d.NewElements), len(d.OnlyOldElements), len(d.OnlyNewElements),
			c.OnlyOldN, c.OnlyNewN, c.TypesN,
			c.Types[ClassEvidence], c.Types[ClassOverlay], c.Types[ClassPending], c.Types[ClassWeak],
			d.TypeSources[ClassEvidence], d.TypeSources[ClassOverlay], d.TypeSources[ClassPending], d.TypeSources[ClassWeak])
	}
	b.WriteString("\n")
	b.WriteString(notesBegin)
	if notes == "" {
		notes = "\n_No notes yet._\n"
	}
	b.WriteString(notes)
	b.WriteString(notesEnd + "\n\n")
	b.WriteString("## How to read the details\n\n")
	b.WriteString("Per element, the report compares the attributes each document would make htemel generate for that element\n")
	b.WriteString("(its own attributes, else a global of the same name), for names that are element-specific on at least one side.\n")
	b.WriteString("Global attribute lists are compared once. Type labels ignore the new `url` flag (the old documents cannot express it).\n")
	b.WriteString("Each difference carries a class:\n\n")
	for _, cl := range []string{ClassEvidence, ClassOverlay, ClassPending, ClassWeak, ClassSkipped, ClassNoSource} {
		fmt.Fprintf(&b, "- **%s**: %s\n", cl, classText[cl])
	}
	b.WriteString("\n")
	for _, nd := range diffs {
		fmt.Fprintf(&b, "## %s\n\n", nd.Name)
		b.WriteString(nd.Diff.Markdown(nd.Old, nd.New))
	}

	return b.String()
}
