// Copyright (c) 2026 Minigo contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later

package cmd

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	xhttp "github.com/minio/minio/internal/http"
	"golang.org/x/time/rate"
)

const (
	// Each server limits LDAP binds by the actual TCP peer. Forwarded headers
	// are deliberately ignored: trusting arbitrary X-Forwarded-For values would
	// allow an unauthenticated caller to bypass the limit.
	minigoLDAPAuthBurst = 30
	minigoLDAPMaxPeers  = 4096
	minigoLDAPPeerIdle  = 10 * time.Minute
)

var (
	minigoLDAPAuthFailed = errors.New("LDAP authentication failed.")
	minigoLDAPAuthLimit  = newLDAPAuthLimiter(rate.Every(time.Second), minigoLDAPAuthBurst, minigoLDAPMaxPeers, minigoLDAPPeerIdle)
)

type ldapAuthPeer struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type ldapAuthLimiter struct {
	mu        sync.Mutex
	peers     map[string]*ldapAuthPeer
	overflow  *rate.Limiter
	limit     rate.Limit
	burst     int
	maxPeers  int
	idle      time.Duration
	nextSweep time.Time
}

func newLDAPAuthLimiter(limit rate.Limit, burst, maxPeers int, idle time.Duration) *ldapAuthLimiter {
	return &ldapAuthLimiter{
		peers: make(map[string]*ldapAuthPeer), overflow: rate.NewLimiter(limit, burst),
		limit: limit, burst: burst, maxPeers: maxPeers, idle: idle,
	}
}

func ldapAuthPeerKey(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		// Malformed or missing addresses share a bounded fallback bucket.
		return "unknown"
	}
	return ip.Unmap().WithZone("").String()
}

func (l *ldapAuthLimiter) allow(r *http.Request, now time.Time) bool {
	key := ldapAuthPeerKey(r.RemoteAddr)
	l.mu.Lock()
	defer l.mu.Unlock()

	if !now.Before(l.nextSweep) {
		for key, peer := range l.peers {
			if now.Sub(peer.lastSeen) >= l.idle {
				delete(l.peers, key)
			}
		}
		// Bound sweep CPU cost independently of an attacker's new peer count.
		l.nextSweep = now.Add(time.Minute)
	}
	if peer := l.peers[key]; peer != nil {
		peer.lastSeen = now
		return peer.limiter.AllowN(now, 1)
	}
	if len(l.peers) >= l.maxPeers {
		// Do not evict active peers, which would reset their budgets. Unknown
		// peers share this bounded overflow budget until idle slots expire.
		return l.overflow.AllowN(now, 1)
	}
	peer := &ldapAuthPeer{limiter: rate.NewLimiter(l.limit, l.burst), lastSeen: now}
	l.peers[key] = peer
	return peer.limiter.AllowN(now, 1)
}

func writeLDAPAuthenticationFailure(ctx context.Context, w http.ResponseWriter) {
	// Username lookup, password bind and missing policy failures have the same
	// status and body. Never return LDAP's diagnostic text, user DN or groups.
	writeSTSErrorResponse(ctx, w, ErrSTSAccessDenied, minigoLDAPAuthFailed)
}

func writeLDAPAuthenticationThrottled(w http.ResponseWriter) {
	response := STSErrorResponse{}
	response.Error.Code = "ThrottlingException"
	response.Error.Message = "Too many LDAP authentication attempts. Retry later."
	response.RequestID = w.Header().Get(xhttp.AmzRequestID)
	w.Header().Set("Retry-After", "1")
	writeResponse(w, http.StatusTooManyRequests, encodeResponse(response), mimeXML)
}
