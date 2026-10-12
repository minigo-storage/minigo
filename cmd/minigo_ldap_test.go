// Copyright (c) 2026 Minigo contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later

package cmd

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func ldapRateRequest(remote string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "http://minigo.test/", nil)
	r.RemoteAddr = remote
	return r
}

func TestMinigoLDAPRateLimitBySocketPeer(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLDAPAuthLimiter(rate.Every(time.Second), 2, 10, time.Minute)
	for _, remote := range []string{"192.0.2.1:1000", "[::ffff:192.0.2.1]:2000"} {
		if !l.allow(ldapRateRequest(remote), now) {
			t.Fatal("initial peer burst was rejected")
		}
	}
	// Spoofing forwarded headers and opening another TCP connection must not
	// obtain a new budget for the same actual peer.
	r := ldapRateRequest("192.0.2.1:3000")
	r.Header.Set("X-Forwarded-For", "198.51.100.10")
	r.Header.Set("X-Real-IP", "198.51.100.11")
	r.Header.Set("Forwarded", "for=198.51.100.12")
	if l.allow(r, now) {
		t.Fatal("forwarded headers or source port bypassed the limit")
	}
	if !l.allow(ldapRateRequest("192.0.2.2:1000"), now) {
		t.Fatal("another peer incorrectly shared the exhausted budget")
	}
	if !l.allow(r, now.Add(time.Second)) || l.allow(r, now.Add(time.Second)) {
		t.Fatal("expected exactly one replenished attempt after one second")
	}
}

func TestMinigoLDAPRateLimitBoundedPeerMemory(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLDAPAuthLimiter(rate.Every(time.Second), 2, 2, 10*time.Minute)
	for i := 0; i < 1000; i++ {
		l.allow(ldapRateRequest(fmt.Sprintf("[2001:db8::%x]:2000", i)), now)
	}
	if len(l.peers) != 2 {
		t.Fatalf("peer map grew to %d entries, want cap of 2", len(l.peers))
	}
	if l.allow(ldapRateRequest("203.0.113.1:2000"), now) {
		t.Fatal("untracked peers must share an exhausted overflow budget")
	}
	// Active entries must not be evicted by churn to reset their budgets.
	peer := ldapRateRequest("[2001:db8::]:4000")
	if !l.allow(peer, now) || l.allow(peer, now) {
		t.Fatal("peer churn reset or consumed a tracked peer's independent budget")
	}
	if !l.allow(ldapRateRequest("203.0.113.1:2000"), now.Add(11*time.Minute)) || len(l.peers) != 1 {
		t.Fatal("idle peers were not reclaimed")
	}
}

func TestMinigoLDAPRateLimitConcurrentAttempts(t *testing.T) {
	l := newLDAPAuthLimiter(rate.Every(time.Second), 30, 10, time.Minute)
	now := time.Unix(1000, 0)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.allow(ldapRateRequest("192.0.2.1:1234"), now) {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 30 {
		t.Fatalf("accepted %d concurrent requests, want burst of 30", accepted.Load())
	}
}

func TestMinigoLDAPPeerNormalization(t *testing.T) {
	for _, remote := range []string{"127.0.0.1:1000", "[::ffff:127.0.0.1]:2000", "127.0.0.1"} {
		if got := ldapAuthPeerKey(remote); got != "127.0.0.1" {
			t.Fatalf("%q normalized to %q", remote, got)
		}
	}
	for _, remote := range []string{"", "attacker-selected-name:1234", "malformed"} {
		if got := ldapAuthPeerKey(remote); got != "unknown" {
			t.Fatalf("invalid address %q created unique peer key %q", remote, got)
		}
	}
	if ldapAuthPeerKey("[fe80::1%en0]:1234") != ldapAuthPeerKey("[fe80::1%en1]:1234") {
		t.Fatal("IPv6 scope unexpectedly created a new peer budget")
	}
}

func TestMinigoLDAPAuthenticationFailureDoesNotExposeDirectory(t *testing.T) {
	w := httptest.NewRecorder()
	writeLDAPAuthenticationFailure(context.Background(), w)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", w.Code)
	}
	var response STSErrorResponse
	if err := xml.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != "AccessDenied" || response.Error.Message != "LDAP authentication failed." {
		t.Fatalf("unexpected public error response: %+v", response.Error)
	}
}

func TestMinigoLDAPHandlerRejectsExhaustedPeerBeforeBinding(t *testing.T) {
	previous := minigoLDAPAuthLimit
	minigoLDAPAuthLimit = newLDAPAuthLimiter(0, 1, 10, time.Minute)
	t.Cleanup(func() { minigoLDAPAuthLimit = previous })
	remote := "192.0.2.15:1234"
	minigoLDAPAuthLimit.allow(ldapRateRequest(remote), time.Now())
	form := url.Values{
		stsAction: {ldapIdentity}, stsVersion: {stsAPIVersion},
		stsLDAPUsername: {"directory-user"}, stsLDAPPassword: {"do-not-return-this-password"},
	}
	r := httptest.NewRequest(http.MethodPost, "http://minigo.test/", strings.NewReader(form.Encode()))
	r.RemoteAddr = remote
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("X-Forwarded-For", "198.51.100.25")
	w := httptest.NewRecorder()
	(&stsAPIHandlers{}).AssumeRoleWithLDAPIdentity(w, r)
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("expected 429 with Retry-After, got %d: %s", w.Code, w.Body.String())
	}
	var response STSErrorResponse
	if err := xml.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != "ThrottlingException" || strings.Contains(w.Body.String(), "directory-user") || strings.Contains(w.Body.String(), "do-not-return") {
		t.Fatalf("unsafe throttling response: %s", w.Body.String())
	}
}
