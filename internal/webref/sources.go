package webref

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-htemel/spec"
)

// Layout of the committed data directory (the -data flag, "data" by default):
//
//	SOURCES.json                 provenance of every vendored input (package, version, integrity, date)
//	webref/elements/<spec>.json  copied verbatim from @webref/elements
//	webref/idl/<spec>.idl        copied verbatim from @webref/idl
//	bcd/<ns>.json                html/svg/mathml subtrees of @mdn/browser-compat-data, trimmed to status + spec_url
//	whatwg/indices.html          pinned WHATWG HTML commit snapshot of multipage/indices.html (optional)
//	overlay/<spec>.json          hand-maintained judgment calls
const (
	ElementsDir  = "webref/elements"
	IDLDir       = "webref/idl"
	BCDDir       = "bcd"
	WHATWGFile   = "whatwg/indices.html"
	OverlayDir   = "overlay"
	SourcesFile  = "SOURCES.json"
	CandidateDir = "specs/candidates"
)

// WebrefElements is the shape of an @webref/elements file.
type WebrefElements struct {
	Spec struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	} `json:"spec"`
	Elements []struct {
		Name      string `json:"name"`
		Href      string `json:"href"`
		Interface string `json:"interface"`
		Obsolete  bool   `json:"obsolete"`
	} `json:"elements"`
}

// BCDStatus is the subset of a browser-compat-data __compat block the generator uses.
type BCDStatus struct {
	Deprecated    bool     `json:"deprecated,omitempty"`
	Experimental  bool     `json:"experimental,omitempty"`
	StandardTrack bool     `json:"standard_track,omitempty"`
	SpecURL       []string `json:"spec_url,omitempty"`
}

// Obsolete reports whether the feature is specified in an obsolete-features section.
func (s *BCDStatus) Obsolete() bool {
	for _, u := range s.SpecURL {
		if strings.Contains(u, "/obsolete.html") {
			return true
		}
	}

	return false
}

// BCDFeature is an element (or the global-attributes pseudo-element) and its direct sub-features.
type BCDFeature struct {
	BCDStatus
	Attributes map[string]*BCDStatus `json:"attributes,omitempty"`
}

// BCDNamespace is the trimmed form of data.json's html, svg or mathml subtree.
type BCDNamespace struct {
	Elements         map[string]*BCDFeature `json:"elements"`
	GlobalAttributes map[string]*BCDStatus  `json:"global_attributes"`
}

// Sources holds every input the builder reads.
type Sources struct {
	Elements map[string]*WebrefElements // keyed by file base name without extension, e.g. "html", "SVG2"
	IDL      *IDL
	BCD      map[string]*BCDNamespace // keyed by namespace: html, svg, mathml
	Overlays map[string]*Overlay      // keyed by spec config name
	WHATWG   *WHATWGIndex             // nil when the snapshot has not been vendored
	Legacy   map[string]*spec.Spec    // previous spec documents, used only for descriptions
}

// LoadSources reads the committed data directory. A missing WHATWG snapshot is not an error.
func LoadSources(dataDir string) (*Sources, error) {
	src := &Sources{
		Elements: map[string]*WebrefElements{},
		IDL:      NewIDL(),
		BCD:      map[string]*BCDNamespace{},
		Overlays: map[string]*Overlay{},
		Legacy:   map[string]*spec.Spec{},
	}

	if err := eachFile(filepath.Join(dataDir, ElementsDir), ".json", func(name string, b []byte) error {
		var e WebrefElements
		if err := json.Unmarshal(b, &e); err != nil {
			return err
		}
		src.Elements[name] = &e
		return nil
	}); err != nil {
		return nil, err
	}

	if err := eachFile(filepath.Join(dataDir, IDLDir), ".idl", func(name string, b []byte) error {
		return src.IDL.Parse(name+".idl", string(b))
	}); err != nil {
		return nil, err
	}

	if err := eachFile(filepath.Join(dataDir, BCDDir), ".json", func(name string, b []byte) error {
		var ns BCDNamespace
		if err := json.Unmarshal(b, &ns); err != nil {
			return err
		}
		src.BCD[name] = &ns
		return nil
	}); err != nil {
		return nil, err
	}

	if err := eachFile(filepath.Join(dataDir, OverlayDir), ".json", func(name string, b []byte) error {
		o, err := ParseOverlay(b)
		if err != nil {
			return err
		}
		src.Overlays[name] = o
		return nil
	}); err != nil {
		return nil, err
	}

	f, err := os.Open(filepath.Join(dataDir, WHATWGFile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// Not vendored yet; the builder falls back to IDL + overlay.
	case err != nil:
		return nil, err
	default:
		defer f.Close()
		idx, err := ParseIndices(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", WHATWGFile, err)
		}
		src.WHATWG = idx
	}

	return src, nil
}

// LoadLegacy reads a previous spec document to reuse its descriptions. A missing file is ignored.
func (s *Sources) LoadLegacy(name, path string) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	sp := &spec.Spec{}
	if err := json.Unmarshal(b, sp); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	s.Legacy[name] = sp

	return nil
}

func eachFile(dir, ext string, fn func(name string, b []byte) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ext {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return err
		}
		if err := fn(strings.TrimSuffix(n, ext), b); err != nil {
			return fmt.Errorf("%s: %w", filepath.Join(dir, n), err)
		}
	}

	return nil
}
