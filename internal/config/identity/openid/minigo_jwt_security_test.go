// Copyright 2026 Minigo contributors. Licensed under AGPL-3.0-or-later.
package openid

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	jwtgo "github.com/golang-jwt/jwt/v4"
	"github.com/minio/minio/internal/arn"
	xnet "github.com/minio/pkg/v3/net"
)

func TestMinigoOIDCSigningKeyRefresh(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{
			"kty": "RSA", "kid": "rotated-key", "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB",
		}}})
	}))
	defer server.Close()
	url, err := xnet.ParseHTTPURL(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	provider := &providerCfg{ClientID: "client-id", ClientSecret: "not-an-id-token-signing-key"}
	provider.JWKS.URL = url
	cfg := Config{
		Enabled:            true,
		pubKeys:            publicKeys{RWMutex: &sync.RWMutex{}, pkMap: map[string]any{}},
		arnProviderCfgsMap: map[arn.ARN]*providerCfg{DummyRoleARN: provider},
		closeRespFn:        func(r io.ReadCloser) { _ = r.Close() },
	}
	claims := jwtgo.MapClaims{"exp": time.Now().Add(time.Hour).Unix(), "aud": provider.ClientID, "sub": "alice"}
	token := jwtgo.NewWithClaims(jwtgo.SigningMethodRS256, claims)
	token.Header["kid"] = "rotated-key"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	if err = cfg.Validate(t.Context(), DummyRoleARN, signed, "", "", map[string]any{}); err != nil {
		t.Fatalf("valid identity-provider token failed after key refresh: %v", err)
	}
	if cfg.pubKeys.get(provider.ClientID) != nil {
		t.Fatal("OAuth client secret was registered as an identity signing key")
	}
	for _, method := range []*jwtgo.SigningMethodHMAC{jwtgo.SigningMethodHS256, jwtgo.SigningMethodHS384, jwtgo.SigningMethodHS512} {
		t.Run(method.Alg(), func(t *testing.T) {
			forged := jwtgo.NewWithClaims(method, claims)
			forged.Header["kid"] = provider.ClientID
			signed, err := forged.SignedString([]byte(provider.ClientSecret))
			if err != nil {
				t.Fatal(err)
			}
			if err = cfg.Validate(t.Context(), DummyRoleARN, signed, "", "", map[string]any{}); err == nil {
				t.Fatal("accepted forged HMAC token after key refresh")
			}
		})
	}
}
