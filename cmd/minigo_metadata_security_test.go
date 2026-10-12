// Copyright 2026 Minigo contributors. Licensed under AGPL-3.0-or-later.
package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio/internal/auth"
	xhttp "github.com/minio/minio/internal/http"
	"github.com/minio/pkg/v3/policy"
)

func TestMinigoUntrustedReplicationMetadata(t *testing.T) {
	h := textproto.MIMEHeader{}
	h.Set(xhttp.MinIOSourceReplicationRequest, "true")
	h.Set(xhttp.AmzBucketReplicationStatus, "REPLICA")
	h.Set("X-Amz-Meta-Application", "retained")
	for key := range replicationToInternalHeaders {
		h.Set(key, "injected")
	}
	metadata, err := extractMetadata(t.Context(), h)
	if err != nil {
		t.Fatal(err)
	}
	for _, internal := range replicationToInternalHeaders {
		if _, ok := metadata[internal]; ok {
			t.Fatalf("untrusted PAX/form metadata injected %s", internal)
		}
	}
	if metadata["X-Amz-Meta-Application"] != "retained" {
		t.Fatal("ordinary user metadata lost")
	}
	// An authorized replication request must retain all encrypted object fields.
	metadata, err = extractMetadataWithReplication(t.Context(), true, h)
	if err != nil {
		t.Fatal(err)
	}
	for _, internal := range replicationToInternalHeaders {
		if metadata[internal] != "injected" {
			t.Fatalf("authorized replication lost %s", internal)
		}
	}
}

func TestMinigoPutObjectRejectsEncryptionMetadataInjection(t *testing.T) {
	ExecObjectLayerAPITest(ExecObjectLayerAPITestArgs{t: t, endpoints: []string{"PutObject"}, objAPITest: func(obj ObjectLayer, instance, bucket string, router http.Handler, root auth.Credentials, t *testing.T) {
		const user = "ordinary-object-writer"
		const secret = "object-writer-regression-secret"
		if _, err := globalIAMSys.CreateUser(t.Context(), user, madmin.AddOrUpdateUserReq{SecretKey: secret, Status: madmin.AccountEnabled}); err != nil {
			t.Fatal(err)
		}
		p, err := policy.ParseConfig(strings.NewReader(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:PutObject"],"Resource":["arn:aws:s3:::*/*"]}]}`))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = globalIAMSys.SetPolicy(t.Context(), "ordinary-writer", *p); err != nil {
			t.Fatal(err)
		}
		if _, err = globalIAMSys.PolicyDBSet(t.Context(), user, "ordinary-writer", regUser, false); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name           string
			marker, inject bool
			status         int
		}{
			{"ordinary-write", false, false, http.StatusOK},
			{"unmarked-injection", false, true, http.StatusForbidden},
			{"forged-replication-marker", true, true, http.StatusForbidden},
		} {
			t.Run(instance+"/"+tc.name, func(t *testing.T) {
				headers := map[string]string{}
				if tc.inject {
					headers["X-Minio-Replication-Server-Side-Encryption-Sealed-Key"] = "fake-sealed-key"
				}
				if tc.marker {
					headers[xhttp.MinIOSourceReplicationRequest] = "true"
				}
				req, err := newTestSignedRequestV4(http.MethodPut, "/"+bucket+"/"+tc.name, 5, bytes.NewReader([]byte("hello")), user, secret, headers)
				if err != nil {
					t.Fatal(err)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, req)
				if response.Code != tc.status {
					t.Fatalf("status %d, wanted %d: %s", response.Code, tc.status, response.Body.String())
				}
				_, err = obj.GetObjectInfo(t.Context(), bucket, tc.name, ObjectOptions{})
				if tc.inject && err == nil {
					t.Fatal("rejected injection still wrote an object")
				}
				if !tc.inject && err != nil {
					t.Fatalf("ordinary write failed: %v", err)
				}
			})
		}
	}})
}
