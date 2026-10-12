// Copyright (c) 2026 Minigo contributors
// SPDX-License-Identifier: AGPL-3.0-only

package hash

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"testing"
)

// BenchmarkReaderSHA256 models the plaintext reader used by signed PutObject
// and UploadPart, with and without the additional S3 SHA256 checksum. The MD5
// variants cover ordinary ETags and callers that explicitly disable MD5.
func BenchmarkReaderSHA256(b *testing.B) {
	for _, size := range []int{64 << 10, 8 << 20} {
		data := bytes.Repeat([]byte("minigo-checksum-benchmark"), size/24+1)[:size]
		digest := sha256.Sum256(data)
		shaHex := hex.EncodeToString(digest[:])
		checksum := NewChecksumWithType(ChecksumSHA256, base64.StdEncoding.EncodeToString(digest[:]))
		for _, disableMD5 := range []bool{false, true} {
			for _, extraChecksum := range []bool{false, true} {
				name := fmt.Sprintf("%dKiB/MD5=%t/Checksum=%t", size>>10, !disableMD5, extraChecksum)
				b.Run(name, func(b *testing.B) {
					buf := make([]byte, 32<<10)
					b.ReportAllocs()
					b.SetBytes(int64(size))
					b.ResetTimer()
					for b.Loop() {
						r, err := NewReaderWithOpts(b.Context(), bytes.NewReader(data), Options{
							Size: int64(size), ActualSize: int64(size), SHA256Hex: shaHex, DisableMD5: disableMD5,
						})
						if err != nil {
							b.Fatal(err)
						}
						if extraChecksum {
							if err := r.AddNonTrailingChecksum(checksum, false); err != nil {
								b.Fatal(err)
							}
						}
						// Hide Discard.ReadFrom so every run uses the same 32 KiB reads.
						if n, err := io.CopyBuffer(struct{ io.Writer }{io.Discard}, r, buf); err != nil || n != int64(size) {
							b.Fatalf("read %d bytes: %v", n, err)
						}
					}
				})
			}
		}
	}
}
