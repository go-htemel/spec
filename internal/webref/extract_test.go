package webref

import (
	"bytes"
	"os"
	"testing"
)

func TestExtractIndexTablesRoundTrip(t *testing.T) {
	b, err := os.ReadFile("testdata/indices_synthetic.html")
	if err != nil {
		t.Skip(err)
	}
	out, err := ExtractIndexTables(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	idx, err := ParseIndices(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Elements) == 0 || len(idx.Attributes) == 0 {
		t.Fatalf("empty: %d/%d", len(idx.Elements), len(idx.Attributes))
	}
	if len(out) >= len(b) {
		t.Logf("note: extract (%d) not smaller than fixture (%d) — fine for a fixture that is only tables", len(out), len(b))
	}
}
