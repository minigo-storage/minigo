// Copyright 2026 Minigo contributors. Licensed under AGPL-3.0-or-later.
package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/minio/minio/internal/auth"
	xhttp "github.com/minio/minio/internal/http"
	"github.com/minio/mux"
	"github.com/minio/pkg/v3/policy"
)

func minigoAuthTestCredentials(t *testing.T) auth.Credentials {
	t.Helper()
	previous := globalActiveCred
	credential, err := auth.CreateCredentials("minigo-auth-test", "minigo-auth-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	globalActiveCred = credential
	globalAPIConfig.mu.Lock()
	previousRootAccess := globalAPIConfig.rootAccess
	globalAPIConfig.rootAccess = true
	globalAPIConfig.mu.Unlock()
	t.Cleanup(func() {
		globalActiveCred = previous
		globalAPIConfig.mu.Lock()
		globalAPIConfig.rootAccess = previousRootAccess
		globalAPIConfig.mu.Unlock()
	})
	return credential
}

func minigoUnsignedRequest() *http.Request {
	// A complete aws-chunked upload with a CRC32 trailer for "abc".
	body := "3\r\nabc\r\n0\r\nx-amz-checksum-crc32:NSRBwg==\r\n\r\n"
	r := httptest.NewRequest(http.MethodPut, "http://storage.example.test/bucket/object", strings.NewReader(body))
	r.Header.Set(xhttp.AmzContentSha256, unsignedPayloadTrailer)
	r.Header.Set(xhttp.AmzDecodedContentLength, "3")
	r.Header.Set(xhttp.ContentEncoding, "aws-chunked")
	r.Header.Set(awsTrailerHeader, "x-amz-checksum-crc32")
	return r
}

func TestMinigoUnsignedTrailerVerifiesHeaderAndQuerySignatures(t *testing.T) {
	credential := minigoAuthTestCredentials(t)
	for _, mode := range []string{"signed", "presigned", "anonymous", "tampered-header", "tampered-query", "query-credential-only", "mixed-auth"} {
		t.Run(mode, func(t *testing.T) {
			r := minigoUnsignedRequest()
			switch mode {
			case "signed", "tampered-header", "mixed-auth":
				if err := signRequestV4(r, credential.AccessKey, credential.SecretKey); err != nil {
					t.Fatal(err)
				}
				if mode == "tampered-header" {
					r.Header.Set(xhttp.Authorization, r.Header.Get(xhttp.Authorization)+"0")
				}
				if mode == "mixed-auth" {
					q := r.URL.Query()
					q.Set(xhttp.AmzCredential, credential.AccessKey+"/"+getScope(UTCNow(), globalSite.Region()))
					r.URL.RawQuery = q.Encode()
				}
			case "presigned", "tampered-query":
				if err := preSignV4(r, credential.AccessKey, credential.SecretKey, 60); err != nil {
					t.Fatal(err)
				}
				if mode == "tampered-query" {
					q := r.URL.Query()
					q.Set(xhttp.AmzSignature, strings.Repeat("0", 64))
					r.URL.RawQuery = q.Encode()
				}
			case "query-credential-only":
				q := r.URL.Query()
				q.Set(xhttp.AmzCredential, credential.AccessKey+"/"+getScope(UTCNow(), globalSite.Region()))
				r.URL.RawQuery = q.Encode()
			}
			if got := getRequestAuthType(r); got != authTypeStreamingUnsignedTrailer {
				t.Fatalf("unexpected auth type %v", got)
			}
			reader, code := newUnsignedV4ChunkedReader(r, true)
			valid := mode == "signed" || mode == "presigned" || mode == "anonymous"
			if !valid {
				if code == ErrNone || reader != nil {
					t.Fatalf("accepted unauthenticated credentials in %s", mode)
				}
				return
			}
			if code != ErrNone {
				t.Fatalf("rejected compatible %s request: %v", mode, code)
			}
			data, err := io.ReadAll(reader)
			if err != nil || string(data) != "abc" || r.Trailer.Get("x-amz-checksum-crc32") != "NSRBwg==" {
				t.Fatalf("decoded %q, trailer %v, error %v", data, r.Trailer, err)
			}
		})
	}
}

type minigoRejectedWriteLayer struct {
	ObjectLayer
	t *testing.T
}

func (o minigoRejectedWriteLayer) PutObject(context.Context, string, string, *PutObjReader, ObjectOptions) (ObjectInfo, error) {
	o.t.Fatal("unauthenticated upload reached the object store")
	return ObjectInfo{}, nil
}

type minigoTrackedBody struct {
	io.Reader
	read bool
}

func (b *minigoTrackedBody) Read(p []byte) (int, error) { b.read = true; return b.Reader.Read(p) }
func (b *minigoTrackedBody) Close() error               { return nil }

// Reproduce both advisories against real handlers. Credential extraction alone
// authorizes these forged requests as owner, so the signature gate must reject
// them before reading or storing any upload bytes (including Snowball entries).
func TestMinigoUnsignedUploadHandlersRejectImpersonationBeforeReading(t *testing.T) {
	credential := minigoAuthTestCredentials(t)
	api := objectAPIHandlers{ObjectAPI: func() ObjectLayer { return minigoRejectedWriteLayer{t: t} }}
	for _, handler := range []struct {
		name  string
		serve http.HandlerFunc
	}{{"PutObject", api.PutObjectHandler}, {"PutObjectPart", api.PutObjectPartHandler}, {"Snowball", api.PutObjectExtractHandler}} {
		for _, source := range []string{"header", "query"} {
			t.Run(handler.name+"/"+source, func(t *testing.T) {
				r := minigoUnsignedRequest()
				body := &minigoTrackedBody{Reader: bytes.NewBufferString("untrusted archive or object bytes")}
				r.Body = body
				q := r.URL.Query()
				q.Set(xhttp.PartNumber, "1")
				q.Set(xhttp.UploadID, "test-upload")
				if source == "query" {
					q.Set(xhttp.AmzCredential, credential.AccessKey+"/"+getScope(UTCNow(), globalSite.Region()))
				} else {
					r.Header.Set(xhttp.AmzDate, UTCNow().Format(iso8601Format))
					r.Header.Set(xhttp.Authorization, fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=host;x-amz-date, Signature=%s", credential.AccessKey, getScope(UTCNow(), globalSite.Region()), strings.Repeat("0", 64)))
				}
				r.URL.RawQuery = q.Encode()
				r = mux.SetURLVars(r, map[string]string{"bucket": "bucket", "object": "object"})
				if handler.name == "Snowball" {
					r.Header.Set(xhttp.AmzSnowballExtract, "true")
				}
				w := httptest.NewRecorder()
				if code := isPutActionAllowed(newContext(r, w, "test"), getRequestAuthType(r), "bucket", "object", r, policy.PutObjectAction); code != ErrNone {
					t.Fatalf("test credentials did not reproduce permission-only authorization: %v", code)
				}
				handler.serve(w, r)
				if w.Code < 400 || w.Code >= 500 {
					t.Fatalf("forged %s credentials got status %d: %s", source, w.Code, w.Body.String())
				}
				if body.read {
					t.Fatal("read upload bytes before rejecting the signature")
				}
			})
		}
	}
}
