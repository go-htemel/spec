package spec

import (
	"bytes"
	"io"
	"testing"
)

func findElement(s *Spec, tag string) *Element {
	for _, e := range s.Elements {
		if e.Tag == tag {
			return e
		}
	}

	return nil
}

func TestGenerateHTMLSpec(t *testing.T) {
	htmlDoc := `
<html>
	<head></head>
	<body>
		<script></script>
		<p></p>
		<h2 id="skip-me"></h2>
		<h2 id="semantics"></h2>
		<h4 id="the-tag-element"><p><code>tag</code></p></h4><p>Good description</p><p>I shouldn't be in output</p>
		<h4 id="the-sub-and-sup-elements"><p><code>sub</code> and <code>sup</code></p></h4><p>Script description</p>
		<h4 id="the-dup-element"><p><code>dup</code> <code>dup</code> <code>tag</code></p></h4><p>Dup description</p>
		<h2 id="parsing-should-stop"><h4 id="the-badtag-element"><p><code>badtag</code></p></h4><p>Bad description</p></h2>
	</body>
</html>
`

	got, err := GenerateHTMLSpec(io.NopCloser(bytes.NewBufferString(htmlDoc)))
	if err != nil {
		t.Fatalf("GenerateHTMLSpec() error = %v", err)
	}
	if got.Name != "HTML" {
		t.Errorf("Name = %v, want HTML", got.Name)
	}

	want := map[string]string{
		"tag": "Good description",
		"sub": "Script description",
		"sup": "Script description",
		"dup": "Dup description",
	}
	for tag, desc := range want {
		e := findElement(got, tag)
		if e == nil {
			t.Errorf("element %q missing", tag)
			continue
		}
		if e.Description != desc {
			t.Errorf("element %q description = %q, want %q", tag, e.Description, desc)
		}
	}
	if findElement(got, "badtag") != nil {
		t.Errorf("badtag should not be parsed")
	}

	counts := map[string]int{}
	for _, e := range got.Elements {
		counts[e.Tag]++
	}
	for tag, n := range counts {
		if n != 1 {
			t.Errorf("element %q appears %d times", tag, n)
		}
	}

	for _, tag := range []string{"h2", "h3", "h4", "h5", "h6"} {
		e := findElement(got, tag)
		if e == nil || !e.Text {
			t.Errorf("%s should exist with Text=true, got %+v", tag, e)
		}
	}
}
