// Copyright 2026 Minigo contributors. Licensed under AGPL-3.0-or-later.
package api

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/minio/console/models"
	"github.com/minio/console/pkg/auth"
	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func minigoTestCookie(t *testing.T) *http.Cookie {
	t.Helper()
	token, err := auth.NewEncryptedTokenForClient(&credentials.Value{AccessKeyID: "session-access", SecretAccessKey: "session-secret", SessionToken: "session-sts-token"}, "parent-account", nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: token}
}

func TestMinigoNamespaceRequiresEncryptedSession(t *testing.T) {
	for _, route := range []string{"server", "users", "users/alice", "groups", "groups/team", "policies", "policies/read", "policy-associations", "service-accounts", "service-accounts/key", "config", "config/api", "buckets/photos/versioning", "buckets/photos/lifecycle", "buckets/photos/replication", "buckets/photos"} {
		for _, token := range []string{"", "Anonymous", "malformed", `{"stsAccessKeyID":"root","stsSecretAccessKey":"root-secret"}`} {
			t.Run(route+"/"+token, func(t *testing.T) {
				h := minigoMiddleware(http.NotFoundHandler(), func(*http.Request, *models.Principal) (any, error) {
					t.Fatal("Unauthenticated request reached dispatch")
					return nil, nil
				})
				r := httptest.NewRequest("GET", "http://console.test"+minigoAPIPrefix+route, nil)
				r.Header.Set("Authorization", "Bearer root")
				if token != "" {
					r.AddCookie(&http.Cookie{Name: "token", Value: token})
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != 401 {
					t.Fatalf("status %d, want 401", w.Code)
				}
				if strings.Contains(w.Body.String(), token) && token != "" {
					t.Fatal("Response echoed untrusted token")
				}
			})
		}
	}
}

func TestMinigoSessionIdentityPreserved(t *testing.T) {
	h := minigoMiddleware(http.NotFoundHandler(), func(r *http.Request, p *models.Principal) (any, error) {
		if p.STSAccessKeyID != "session-access" || p.STSSecretAccessKey != "session-secret" || p.STSSessionToken != "session-sts-token" || p.AccountAccessKey != "parent-account" {
			t.Fatal("Session identity changed")
		}
		if _, ok := r.Context().Deadline(); !ok {
			t.Fatal("No upstream deadline")
		}
		return map[string]bool{"ok": true}, nil
	})
	r := httptest.NewRequest("GET", "http://console.test"+minigoAPIPrefix+"users", nil)
	r.AddCookie(minigoTestCookie(t))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d headers %v", w.Code, w.Header())
	}
}

func TestMinigoDoesNotInterceptExistingRoutes(t *testing.T) {
	for _, path := range []string{"/api/v1/session", "/api/v1/buckets", "/ws/objectManager", "/api/v1/minigoevil/server", "/"} {
		h := minigoMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }), func(*http.Request, *models.Principal) (any, error) {
			t.Fatal("Intercepted original route")
			return nil, nil
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 204 {
			t.Fatalf("route %s: status %d", path, w.Code)
		}
	}
}

func TestMinigoUnknownRoutesAndUnsupportedMethods(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "unknown", 404}, {"GET", "server/extra", 404}, {"GET", "users/alice/extra", 404},
		{"GET", "groups/team/extra", 404}, {"GET", "policies/read/extra", 404}, {"GET", "service-accounts/key/extra", 404},
		{"GET", "buckets/photos/unknown", 404}, {"GET", "config/api/extra", 404},
		{"PUT", "server", 405}, {"POST", "users", 405}, {"POST", "groups", 405}, {"POST", "policies", 405},
		{"GET", "policy-associations", 405}, {"POST", "buckets/photos/versioning", 405},
	} {
		r := httptest.NewRequest(tc.method, "http://console.test"+minigoAPIPrefix+tc.path, strings.NewReader(`{}`))
		r.AddCookie(minigoTestCookie(t))
		r.Header.Set("Origin", "http://console.test")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		minigoMiddleware(http.NotFoundHandler(), minigoDispatch).ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s %s: status %d, want %d: %s", tc.method, tc.path, w.Code, tc.want, w.Body.String())
		}
	}
}

func TestMinigoCSRFAndContentType(t *testing.T) {
	t.Setenv(ConsoleBrowserRedirectURL, "https://public.example/console/")
	cases := []struct {
		name, method, origin, contentType, fetchSite string
		want                                         int
	}{
		{"same origin", "PUT", "http://console.test", "application/json", "same-origin", 200},
		{"TLS proxy", "PUT", "https://console.test", "application/json; charset=utf-8", "same-origin", 200},
		{"configured proxy", "PUT", "https://public.example", "application/json", "same-origin", 200},
		{"missing origin", "PUT", "", "application/json", "", 403},
		{"cross origin", "PUT", "http://evil.test", "application/json", "", 403},
		{"same site sibling", "PUT", "http://other.console.test", "application/json", "same-site", 403},
		{"null origin", "PUT", "null", "application/json", "", 403},
		{"credential URL", "PUT", "http://x@console.test", "application/json", "", 403},
		{"origin path", "PUT", "http://console.test/fake", "application/json", "", 403},
		{"origin query", "PUT", "http://console.test?fake", "application/json", "", 403},
		{"bad port", "PUT", "http://console.test:8080", "application/json", "", 403},
		{"form", "POST", "http://console.test", "application/x-www-form-urlencoded", "", 415},
		{"text", "POST", "http://console.test", "text/plain", "", 415},
		{"delete origin", "DELETE", "", "application/json", "", 403},
		{"delete JSON", "DELETE", "http://console.test", "", "", 415},
		{"cross site metadata", "PUT", "http://console.test", "application/json", "cross-site", 403},
		{"cross origin GET", "GET", "http://evil.test", "", "", 403},
		{"same origin GET", "GET", "", "", "same-origin", 200},
	}
	cookie := minigoTestCookie(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			h := minigoMiddleware(http.NotFoundHandler(), func(*http.Request, *models.Principal) (any, error) { called = true; return nil, nil })
			r := httptest.NewRequest(tc.method, "http://console.test"+minigoAPIPrefix+"users/alice", strings.NewReader(`{}`))
			r.AddCookie(cookie)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Content-Type", tc.contentType)
			r.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			r.Header.Set("X-Forwarded-Host", "evil.test")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || called != (tc.want == 200) {
				t.Fatalf("status %d want %d, dispatched %v", w.Code, tc.want, called)
			}
		})
	}
	r := httptest.NewRequest("PUT", "https://console.test"+minigoAPIPrefix+"users/alice", nil)
	r.TLS = &tls.ConnectionState{}
	r.Header.Set("Origin", "http://console.test")
	r.Header.Set("Content-Type", "application/json")
	if minigoCheckRequest(r) == nil {
		t.Fatal("Accepted insecure origin for direct TLS")
	}
	r.Header.Set("Origin", "https://console.test")
	r.Header.Add("Origin", "http://evil.test")
	if minigoCheckRequest(r) == nil {
		t.Fatal("Accepted multiple Origin headers")
	}
}

func TestMinigoRequestBodyLimit(t *testing.T) {
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		h := minigoMiddleware(http.NotFoundHandler(), func(*http.Request, *models.Principal) (any, error) {
			t.Fatal("Oversize body reached dispatch")
			return nil, nil
		})
		r := httptest.NewRequest(method, "http://console.test"+minigoAPIPrefix+"users/alice", strings.NewReader(strings.Repeat("x", minigoMaxBody+1)))
		r.ContentLength = -1 // exercise chunked requests, not just Content-Length
		r.AddCookie(minigoTestCookie(t))
		r.Header.Set("Origin", "http://console.test")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 413 {
			t.Fatalf("%s: status %d", method, w.Code)
		}
	}
}

func TestMinigoRejectsAmbiguousJSON(t *testing.T) {
	for _, body := range []string{`null`, `[]`, `"string"`, `{"extra":"secret"}`, `{} {}`, `{"name":`, `{}` + "x"} {
		var dst struct {
			Name string `json:"name"`
		}
		if err := minigoDecode(httptest.NewRequest("PUT", "/", strings.NewReader(body)), &dst); err == nil {
			t.Fatalf("Accepted invalid body: %s", body)
		}
	}
}

func TestMinigoUpstreamUsesSessionCredentialsAndEnforcesPermissions(t *testing.T) {
	var signedRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "Credential=session-access/") || r.Header.Get("X-Amz-Security-Token") != "session-sts-token" {
			t.Error("Admin call did not use the logged-in STS credentials")
		}
		signedRequests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		_, _ = fmt.Fprint(w, `{"Code":"AccessDenied","Message":"do-not-disclose-upstream-secret"}`)
	}))
	defer server.Close()
	t.Setenv(ConsoleMinIOServer, server.URL)
	h := minigoMiddleware(http.NotFoundHandler(), minigoDispatch)
	r := httptest.NewRequest("GET", "http://console.test"+minigoAPIPrefix+"users", nil)
	r.AddCookie(minigoTestCookie(t))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 || signedRequests == 0 {
		t.Fatalf("status %d, signed requests %d: %s", w.Code, signedRequests, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "do-not-disclose") {
		t.Fatal("Leaked upstream error details")
	}
}

func TestMinigoResponsesOmitServerSecrets(t *testing.T) {
	server := madmin.InfoMessage{Servers: []madmin.ServerProperties{{Endpoint: "server:9000", State: "online", MinioEnvVars: map[string]string{"MINIO_ROOT_PASSWORD": "server-secret"}}}}
	data, err := json.Marshal(minigoServerInfo(server))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "server-secret") || strings.Contains(string(data), "MINIO_ROOT_PASSWORD") {
		t.Fatal("Server info leaked environment values")
	}
	data, err = json.Marshal(minigoSafeUser("alice", madmin.UserInfo{SecretKey: "user-secret", Status: madmin.AccountEnabled}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "user-secret") || strings.Contains(string(data), "secretKey") {
		t.Fatal("User info leaked a secret")
	}
}

func TestMinigoServiceAccountsPreserveCallerDefault(t *testing.T) {
	for _, user := range []string{"", "other-account"} {
		t.Run(user, func(t *testing.T) {
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if got := r.URL.Query().Get("user"); got != user {
					t.Errorf("upstream user %q, want %q", got, user)
				}
				if !strings.Contains(r.Header.Get("Authorization"), "Credential=session-access/") || r.Header.Get("X-Amz-Security-Token") != "session-sts-token" {
					t.Error("Service account listing did not use caller STS credentials")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(403)
				_, _ = fmt.Fprint(w, `{"Code":"AccessDenied","Message":"denied"}`)
			}))
			defer server.Close()
			t.Setenv(ConsoleMinIOServer, server.URL)
			r := httptest.NewRequest("GET", "http://console.test"+minigoAPIPrefix+"service-accounts?user="+url.QueryEscape(user), nil)
			r.AddCookie(minigoTestCookie(t))
			w := httptest.NewRecorder()
			minigoMiddleware(http.NotFoundHandler(), minigoDispatch).ServeHTTP(w, r)
			if !called || w.Code != 403 {
				t.Fatalf("called %t, status %d: %s", called, w.Code, w.Body.String())
			}
		})
	}
}

func TestMinigoEncodedIdentifiersReachUpstreamUnchanged(t *testing.T) {
	for _, name := range []string{"team/member", "团队/%value"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("accessKey") != name {
				t.Errorf("upstream identifier %q, want %q", r.URL.Query().Get("accessKey"), name)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(403)
			_, _ = fmt.Fprint(w, `{"Code":"AccessDenied","Message":"denied"}`)
		}))
		t.Setenv(ConsoleMinIOServer, server.URL)
		r := httptest.NewRequest("GET", "http://console.test"+minigoAPIPrefix+"users/"+url.PathEscape(name), nil)
		r.AddCookie(minigoTestCookie(t))
		w := httptest.NewRecorder()
		minigoMiddleware(http.NotFoundHandler(), minigoDispatch).ServeHTTP(w, r)
		server.Close()
		if w.Code != 403 {
			t.Fatalf("identifier %q: status %d: %s", name, w.Code, w.Body.String())
		}
	}
}

func TestMinigoBucketOperationsEnforceStorageAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "Credential=session-access/") || r.Header.Get("X-Amz-Security-Token") != "session-sts-token" {
			t.Error("Bucket call did not use the logged-in STS credentials")
		}
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Query().Has("location") {
			_, _ = fmt.Fprint(w, `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)
			return
		}
		w.WriteHeader(403)
		_, _ = fmt.Fprint(w, `<Error><Code>AccessDenied</Code><Message>denied</Message></Error>`)
	}))
	defer server.Close()
	t.Setenv(ConsoleMinIOServer, server.URL)
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "versioning", ""}, {"PUT", "versioning", `{"Status":"Enabled"}`},
		{"GET", "lifecycle", ""}, {"PUT", "lifecycle", `{"Rules":[{"ID":"expire","Status":"Enabled","Expiration":{"Days":30}}]}`}, {"DELETE", "lifecycle", ""},
		{"GET", "replication", ""}, {"PUT", "replication", `{"Rules":[{"ID":"copy","Status":"Enabled","Priority":1,"Destination":{"Bucket":"arn:aws:s3:::target"}}]}`}, {"DELETE", "replication", ""},
		{"DELETE", "", ""},
	} {
		t.Run(tc.method+"/"+tc.path, func(t *testing.T) {
			path := minigoAPIPrefix + "buckets/photos"
			if tc.path != "" {
				path += "/" + tc.path
			}
			r := httptest.NewRequest(tc.method, "http://console.test"+path, strings.NewReader(tc.body))
			r.AddCookie(minigoTestCookie(t))
			r.Header.Set("Origin", "http://console.test")
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			minigoMiddleware(http.NotFoundHandler(), minigoDispatch).ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatalf("status %d, want 403: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestMinigoConfigAllowlistAndInjection(t *testing.T) {
	allowed := map[string]bool{"requests_max": true, "cluster_deadline": true}
	values, err := minigoParseConfig(`api requests_max=10 cluster_deadline="10s" secret_key="sensitive" comment="requests_max=secret"`, "api", allowed)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values["requests_max"] != "10" || values["cluster_deadline"] != "10s" {
		t.Fatalf("Unexpected config: %#v", values)
	}
	for _, input := range []map[string]string{{"root_access": "off"}, {"requests_max": "1\nidentity_openid client_secret=secret"}, {"requests_max": `1" root_access="off`}, {"requests_max": "1 key=value"}} {
		if _, err := minigoConfigUpdate("api", input, allowed); err == nil {
			t.Fatalf("Accepted unsafe config: %#v", input)
		}
	}
	kv, err := minigoConfigUpdate("api", map[string]string{"requests_max": "10", "cluster_deadline": "10s"}, allowed)
	if err != nil || kv != `api cluster_deadline="10s" requests_max="10"` {
		t.Fatalf("config %q, error %v", kv, err)
	}
	if _, err := minigoParseConfig(`api requests_max="unterminated`, "api", allowed); err == nil {
		t.Fatal("Accepted malformed upstream config")
	}
	if _, err := minigoParseConfig(`identity_openid secret_key=secret`, "api", allowed); err == nil {
		t.Fatal("Accepted wrong subsystem")
	}
}
