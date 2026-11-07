// Copyright 2026 Minigo contributors. Licensed under AGPL-3.0-or-later.
package api

import (
	"crypto/x509"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestMinigoSessionCookiesFollowConfiguredHTTPS(t *testing.T) {
	old := GlobalPublicCerts
	t.Cleanup(func() { GlobalPublicCerts = old })
	for _, tc := range []struct {
		name, publicURL string
		tls, secure     bool
	}{
		{"local HTTP", "", false, false},
		{"configured HTTP", "http://storage.example/console/", false, false},
		{"proxy HTTPS", "https://storage.example/console/", false, true},
		{"direct TLS", "", true, true},
		{"TLS overrides HTTP setting", "http://storage.example", true, true},
		{"relative URL", "//storage.example", false, false},
		{"malformed URL", "https://%", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(ConsoleBrowserRedirectURL, tc.publicURL)
			GlobalPublicCerts = nil
			if tc.tls {
				GlobalPublicCerts = []*x509.Certificate{{}}
			}
			for _, cookie := range []http.Cookie{NewSessionCookieForConsole("session-secret"), ExpireSessionCookie()} {
				if cookie.Secure != tc.secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
					t.Fatalf("unexpected cookie security attributes: Secure=%t HttpOnly=%t SameSite=%v", cookie.Secure, cookie.HttpOnly, cookie.SameSite)
				}
			}
		})
	}
}

func TestMinigoDebugHeadersDoNotExposeCredentials(t *testing.T) {
	headers := http.Header{
		"Authorization":        {"Bearer authorization-secret"},
		"Proxy-Authorization":  {"Basic proxy-secret"},
		"Cookie":               {"token=session-secret"},
		"Set-Cookie":           {"token=response-secret"},
		"X-Amz-Security-Token": {"sts-secret"},
		"X-Api-Key":            {"api-secret"},
		"X-Amz-Server-Side-Encryption-Customer-Key": {"encryption-secret"},
		"X-Custom-Header": {"custom-secret"},
		"Content-Type":    {"application/json"},
		"X-Request-Id":    {"request-123"},
	}
	var output strings.Builder
	debugLogHeaders(&output, headers)
	if strings.Contains(output.String(), "secret") {
		t.Fatalf("debug headers leaked sensitive values: %s", output.String())
	}
	for _, value := range []string{"application/json", "request-123"} {
		if !strings.Contains(output.String(), value) {
			t.Errorf("debug headers lost useful diagnostic %q", value)
		}
	}
	if headers.Get("Authorization") != "Bearer authorization-secret" {
		t.Fatal("logging changed request authentication")
	}
}

func TestMinigoDebugURLRedactsAllQueryValues(t *testing.T) {
	u, err := url.Parse("https://username:password@storage.example/api/v1/download?token=session-secret&X-Amz-Signature=signature-secret&code=oauth-secret&unknown=custom-secret#fragment-secret")
	if err != nil {
		t.Fatal(err)
	}
	before := u.String()
	logged := debugLogURL(u)
	for _, secret := range []string{"secret", "username", "password"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("URL logging leaked %q: %s", secret, logged)
		}
	}
	if !strings.Contains(logged, "/api/v1/download") || u.String() != before {
		t.Fatal("logging lost the path or changed the original URL")
	}
}
