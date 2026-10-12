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
	for _, base := range []string{
		"//attacker/", "///attacker/", "https://attacker/", "relative/path/", "%2frelative/", `/"><script>alert(1)</script>`,
		"/path?query=1", "/path?", "/path#fragment", "/path#", "/path\\evil/", "/%2fattacker/", "/%5cattacker/",
		"/./", "/../", "/path/../other/", "/%2e%2e/", "/path/%2E./", "/path%2f..%2fother/",
		"/path\n/", "/%00/", "/%09/", "/%7f/", "/%22%3e%3cscript%3e/", "/invalid%escape/",
	} {
		if got := string(replaceBaseInIndex(html, base)); got != string(html) {
			t.Fatalf("unsafe base %q was inserted: %s", base, got)
		}
	}
}

func TestConsoleBasePathPreservesValidURLPaths(t *testing.T) {
	index := []byte(`<head><base href="/"/></head>`)
	for _, tc := range []struct{ base, href string }{
		{"/minio.v1/", "/minio.v1/"}, {"/.well-known/console/", "/.well-known/console/"}, {"/storage.../", "/storage.../"},
		{"/控制台/", "/%E6%8E%A7%E5%88%B6%E5%8F%B0/"}, {"/%E6%8E%A7%E5%88%B6%E5%8F%B0/", "/%E6%8E%A7%E5%88%B6%E5%8F%B0/"},
		{"/console%20space/", "/console%20space/"}, {"/100%25/", "/100%25/"}, {"/a%2fb/", "/a%2fb/"},
		{"/dollar$1/", "/dollar$1/"}, {"/releases$v2/", "/releases$v2/"}, {"/a&b/", "/a&amp;b/"}, {"/a&%2347;b/", "/a&amp;%2347;b/"},
	} {
		want := `<head><base href="` + tc.href + `"/></head>`
		if got := string(replaceBaseInIndex(index, tc.base)); got != want {
			t.Fatalf("base %q: got %s, want %s", tc.base, got, want)
		}
	}
}
