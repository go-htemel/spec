package webref

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Source records where one vendored input came from.
type Source struct {
	Name      string   `json:"name"`
	Version   string   `json:"version,omitempty"`
	URL       string   `json:"url,omitempty"`
	Integrity string   `json:"integrity,omitempty"` // npm-style sha512-<base64> of the tarball, or sha256-<hex> of a fetched file
	Retrieved string   `json:"retrieved,omitempty"` // date the input was vendored (YYYY-MM-DD)
	Published string   `json:"published,omitempty"` // upstream timestamp, when the package records one
	License   string   `json:"license,omitempty"`
	Files     []string `json:"files,omitempty"` // files under the data directory produced from this source
	Status    string   `json:"status,omitempty"`
	Note      string   `json:"note,omitempty"`
}

// SourcesManifest is the content of data/SOURCES.json.
type SourcesManifest struct {
	Comment string   `json:"comment"`
	Sources []Source `json:"sources"`
}

// ReadManifest loads data/SOURCES.json, returning an empty manifest when it does not exist.
func ReadManifest(dataDir string) (*SourcesManifest, error) {
	b, err := os.ReadFile(filepath.Join(dataDir, SourcesFile))
	if os.IsNotExist(err) {
		return &SourcesManifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := &SourcesManifest{}

	return m, json.Unmarshal(b, m)
}

// Set replaces (or adds) the entry with s.Name and keeps entries sorted by name.
func (m *SourcesManifest) Set(s Source) {
	for i := range m.Sources {
		if m.Sources[i].Name == s.Name {
			m.Sources[i] = s
			return
		}
	}
	m.Sources = append(m.Sources, s)
	sort.Slice(m.Sources, func(i, j int) bool { return m.Sources[i].Name < m.Sources[j].Name })
}

// Write stores the manifest as data/SOURCES.json.
func (m *SourcesManifest) Write(dataDir string) error {
	m.Comment = "Provenance of the vendored inputs under this directory. Regenerate with `go run ./cmd/specgen2 vendor` and `go run ./cmd/specgen2 fetch`; see cmd/specgen2."
	return writeJSON(filepath.Join(dataDir, SourcesFile), m)
}

// VendorConfig selects which files of each npm package are vendored.
type VendorConfig struct {
	ElementFiles []string // @webref/elements files, e.g. "html.json"
	IDLFiles     []string // @webref/idl files, e.g. "html.idl"
	BCDTrees     []string // top-level data.json keys, e.g. "html"
}

// DefaultVendorConfig lists the inputs needed for the html, svg and mathml specs.
var DefaultVendorConfig = VendorConfig{
	ElementFiles: []string{"html.json", "SVG2.json", "filter-effects-1.json", "css-masking-1.json", "svg-animations.json", "svg-paths.json", "mathml-core.json"},
	IDLFiles:     []string{"html.idl", "SVG.idl", "filter-effects.idl", "css-masking.idl", "svg-animations.idl", "svg-paths.idl", "mathml-core.idl"},
	BCDTrees:     []string{"html", "svg", "mathml"},
}

// VendorTarballs copies the configured files out of local npm tarballs (`npm pack <pkg>@<version>`)
// into dataDir and records them in SOURCES.json. The tarballs are only read as archives; nothing in
// them is executed. An empty path skips that package.
func VendorTarballs(dataDir string, cfg VendorConfig, elementsTgz, idlTgz, bcdTgz string, today time.Time) error {
	m, err := ReadManifest(dataDir)
	if err != nil {
		return err
	}
	date := today.UTC().Format("2006-01-02")

	type job struct {
		tgz     string
		license string
		extract func(files map[string][]byte) ([]string, error)
	}
	jobs := []job{
		{elementsTgz, "MIT (webref tooling); element data extracted from the specifications", func(files map[string][]byte) ([]string, error) {
			return copyFiles(dataDir, ElementsDir, cfg.ElementFiles, files)
		}},
		{idlTgz, "MIT (webref tooling); IDL extracted from the specifications under their licenses", func(files map[string][]byte) ([]string, error) {
			return copyFiles(dataDir, IDLDir, cfg.IDLFiles, files)
		}},
		{bcdTgz, "CC0-1.0", func(files map[string][]byte) ([]string, error) {
			return trimBCD(dataDir, cfg.BCDTrees, files["data.json"])
		}},
	}
	for _, j := range jobs {
		if j.tgz == "" {
			continue
		}
		raw, err := os.ReadFile(j.tgz)
		if err != nil {
			return err
		}
		files, err := readTarball(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", j.tgz, err)
		}
		var pkg struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal(files["package.json"], &pkg); err != nil {
			return fmt.Errorf("%s: package.json: %w", j.tgz, err)
		}
		written, err := j.extract(files)
		if err != nil {
			return fmt.Errorf("%s: %w", j.tgz, err)
		}
		sum := sha512.Sum512(raw)
		src := Source{
			Name:      pkg.Name,
			Version:   pkg.Version,
			URL:       npmTarballURL(pkg.Name, pkg.Version),
			Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum[:]),
			Retrieved: date,
			License:   j.license,
			Files:     written,
		}
		if pkg.Name == "@mdn/browser-compat-data" {
			var meta struct {
				Meta struct {
					Timestamp string `json:"timestamp"`
				} `json:"__meta"`
			}
			_ = json.Unmarshal(files["data.json"], &meta)
			src.Published = meta.Meta.Timestamp
		}
		m.Set(src)
	}

	if _, err := os.Stat(filepath.Join(dataDir, WHATWGFile)); os.IsNotExist(err) {
		m.Set(Source{
			Name:   "whatwg-html-indices",
			URL:    WHATWGSnapshotURL("<commit-sha>"),
			Status: "pending",
			Note:   "Not vendored yet: run `go run ./cmd/specgen2 fetch -sha <commit>` from a machine that can reach html.spec.whatwg.org. Until then element categories, void/text flags and enumerated values come from overlay/html.json.",
		})
	}

	return m.Write(dataDir)
}

func npmTarballURL(name, version string) string {
	base := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		base = name[i+1:]
	}

	return "https://registry.npmjs.org/" + name + "/-/" + base + "-" + version + ".tgz"
}

// readTarball returns the regular files of an npm tarball keyed by path relative to "package/".
func readTarball(raw []byte) (map[string][]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(zr)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		name := path.Clean(h.Name)
		if i := strings.IndexByte(name, '/'); i >= 0 {
			name = name[i+1:] // strip "package/"
		}
		b, err := io.ReadAll(io.LimitReader(tr, 256<<20))
		if err != nil {
			return nil, err
		}
		out[name] = b
	}
}

func copyFiles(dataDir, sub string, want []string, files map[string][]byte) ([]string, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, sub), 0o755); err != nil {
		return nil, err
	}
	var written []string
	for _, f := range want {
		b, ok := files[f]
		if !ok {
			return nil, fmt.Errorf("package has no %s", f)
		}
		rel := path.Join(sub, f)
		if err := os.WriteFile(filepath.Join(dataDir, rel), b, 0o644); err != nil {
			return nil, err
		}
		written = append(written, rel)
	}

	return written, nil
}

// trimBCD keeps, for each tree, every element and global attribute with its direct sub-features,
// reduced to the status flags and spec URLs.
func trimBCD(dataDir string, trees []string, data []byte) ([]string, error) {
	if data == nil {
		return nil, fmt.Errorf("package has no data.json")
	}
	type compat struct {
		Status struct {
			Deprecated    bool `json:"deprecated"`
			Experimental  bool `json:"experimental"`
			StandardTrack bool `json:"standard_track"`
		} `json:"status"`
		SpecURL json.RawMessage `json:"spec_url"`
	}
	status := func(raw json.RawMessage) (*BCDStatus, error) {
		var node struct {
			Compat *compat `json:"__compat"`
		}
		if err := json.Unmarshal(raw, &node); err != nil {
			return nil, err
		}
		if node.Compat == nil {
			return nil, nil
		}
		s := &BCDStatus{
			Deprecated:    node.Compat.Status.Deprecated,
			Experimental:  node.Compat.Status.Experimental,
			StandardTrack: node.Compat.Status.StandardTrack,
		}
		if len(node.Compat.SpecURL) > 0 {
			var one string
			if json.Unmarshal(node.Compat.SpecURL, &one) == nil {
				s.SpecURL = []string{one}
			} else if err := json.Unmarshal(node.Compat.SpecURL, &s.SpecURL); err != nil {
				return nil, err
			}
		}
		return s, nil
	}
	children := func(raw json.RawMessage) (map[string]*BCDStatus, error) {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		out := map[string]*BCDStatus{}
		for k, v := range m {
			if k == "__compat" {
				continue
			}
			s, err := status(v)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			if s != nil {
				out[k] = s
			}
		}
		return out, nil
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, BCDDir), 0o755); err != nil {
		return nil, err
	}
	var written []string
	for _, tree := range trees {
		var t struct {
			Elements         map[string]json.RawMessage `json:"elements"`
			GlobalAttributes json.RawMessage            `json:"global_attributes"`
		}
		if err := json.Unmarshal(root[tree], &t); err != nil {
			return nil, fmt.Errorf("%s: %w", tree, err)
		}
		ns := &BCDNamespace{Elements: map[string]*BCDFeature{}}
		for el, raw := range t.Elements {
			s, err := status(raw)
			if err != nil || s == nil {
				return nil, fmt.Errorf("%s.elements.%s: missing __compat (%v)", tree, el, err)
			}
			attrs, err := children(raw)
			if err != nil {
				return nil, fmt.Errorf("%s.elements.%s: %w", tree, el, err)
			}
			ns.Elements[el] = &BCDFeature{BCDStatus: *s, Attributes: attrs}
		}
		if t.GlobalAttributes != nil {
			g, err := children(t.GlobalAttributes)
			if err != nil {
				return nil, fmt.Errorf("%s.global_attributes: %w", tree, err)
			}
			ns.GlobalAttributes = g
		}
		rel := path.Join(BCDDir, tree+".json")
		if err := writeJSON(filepath.Join(dataDir, rel), ns); err != nil {
			return nil, err
		}
		written = append(written, rel)
	}

	return written, nil
}

var shaRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

// WHATWGLiveURL is the single-page HTML standard. Commit snapshots exist only in this single-page
// form (there is no /multipage/ under commit-snapshots/).
const WHATWGLiveURL = "https://html.spec.whatwg.org/"

// WHATWGSnapshotURL is the commit-snapshot URL of the single-page HTML standard for sha.
func WHATWGSnapshotURL(sha string) string {
	return "https://html.spec.whatwg.org/commit-snapshots/" + sha + "/"
}

// commitLinkRE finds the commit the live page was built from. The page's header links to its own
// snapshot as /commit-snapshots/<sha>/ (id="commit-snapshot-link"); older builds linked to
// github.com/whatwg/html/commit/<sha> instead, so both forms are accepted.
var commitLinkRE = regexp.MustCompile(`(?:/commit-snapshots/|github\.com/whatwg/html/commit/)([0-9a-f]{40})`)

// pinnedCommit returns the whatwg/html commit hash the HTML standard page reports, or "".
func pinnedCommit(page []byte) string {
	m := commitLinkRE.FindSubmatch(page)
	if m == nil {
		return ""
	}

	return string(m[1])
}

// FetchWHATWG downloads the HTML standard, extracts its element and attribute index tables into
// data/whatwg/indices.html, and records the pinned commit in SOURCES.json.
//
// With a full commit hash it fetches that commit snapshot. With an empty sha it fetches the live
// page and pins the commit the page itself reports (the most recently deployed one), which avoids
// asking for a snapshot that has not been built yet. Only the two index tables are vendored, not
// the ~14 MB page, so the module stays small; the integrity recorded is that of the full page.
func FetchWHATWG(client *http.Client, dataDir, sha string, today time.Time) error {
	url := WHATWGLiveURL
	if sha != "" {
		if !shaRE.MatchString(sha) {
			return fmt.Errorf("-sha must be empty or a full 40-character lower-case commit hash of whatwg/html, got %q", sha)
		}
		url = WHATWGSnapshotURL(sha)
	}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	version := sha
	if sha == "" {
		if version = pinnedCommit(b); version == "" {
			// The live page does not always expose its commit. Pin by date instead; the vendored
			// extract is still reproducible from the committed file, just not re-fetchable by hash.
			version = "live-" + today.UTC().Format("2006-01-02")
			fmt.Fprintf(os.Stderr, "specgen2: warning: %s does not expose its commit hash; pinning as %s (pass -sha to pin a commit snapshot)\n", url, version)
		}
	}
	tables, err := ExtractIndexTables(bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("%s does not look like the HTML standard: %w", url, err)
	}
	idx, err := ParseIndices(bytes.NewReader(tables))
	if err != nil {
		return fmt.Errorf("%s: extracted index tables do not parse: %w", url, err)
	}
	if len(idx.Elements) == 0 || len(idx.Attributes) == 0 {
		return fmt.Errorf("%s: parsed %d elements and %d attributes; refusing to vendor", url, len(idx.Elements), len(idx.Attributes))
	}
	if err := os.MkdirAll(filepath.Join(dataDir, filepath.Dir(WHATWGFile)), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dataDir, WHATWGFile), tables, 0o644); err != nil {
		return err
	}
	m, err := ReadManifest(dataDir)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	m.Set(Source{
		Name:      "whatwg-html-indices",
		Version:   version,
		URL:       url,
		Integrity: "sha256-" + hex.EncodeToString(sum[:]),
		Retrieved: today.UTC().Format("2006-01-02"),
		License:   "CC-BY-4.0 (WHATWG HTML Living Standard)",
		Files:     []string{WHATWGFile},
	})

	return m.Write(dataDir)
}

func writeJSON(p string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}

	return os.WriteFile(p, buf.Bytes(), 0o644)
}
