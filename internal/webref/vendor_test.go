package webref

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tarball(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, name := range sortedKeys(files) {
		body := files[name]
		if err := tw.WriteHeader(&tar.Header{Name: "package/" + name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "pkg.tgz")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	return p
}

func TestVendorTarballs(t *testing.T) {
	data := t.TempDir()
	bcd := tarball(t, map[string]string{
		"package.json": `{"name":"@mdn/browser-compat-data","version":"1.2.3"}`,
		"data.json": `{"__meta":{"timestamp":"2026-01-01T00:00:00Z"},
			"html":{"elements":{"a":{"__compat":{"status":{"standard_track":true}},
				"href":{"__compat":{"spec_url":"https://example/#href","status":{"standard_track":true}}},
				"charset":{"__compat":{"spec_url":["https://html.spec.whatwg.org/multipage/obsolete.html#x"],"status":{"deprecated":true,"standard_track":true}}}}},
				"global_attributes":{"id":{"__compat":{"status":{"standard_track":true}}}}}}`,
	})
	elements := tarball(t, map[string]string{
		"package.json": `{"name":"@webref/elements","version":"9.9.9"}`,
		"html.json":    `{"spec":{"title":"T","url":"u"},"elements":[{"name":"a","interface":"HTMLAnchorElement"}]}`,
	})
	cfg := VendorConfig{ElementFiles: []string{"html.json"}, BCDTrees: []string{"html"}}
	if err := VendorTarballs(data, cfg, elements, "", bcd, time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(filepath.Join(data, "bcd/html.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ns BCDNamespace
	if err := json.Unmarshal(b, &ns); err != nil {
		t.Fatal(err)
	}
	if !ns.Elements["a"].Attributes["charset"].Obsolete() || ns.Elements["a"].Attributes["href"].Obsolete() {
		t.Errorf("trimmed BCD lost spec_url: %s", b)
	}
	if _, ok := ns.GlobalAttributes["id"]; !ok {
		t.Errorf("globals missing: %s", b)
	}

	m, err := ReadManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range m.Sources {
		names = append(names, s.Name+"@"+s.Version+":"+s.Status)
		if s.Version != "" && (!strings.HasPrefix(s.Integrity, "sha512-") || s.Retrieved != "2026-10-31") {
			t.Errorf("source %+v lacks integrity/date", s)
		}
	}
	want := "@mdn/browser-compat-data@1.2.3:,@webref/elements@9.9.9:,whatwg-html-indices@:pending"
	if strings.Join(names, ",") != want {
		t.Errorf("manifest = %s", strings.Join(names, ","))
	}
}

func TestFetchWHATWGRejectsBadSHA(t *testing.T) {
	if err := FetchWHATWG(nil, t.TempDir(), "main", time.Now()); err == nil {
		t.Error("expected an error for a non-hash revision")
	}
}

func TestPinnedCommit(t *testing.T) {
	const sha = "404b3eb14a7a936dddefb2d78e224dbeb01d2358"
	cases := map[string]string{
		`<a id="commit-snapshot-link" href="/commit-snapshots/` + sha + `/" class="changes">`: sha,
		`<a href="https://github.com/whatwg/html/commit/` + sha + `">`:                        sha,
		`<a href="https://github.com/whatwg/html/commits" class="changes">`:                   "",
	}
	for page, want := range cases {
		if got := pinnedCommit([]byte(page)); got != want {
			t.Errorf("pinnedCommit(%q) = %q, want %q", page, got, want)
		}
	}
}
