// Command specgen2 builds spec documents from committed, versioned web platform data instead of
// scraping the live HTML standard.
//
// Subcommands:
//
//	specgen2 generate [-data data] [-out specs/candidates] [-legacy specs] [-include-obsolete] [html svg mathml]
//	    Build candidate documents from the data directory. Offline and deterministic.
//	specgen2 vendor -data data -elements elements.tgz -idl idl.tgz -bcd bcd.tgz
//	    Refresh data/webref and data/bcd from npm tarballs and record them in data/SOURCES.json.
//	    Get the tarballs with e.g. `npm pack @webref/elements@2.9.2 @webref/idl@3.86.0 @mdn/browser-compat-data@8.1.7`.
//	specgen2 fetch -data data -sha <whatwg/html commit>
//	    Download https://html.spec.whatwg.org/commit-snapshots/<sha>/multipage/indices.html into
//	    data/whatwg/indices.html and record it in data/SOURCES.json.
//	specgen2 diff [-old specs/html.json] [-new specs/candidates/html.json] [-provenance ...]
//	    Print a markdown coverage report comparing two documents.
//	specgen2 report [-out docs/webref-coverage.md]
//	    Write the coverage report for html and svg (candidates vs committed specs).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-htemel/spec"
	"github.com/go-htemel/spec/internal/webref"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "specgen2:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "generate"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "generate":
		return generate(args)
	case "vendor":
		return vendor(args)
	case "fetch":
		return fetch(args)
	case "diff":
		return diff(args)
	case "report":
		return report(args)
	}

	return fmt.Errorf("unknown subcommand %q (want generate, vendor, fetch, diff or report)", cmd)
}

func generate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	data := fs.String("data", "data", "committed data directory")
	out := fs.String("out", webref.CandidateDir, "output directory")
	legacy := fs.String("legacy", "specs", "directory of previous documents to take descriptions from (empty to skip)")
	obsolete := fs.Bool("include-obsolete", false, "keep obsolete elements and attributes, marked deprecated")
	_ = fs.Parse(args)

	names := fs.Args()
	if len(names) == 0 {
		for _, c := range webref.Configs {
			names = append(names, c.Name)
		}
	}
	src, err := webref.LoadSources(*data)
	if err != nil {
		return err
	}
	if src.WHATWG == nil {
		fmt.Fprintf(os.Stderr, "specgen2: note: %s not vendored; using overlay stand-ins for categories, void flags and enumerated values\n",
			filepath.Join(*data, webref.WHATWGFile))
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	for _, name := range names {
		cfg, ok := webref.ConfigByName(name)
		if !ok {
			return fmt.Errorf("unknown spec %q", name)
		}
		if *legacy != "" {
			if err := src.LoadLegacy(name, filepath.Join(*legacy, name+".json")); err != nil {
				return err
			}
		}
		sp, prov, err := webref.Build(cfg, src, webref.Options{IncludeObsolete: *obsolete})
		if err != nil {
			return err
		}
		for _, w := range prov.Warnings {
			fmt.Fprintf(os.Stderr, "specgen2: %s: warning: %s\n", name, w)
		}
		if err := writeJSON(filepath.Join(*out, name+".json"), sp); err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(*out, name+".provenance.json"), prov); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "specgen2: wrote %s (%d elements, %d global attributes)\n",
			filepath.Join(*out, name+".json"), len(sp.Elements), len(sp.Attributes))
	}

	return nil
}

func vendor(args []string) error {
	fs := flag.NewFlagSet("vendor", flag.ExitOnError)
	data := fs.String("data", "data", "data directory to write")
	el := fs.String("elements", "", "@webref/elements tarball")
	idl := fs.String("idl", "", "@webref/idl tarball")
	bcd := fs.String("bcd", "", "@mdn/browser-compat-data tarball")
	_ = fs.Parse(args)
	if *el == "" && *idl == "" && *bcd == "" {
		return fmt.Errorf("vendor: give at least one of -elements, -idl, -bcd")
	}

	return webref.VendorTarballs(*data, webref.DefaultVendorConfig, *el, *idl, *bcd, time.Now())
}

func fetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	data := fs.String("data", "data", "data directory to write")
	sha := fs.String("sha", "", "full whatwg/html commit hash to pin")
	_ = fs.Parse(args)

	return webref.FetchWHATWG(&http.Client{Timeout: 2 * time.Minute}, *data, *sha, time.Now())
}

func diff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	oldPath := fs.String("old", "specs/html.json", "previous document")
	newPath := fs.String("new", filepath.Join(webref.CandidateDir, "html.json"), "candidate document")
	provPath := fs.String("provenance", "", "candidate provenance file (default: next to -new)")
	_ = fs.Parse(args)
	if *provPath == "" {
		*provPath = strings.TrimSuffix(*newPath, ".json") + ".provenance.json"
	}
	r, err := loadDiff(*oldPath, *newPath, *provPath)
	if err != nil {
		return err
	}
	fmt.Print(r.Markdown(*oldPath, *newPath))

	return nil
}

func report(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	out := fs.String("out", "docs/webref-coverage.md", "report file")
	data := fs.String("data", "data", "data directory (for SOURCES.json)")
	_ = fs.Parse(args)

	manifest, err := webref.ReadManifest(*data)
	if err != nil {
		return err
	}
	var diffs []webref.NamedDiff
	for _, name := range []string{"html", "svg"} {
		oldPath := filepath.Join("specs", name+".json")
		newPath := filepath.Join(webref.CandidateDir, name+".json")
		r, err := loadDiff(oldPath, newPath, strings.TrimSuffix(newPath, ".json")+".provenance.json")
		if err != nil {
			return err
		}
		diffs = append(diffs, webref.NamedDiff{Name: name, Old: oldPath, New: newPath, Diff: r})
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}

	return os.WriteFile(*out, []byte(webref.ReportPreservingNotes(*out, manifest, diffs)), 0o644)
}

func loadDiff(oldPath, newPath, provPath string) (*webref.Diff, error) {
	oldSp, err := readSpec(oldPath)
	if err != nil {
		return nil, err
	}
	newSp, err := readSpec(newPath)
	if err != nil {
		return nil, err
	}
	var prov *webref.Provenance
	if b, err := os.ReadFile(provPath); err == nil {
		prov = &webref.Provenance{}
		if err := json.Unmarshal(b, prov); err != nil {
			return nil, fmt.Errorf("%s: %w", provPath, err)
		}
	}

	return webref.Compare(oldSp, newSp, prov), nil
}

func readSpec(path string) (*spec.Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sp := &spec.Spec{}
	if err := json.Unmarshal(b, sp); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return sp, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(b, '\n'), 0o644)
}
