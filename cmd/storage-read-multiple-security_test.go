// Copyright 2026 Minigo contributors. AGPL-3.0-or-later.
package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadMultipleRejectsMsgpackTraversal(t *testing.T) {
	root := t.TempDir()
	drive := filepath.Join(root, "drive")
	if err := os.MkdirAll(filepath.Join(drive, "bucket"), 0700); err != nil {
		t.Fatal(err)
	}
	secret := []byte("fixture outside configured drive")
	if err := os.WriteFile(filepath.Join(root, "outside.txt"), secret, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(drive, "bucket", "inside.txt"), []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &xlStorage{drivePath: drive}
	cases := []struct {
		name string
		req  ReadMultipleReq
	}{
		{"bucket", ReadMultipleReq{Bucket: "..", Files: []string{"outside.txt"}}},
		{"bucket-nested", ReadMultipleReq{Bucket: "../drive/..", Files: []string{"outside.txt"}}},
		{"prefix", ReadMultipleReq{Bucket: "bucket", Prefix: "../..", Files: []string{"outside.txt"}}},
		{"file", ReadMultipleReq{Bucket: "bucket", Files: []string{"../../outside.txt"}}},
		{"late-file", ReadMultipleReq{Bucket: "bucket", Files: []string{"inside.txt", "../../outside.txt"}}},
		{"empty-bucket-traversal", ReadMultipleReq{Files: []string{"../outside.txt"}}},
		{"metadata", ReadMultipleReq{Bucket: "..", Files: []string{"outside.txt"}, MetadataOnly: true}},
		{"absolute-prefix", ReadMultipleReq{Bucket: "bucket", Prefix: "/etc", Files: []string{"passwd"}}},
		{"backslash-traversal", ReadMultipleReq{Bucket: "bucket", Files: []string{`..\..\outside.txt`}}},
		{"null", ReadMultipleReq{Bucket: "bucket", Files: []string{"inside.txt\x00"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Reproduce the real route's msgpack decode before the storage call.
			body, err := tc.req.MarshalMsg(nil)
			if err != nil {
				t.Fatal(err)
			}
			var decoded ReadMultipleReq
			if _, err := decoded.UnmarshalMsg(body); err != nil {
				t.Fatal(err)
			}
			responses := make(chan ReadMultipleResp, len(tc.req.Files))
			err = s.ReadMultiple(context.Background(), decoded, responses)
			if err != errFileAccessDenied {
				t.Errorf("wanted access denied before disk reads, got %v", err)
			}
			for r := range responses {
				t.Errorf("invalid request emitted response: file=%q data=%q error=%q", r.File, r.Data, r.Error)
			}
		})
	}
}

func TestReadMultiplePreservesRelativePaths(t *testing.T) {
	drive := t.TempDir()
	for _, name := range []string{"bucket/nested/inside.txt", ".minio.sys/multipart/upload/part.1"} {
		p := filepath.Join(drive, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s := &xlStorage{drivePath: drive}
	for _, req := range []ReadMultipleReq{
		{Bucket: "bucket", Prefix: "nested/", Files: []string{"inside.txt"}},
		{Files: []string{"bucket/nested/inside.txt", ".minio.sys/multipart/upload/part.1"}},
		{Bucket: ".minio.sys/multipart", Prefix: "upload", Files: []string{"part.1"}},
	} {
		responses := make(chan ReadMultipleResp, len(req.Files))
		if err := s.ReadMultiple(context.Background(), req, responses); err != nil {
			t.Fatal(err)
		}
		count := 0
		for r := range responses {
			if !r.Exists || r.Error != "" || len(r.Data) == 0 {
				t.Errorf("valid relative path failed: %+v", r)
			}
			count++
		}
		if count != len(req.Files) {
			t.Errorf("got %d responses, want %d", count, len(req.Files))
		}
	}
}
