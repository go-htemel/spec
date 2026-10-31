# Spec

This package defines all the primitive types used in by the htemel generators.

## Usage

This package was split out of the `go-htemel/htemel` package in an effort to reduce the footprint of the original package.
The generator/parser code for creating specs is a little gnarly but suffices for now.

### Install

```text
go get github.com/go-htemel/spec
```

## Generating specs from web platform data (prototype)

`cmd/specgen2` builds spec documents offline from committed, versioned inputs under `data/`
instead of scraping the live HTML standard:

- `data/webref/elements`, `data/webref/idl`: element lists and Web IDL from `@webref/elements` and
  `@webref/idl` (reflected IDL attributes give content attribute names and types).
- `data/bcd`: the html/svg/mathml part of `@mdn/browser-compat-data` (deprecation, attributes not
  reflected in IDL, global attributes).
- `data/whatwg/indices.html`: a pinned commit snapshot of the HTML standard's index tables
  (categories, content models, attribute value types). Not vendored yet.
- `data/overlay/<spec>.json`: hand-maintained judgment calls, each with a reason.
- `data/SOURCES.json`: package versions, integrity hashes and retrieval dates.

```text
go run ./cmd/specgen2 generate            # writes specs/candidates/{html,svg,mathml}.json (+ .provenance.json)
go run ./cmd/specgen2 report              # writes docs/webref-coverage.md (candidates vs specs/*.json)
go run ./cmd/specgen2 diff -old specs/svg.json -new specs/candidates/svg.json
go run ./cmd/specgen2 vendor -elements elements.tgz -idl idl.tgz -bcd bcd.tgz   # tarballs from `npm pack`
go run ./cmd/specgen2 fetch -sha <whatwg/html commit>                          # needs network access
```

The candidates are not yet used in place of `specs/html.json` and `specs/svg.json`; see
`docs/webref-coverage.md` for how they differ.

## Warning

This package should not be directly used.

## License

MIT
