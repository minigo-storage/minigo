package api

import (
	"strings"
	"testing"
)

func TestConsoleBasePathVariants(t *testing.T) {
	for _, tag := range []string{`<base href="/"/>`, `<base href="/" />`, `<base href="/">`} {
		got := string(replaceBaseInIndex([]byte("<head>"+tag+"</head>"), "/storage_console/"))
		if !strings.Contains(got, `href="/storage_console/"`) {
			t.Fatalf("base substitution failed: %s", got)
		}
	}
}

func TestConsoleBasePathCannotChangeOrigin(t *testing.T) {
	html := []byte(`<head><base href="/"/></head>`)
	for _, base := range []string{"//attacker/", "https://attacker/", `/"><script>alert(1)</script>`, "/path?query=1", "/path\\evil/"} {
		if got := string(replaceBaseInIndex(html, base)); got != string(html) {
			t.Fatalf("unsafe base %q was inserted: %s", base, got)
		}
	}
}
