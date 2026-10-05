package correlation

import (
	"strings"
	"testing"
)

func TestExpansionEscapingAndLiteralValues(t *testing.T) {
	tmpl, err := Compile(`{{value|json}} {{value|url}} {{value|path}} {{literal}}`, map[string]bool{"value": true, "literal": true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := tmpl.Expand(map[string]string{"value": "a/b &\"c", "literal": "{{other}}"})
	if err != nil {
		t.Fatal(err)
	}
	want := `"a/b \u0026\"c" a%2Fb+%26%22c a%2Fb%20&%22c {{other}}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTemplateRejectsMissingVariablesAndOversizedExpansion(t *testing.T) {
	for _, raw := range []string{"{{unknown}}", "{{known|execute}}", "{{known", "stray }}", "{{known|url|json}}"} {
		if _, err := Compile(raw, map[string]bool{"known": true}); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	tmpl, _ := Compile("{{known}}", map[string]bool{"known": true})
	if _, err := tmpl.Expand(nil); err == nil {
		t.Fatal("missing value accepted")
	}
	tmpl, _ = Compile("{{known}}{{known}}", map[string]bool{"known": true})
	if _, err := tmpl.Expand(map[string]string{"known": strings.Repeat("x", MaxValueBytes/2+1)}); err == nil {
		t.Fatal("unbounded expansion")
	}
}

func TestJSONPointerEscapesArraysAndExactNumbers(t *testing.T) {
	doc, err := DecodeJSON([]byte(`{"a/b":{"~key":["value",9007199254740993,true,null]},"":{"":false}}`))
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"/a~1b/~0key/0": "value", "/a~1b/~0key/1": "9007199254740993", "/a~1b/~0key/2": "true", "//": "false"} {
		p, err := ParsePointer(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := p.Scalar(doc)
		if err != nil || got != want {
			t.Errorf("%s: got %q, err %v", path, got, err)
		}
	}
	for _, path := range []string{"/a~1b/~0key/01", "/a~1b/~0key/-", "/a~1b/~0key/+1", "/a~1b/~0key/3", "/missing", "/a~1b"} {
		p, _ := ParsePointer(path)
		if _, err := p.Scalar(doc); err == nil {
			t.Errorf("accepted unavailable or nonscalar pointer %q", path)
		}
	}
	for _, path := range []string{"a/b", "/~2bad", "/trailing~"} {
		if _, err := ParsePointer(path); err == nil {
			t.Errorf("accepted invalid pointer %q", path)
		}
	}
	if _, err := DecodeJSON([]byte(`{} {}`)); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}
