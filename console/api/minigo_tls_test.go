// Copyright 2026 Minigo contributors. Licensed under AGPL-3.0-or-later.
package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func minigoTestCertificate(t *testing.T, names []string, ips []net.IP) *tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "legacy-name.example"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: names, IPAddresses: ips, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

func TestMinigoLocalTLSVerifiesDNSIdentityAndFollowsCertificateReload(t *testing.T) {
	oldTransport := GlobalTransport
	t.Cleanup(func() { GlobalTransport.CloseIdleConnections(); GlobalTransport = oldTransport })
	current := atomic.Pointer[tls.Certificate]{}
	current.Store(minigoTestCertificate(t, []string{"objects.example.test"}, nil))
	getCertificate := func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return current.Load(), nil }
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	server.TLS = &tls.Config{GetCertificate: getCertificate, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	// httptest adds its default cert when Certificates is empty; remove it so
	// GetCertificate serves the same rotating identity as the MinIO manager.
	server.TLS.Certificates = nil
	GlobalTransport = oldTransport.Clone()
	GlobalTransport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: x509.NewCertPool()}
	GlobalTransport.TLSClientConfig.RootCAs.AddCert(current.Load().Leaf)
	client := &http.Client{Transport: GlobalTransport}
	if resp, err := client.Get(server.URL); err == nil {
		resp.Body.Close()
		t.Fatal("DNS-only certificate unexpectedly matched loopback without identity configuration")
	}
	if err := ConfigureLocalBackendTLS(server.URL, getCertificate); err != nil {
		t.Fatal(err)
	}
	if GlobalTransport.TLSClientConfig.InsecureSkipVerify || GlobalTransport.TLSClientConfig.ServerName != "" {
		t.Fatal("Changed verification for unrelated authorities")
	}
	client.Transport = GlobalTransport
	for _, names := range [][]string{{"objects.example.test"}, {"rotated.example.test"}, {"*.storage.example.test"}} {
		current.Store(minigoTestCertificate(t, names, nil))
		GlobalTransport.CloseIdleConnections()
		response, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("valid configured identity %v failed on loopback: %v", names, err)
		}
		response.Body.Close()
	}
}

func TestMinigoLocalTLSDoesNotOverrideExternalIdentity(t *testing.T) {
	oldTransport := GlobalTransport
	t.Cleanup(func() { GlobalTransport.CloseIdleConnections(); GlobalTransport = oldTransport })
	local := minigoTestCertificate(t, []string{"objects.example.test"}, nil)
	external := minigoTestCertificate(t, []string{"identity.example.test"}, nil)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "identity") }))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{*external}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	serverURL, _ := url.Parse(server.URL)
	GlobalTransport = oldTransport.Clone()
	GlobalTransport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: x509.NewCertPool()}
	GlobalTransport.TLSClientConfig.RootCAs.AddCert(external.Leaf)
	GlobalTransport.Proxy = nil
	GlobalTransport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, serverURL.Host)
	}
	if err := ConfigureLocalBackendTLS("https://127.0.0.1:9000", func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return local, nil }); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: GlobalTransport}
	response, err := client.Get("https://identity.example.test:" + serverURL.Port())
	if err != nil {
		t.Fatalf("normal external hostname validation failed: %v", err)
	}
	response.Body.Close()
	GlobalTransport.CloseIdleConnections()
	if response, err := client.Get("https://wrong.example.test:" + serverURL.Port()); err == nil {
		response.Body.Close()
		t.Fatal("accepted wrong external hostname")
	}
	if response, err := client.Get("https://objects.example.test:" + serverURL.Port()); err == nil {
		response.Body.Close()
		t.Fatal("local backend identity leaked into external TLS validation")
	}
}

func TestMinigoLocalTLSIdentitySelection(t *testing.T) {
	for _, tc := range []struct {
		names []string
		ips   []net.IP
		want  string
	}{
		{[]string{"a.example.test", "b.example.test"}, nil, "a.example.test"},
		{[]string{"*.example.test"}, nil, "minigo-loopback.example.test"},
		{[]string{"*.example.test", "exact.example.test"}, nil, "exact.example.test"},
		{nil, []net.IP{net.ParseIP("127.0.0.1")}, "127.0.0.1"},
		{nil, []net.IP{net.ParseIP("192.0.2.1")}, "192.0.2.1"},
		{nil, nil, ""},
	} {
		name, err := minigoCertificateIdentity(minigoTestCertificate(t, tc.names, tc.ips).Leaf, "127.0.0.1")
		if tc.want == "" {
			if err == nil {
				t.Fatal("accepted CommonName without SAN")
			}
			continue
		}
		if err != nil || name != tc.want {
			t.Fatalf("identity %q, error %v; want %q", name, err, tc.want)
		}
	}
	for _, endpoint := range []string{"http://127.0.0.1:9000", "https://storage.example.test:9000", "https://user@127.0.0.1:9000"} {
		if err := ConfigureLocalBackendTLS(endpoint, func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return nil, nil }); err == nil {
			t.Fatalf("accepted unsupported endpoint %s", endpoint)
		}
	}
}
