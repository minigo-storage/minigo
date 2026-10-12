package api

import (
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConsoleTransportVerifiesTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "verified")
	}))
	defer server.Close()
	transport := GlobalTransport.Clone()
	defer transport.CloseIdleConnections()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.RootCAs = x509.NewCertPool()
	client := &http.Client{Transport: transport}
	if response, err := client.Get(server.URL); err == nil {
		response.Body.Close()
		t.Fatal("accepted an untrusted backend certificate")
	}
	transport.TLSClientConfig.RootCAs.AddCert(server.Certificate())
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("configured CA should be trusted: %v", err)
	}
	response.Body.Close()
	transport.CloseIdleConnections()
	transport.TLSClientConfig.ServerName = "wrong-host.example"
	if response, err := client.Get(server.URL); err == nil {
		response.Body.Close()
		t.Fatal("accepted a backend certificate for the wrong hostname")
	}
}
