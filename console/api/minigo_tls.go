// Copyright 2026 Minigo contributors. Licensed under AGPL-3.0-or-later.
package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"strings"
)

// ConfigureLocalBackendTLS connects the embedded console to its own HTTPS
// listener over loopback while authenticating the server's configured identity.
// Public certificates usually have DNS SANs rather than a 127.0.0.1 SAN. Only
// this exact local authority receives a selected ServerName and local trust;
// STS, identity providers and explicitly configured upstreams retain ordinary
// certificate and hostname verification. Call once before starting the console.
func ConfigureLocalBackendTLS(backend string, getCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error)) error {
	endpoint, err := url.Parse(backend)
	if err != nil || endpoint.Scheme != "https" || endpoint.User != nil || getCertificate == nil {
		return errors.New("invalid local console HTTPS backend")
	}
	ip := net.ParseIP(endpoint.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return errors.New("local console TLS identity requires a loopback backend")
	}
	port := endpoint.Port()
	if port == "" {
		port = "443"
	}
	backendAddress := net.JoinHostPort(endpoint.Hostname(), port)
	transport := GlobalTransport.Clone()
	baseTLS := transport.TLSClientConfig.Clone()
	if baseTLS.InsecureSkipVerify {
		return errors.New("local console TLS requires certificate verification")
	}
	tcpDial := transport.DialContext
	if tcpDial == nil {
		tcpDial = (&net.Dialer{}).DialContext
	}
	previousTLSDial := transport.DialTLSContext
	transport.DialTLSContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != backendAddress && previousTLSDial != nil {
			return previousTLSDial(ctx, network, address)
		}
		config := baseTLS.Clone()
		if address == backendAddress {
			// Consult the certificate manager per new connection, rather than
			// pinning startup certificate bytes across certificate hot reloads.
			certificate, err := getCertificate(&tls.ClientHelloInfo{})
			if err != nil {
				return nil, err
			}
			leaf, err := minigoCertificateLeaf(certificate)
			if err != nil {
				return nil, err
			}
			name, err := minigoCertificateIdentity(leaf, endpoint.Hostname())
			if err != nil {
				return nil, err
			}
			// SNI can select a different configured certificate. Trust precisely
			// the certificate the manager currently selects for this identity.
			certificate, err = getCertificate(&tls.ClientHelloInfo{ServerName: name})
			if err != nil {
				return nil, err
			}
			leaf, err = minigoCertificateLeaf(certificate)
			if err != nil {
				return nil, err
			}
			if err := leaf.VerifyHostname(name); err != nil {
				return nil, err
			}
			config.ServerName = name
			if config.RootCAs == nil {
				config.RootCAs = x509.NewCertPool()
			} else {
				config.RootCAs = config.RootCAs.Clone()
			}
			config.RootCAs.AddCert(leaf)
		} else if config.ServerName == "" {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			config.ServerName = host
		}
		connection, err := tcpDial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		secured := tls.Client(connection, config)
		if transport.TLSHandshakeTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, transport.TLSHandshakeTimeout)
			defer cancel()
		}
		if err := secured.HandshakeContext(ctx); err != nil {
			connection.Close()
			return nil, err
		}
		return secured, nil
	}
	GlobalTransport = transport
	return nil
}

func minigoCertificateLeaf(certificate *tls.Certificate) (*x509.Certificate, error) {
	if certificate == nil || len(certificate.Certificate) == 0 {
		return nil, errors.New("local console backend has no configured certificate")
	}
	if certificate.Leaf != nil {
		return certificate.Leaf, nil
	}
	return x509.ParseCertificate(certificate.Certificate[0])
}

func minigoCertificateIdentity(certificate *x509.Certificate, dialHost string) (string, error) {
	if certificate.VerifyHostname(dialHost) == nil {
		return dialHost, nil
	}
	for _, name := range certificate.DNSNames {
		if !strings.Contains(name, "*") && certificate.VerifyHostname(name) == nil {
			return name, nil
		}
	}
	for _, name := range certificate.DNSNames {
		if strings.HasPrefix(name, "*.") {
			name = "minigo-loopback." + strings.TrimPrefix(name, "*.")
			if certificate.VerifyHostname(name) == nil {
				return name, nil
			}
		}
	}
	for _, ip := range certificate.IPAddresses {
		if certificate.VerifyHostname(ip.String()) == nil {
			return ip.String(), nil
		}
	}
	return "", errors.New("local console backend certificate requires a valid DNS or IP subject alternative name")
}
