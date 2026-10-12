package api

import (
	"net/http/httptest"
	"testing"
)

func TestConsoleWebsocketOriginSecurity(t *testing.T) {
	t.Setenv(ConsoleBrowserRedirectURL, "https://storage.example.com/console/")
	t.Setenv(SubPath, "/console/")
	t.Setenv(ConsoleDevMode, "on")
	for _, tc := range []struct {
		name, origin string
		want         bool
	}{
		{"non-browser", "", true},
		{"same host", "http://internal:9001", true},
		{"configured proxy", "https://storage.example.com", true},
		{"wrong scheme", "http://storage.example.com", false},
		{"unrelated site", "https://attacker.example", false},
		{"suffix attack", "https://storage.example.com.attacker.example", false},
		{"opaque", "null", false},
		{"userinfo", "https://attacker@storage.example.com", false},
		{"path", "https://storage.example.com/attacker", false},
		{"query", "https://storage.example.com?attacker", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://internal:9001/ws/objectManager", nil)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if got := consoleWebsocketOriginAllowed(r); got != tc.want {
				t.Fatalf("origin %q: got %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
	r := httptest.NewRequest("GET", "http://internal:9001/ws/objectManager", nil)
	r.Header.Add("Origin", "http://internal:9001")
	r.Header.Add("Origin", "https://attacker.example")
	if consoleWebsocketOriginAllowed(r) {
		t.Fatal("multiple origins accepted")
	}
}
