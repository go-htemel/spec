package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-htemel/spec"
)

type Config struct {
	outputDir    string
	all          bool
	htmlOnly     bool
	htmlSpecSite string
}

func main() {
	cfg := Config{}

	flag.StringVar(&cfg.outputDir, "output", "specs", "Directory to write spec files to")
	flag.BoolVar(&cfg.all, "all", true, "Generate all spec files")
	flag.BoolVar(&cfg.htmlOnly, "html", false, "Only generate HTML spec files")
	flag.StringVar(&cfg.htmlSpecSite, "html-spec-site", "https://html.spec.whatwg.org/", "HTML spec site name")
	flag.Parse()

	if _, err := os.Stat(cfg.outputDir); err != nil {
		if err = os.MkdirAll(cfg.outputDir, 0755); err != nil {
			log.Fatal(err)
		}
	}

	if cfg.htmlOnly || cfg.all {
		if err := generateHTML(cfg); err != nil {
			log.Fatal(err)
		}
	}
}

func generateHTML(cfg Config) (err error) {
	path := filepath.Join(cfg.outputDir, "html.json")

	resp, err := http.Get(cfg.htmlSpecSite)
	if err != nil {
		return err
	}
	// GenerateHTMLSpec closes the body.
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return fmt.Errorf("fetching %s: unexpected status %s", cfg.htmlSpecSite, resp.Status)
	}

	out, err := spec.GenerateHTMLSpec(resp.Body)
	if err != nil {
		return err
	}

	jsonOut, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = f.Write(jsonOut)

	return err
}
