// Copyright (c) 2026 Minigo contributors
// SPDX-License-Identifier: AGPL-3.0-only

package hash

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"testing"
)

func checksumSHA256(data []byte) (string, string) {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(sum[:])
}

// Sharing digest work must never turn two independent expected values into one
// validation, including a checksum that becomes available only after EOF.
func TestReaderSHA256IndependentValidation(t *testing.T) {
	for _, data := range [][]byte{nil, bytes.Repeat([]byte("payload"), 1<<14)} {
		goodHex, goodBase64 := checksumSHA256(data)
		badHex, badBase64 := checksumSHA256([]byte("different payload"))
		for _, tc := range []struct {
			name     string
			shaHex   string
			checksum string
			trailing bool
			want     string
		}{
			{name: "both valid", shaHex: goodHex, checksum: goodBase64},
			{name: "payload mismatch", shaHex: badHex, checksum: goodBase64, want: "payload"},
			{name: "checksum mismatch", shaHex: goodHex, checksum: badBase64, want: "checksum"},
			{name: "both mismatch", shaHex: badHex, checksum: badBase64, want: "payload"},
			{name: "trailing valid", shaHex: goodHex, checksum: goodBase64, trailing: true},
			{name: "trailing mismatch", shaHex: goodHex, checksum: badBase64, trailing: true, want: "checksum"},
			{name: "trailing malformed", shaHex: goodHex, checksum: "!not-base64!", trailing: true, want: "checksum"},
			{name: "trailing absent", shaHex: goodHex, trailing: true, want: "checksum"},
		} {
			t.Run(tc.name+"/"+goodHex[:8], func(t *testing.T) {
				trailer := make(http.Header)
				src := &checksumTrailerReader{Reader: bytes.NewReader(data), trailer: trailer, value: tc.checksum}
				r, err := NewReader(t.Context(), src, int64(len(data)), "", tc.shaHex, int64(len(data)))
				if err != nil {
					t.Fatal(err)
				}
				req := &http.Request{Header: make(http.Header), Trailer: trailer}
				if tc.trailing {
					req.Header.Set("X-Amz-Trailer", "x-amz-checksum-sha256")
				} else {
					req.Header.Set("X-Amz-Checksum-Sha256", tc.checksum)
				}
				if err := r.AddChecksum(req, false); err != nil {
					t.Fatal(err)
				}
				got, err := io.ReadAll(r)
				assertSHA256ReadError(t, err, tc.want)
				if tc.want == "" && !bytes.Equal(got, data) {
					t.Fatal("reader changed payload")
				}
				if tc.want == "" && r.ContentCRC()["SHA256"] != tc.checksum {
					t.Fatalf("checksum metadata was lost: %v", r.ContentCRC())
				}
			})
		}
	}
}

type checksumTrailerReader struct {
	*bytes.Reader
	trailer http.Header
	value   string
}

func (r *checksumTrailerReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.trailer.Set("X-Amz-Checksum-Sha256", r.value)
	}
	return n, err
}

func assertSHA256ReadError(t *testing.T, err error, want string) {
	t.Helper()
	var payloadErr SHA256Mismatch
	var checksumErr ChecksumMismatch
	switch want {
	case "":
		if err != nil {
			t.Fatal(err)
		}
	case "payload":
		if !errors.As(err, &payloadErr) {
			t.Fatalf("expected SHA256Mismatch, got %v", err)
		}
	case "checksum":
		if !errors.As(err, &checksumErr) {
			t.Fatalf("expected ChecksumMismatch, got %v", err)
		}
	}
}

func TestReaderSHA256MergeValidation(t *testing.T) {
	data := []byte("merge must retain every expected digest")
	goodHex, goodBase64 := checksumSHA256(data)
	badHex, _ := checksumSHA256([]byte("incorrect"))
	for _, tc := range []struct {
		name           string
		inner, outer   string
		beforeChecksum bool
		want           string
	}{
		{name: "add valid payload hash", outer: goodHex},
		{name: "add invalid payload hash", outer: badHex, want: "payload"},
		{name: "retain valid payload hash", inner: goodHex},
		{name: "retain invalid payload hash", inner: badHex, want: "payload"},
		{name: "checksum before valid merge", outer: goodHex, beforeChecksum: true},
		{name: "checksum before invalid merge", outer: badHex, beforeChecksum: true, want: "payload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewReader(t.Context(), bytes.NewReader(data), -1, "", tc.inner, -1)
			if err != nil {
				t.Fatal(err)
			}
			cs := NewChecksumWithType(ChecksumSHA256, goodBase64)
			if tc.beforeChecksum {
				if err := r.AddNonTrailingChecksum(cs, false); err != nil {
					t.Fatal(err)
				}
			}
			r, err = NewReader(t.Context(), r, int64(len(data)), "", tc.outer, int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			if !tc.beforeChecksum {
				if err := r.AddNonTrailingChecksum(cs, false); err != nil {
					t.Fatal(err)
				}
			}
			_, err = io.Copy(io.Discard, r)
			assertSHA256ReadError(t, err, tc.want)
		})
	}
}

// Existing callers can configure a checksum after reading or replace one.
// Such a new digest covers only subsequent bytes and must not reuse a digest
// that already includes a prefix. Server-side hashers remain independently
// owned because they are public and callers can Reset them.
func TestReaderSHA256SeparateStreams(t *testing.T) {
	data := []byte("prefix:suffix")
	goodHex, goodBase64 := checksumSHA256(data)
	_, suffixBase64 := checksumSHA256(data[7:])
	for _, replace := range []bool{false, true} {
		r, err := NewReader(t.Context(), bytes.NewReader(data), int64(len(data)), "", goodHex, int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		if replace {
			if err := r.AddNonTrailingChecksum(NewChecksumWithType(ChecksumSHA256, goodBase64), false); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := io.ReadFull(r, make([]byte, 7)); err != nil {
			t.Fatal(err)
		}
		if err := r.AddNonTrailingChecksum(NewChecksumWithType(ChecksumSHA256, suffixBase64), false); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, r); err != nil {
			t.Fatalf("late checksum must retain its separate input: %v", err)
		}
	}
	for _, typ := range BaseChecksumTypes {
		r, err := NewReader(t.Context(), bytes.NewReader(data), int64(len(data)), "", goodHex, int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		r.AddServerSideChecksumHasher(typ)
		if _, err := io.ReadFull(r, make([]byte, 7)); err != nil {
			t.Fatal(err)
		}
		r.ServerSideHasher.Reset()
		if _, err := io.Copy(io.Discard, r); err != nil {
			t.Fatalf("server-side Reset changed payload validation: %v", err)
		}
		h := typ.Hasher()
		h.Write(data[7:])
		if !bytes.Equal(r.ServerSideChecksumResult.Raw, h.Sum(nil)) {
			t.Fatalf("wrong server-side checksum for %v", typ)
		}
	}
}

func TestReaderSHA256OtherChecksums(t *testing.T) {
	data := []byte("keep CRC, SHA1, ignored-value and server-side semantics")
	goodHex, _ := checksumSHA256(data)
	for _, typ := range BaseChecksumTypes {
		for _, ignore := range []bool{false, true} {
			r, err := NewReader(t.Context(), bytes.NewReader(data), int64(len(data)), "", goodHex, int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			h := typ.Hasher()
			h.Write(data)
			cs := NewChecksumWithType(typ, base64.StdEncoding.EncodeToString(h.Sum(nil)))
			if ignore {
				h.Reset()
				cs = NewChecksumWithType(typ, base64.StdEncoding.EncodeToString(h.Sum(nil)))
			}
			if err := r.AddNonTrailingChecksum(cs, ignore); err != nil {
				t.Fatal(err)
			}
			r.AddServerSideChecksumHasher(ChecksumSHA256)
			if _, err := io.Copy(io.Discard, r); err != nil {
				t.Fatalf("checksum %v, ignore=%t: %v", typ, ignore, err)
			}
			if ignore != (r.ServerSideChecksumResult != nil) {
				t.Fatal("client/server checksum precedence changed")
			}
		}
	}
}
