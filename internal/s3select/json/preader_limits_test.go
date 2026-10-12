// Copyright 2026 Minigo contributors. AGPL-3.0-or-later.
package json

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
)

type countedJSONInput struct {
	io.Reader
	read atomic.Int64
}

func (r *countedJSONInput) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read.Add(int64(n))
	return n, err
}

func TestJSONLinesUnterminatedRecordBoundedRead(t *testing.T) {
	const limit = 1 << 20
	for _, compressed := range []bool{false, true} {
		name := "plain"
		if compressed {
			name = "gzip"
		}
		t.Run(name, func(t *testing.T) {
			payload := `{"value":"` + strings.Repeat("a", 4*limit) + `"}`
			var input io.Reader = strings.NewReader(payload)
			if compressed {
				var encoded bytes.Buffer
				zw := gzip.NewWriter(&encoded)
				if _, err := io.WriteString(zw, payload); err != nil {
					t.Fatal(err)
				}
				if err := zw.Close(); err != nil {
					t.Fatal(err)
				}
				zr, err := gzip.NewReader(&encoded)
				if err != nil {
					t.Fatal(err)
				}
				defer zr.Close()
				input = zr
			}
			counted := &countedJSONInput{Reader: input}
			r := NewPReader(io.NopCloser(counted), &ReaderArgs{})
			defer r.Close()
			_, err := r.Read(nil)
			var apiError interface{ ErrorCode() string }
			if !errors.As(err, &apiError) || apiError.ErrorCode() != "OverMaxRecordSize" {
				t.Errorf("wanted OverMaxRecordSize, got %v", err)
			}
			if n := counted.read.Load(); n > limit+2*jsonSplitSize {
				t.Errorf("read %d decompressed bytes; maximum bounded lookahead is %d", n, limit+2*jsonSplitSize)
			}
		})
	}
}

func TestJSONLinesLimitBoundary(t *testing.T) {
	const limit = 1 << 20
	for _, skip := range []int{0, jsonSplitSize} {
		for _, ending := range []string{"", "\n", "\r\n"} {
			data := `{"v":"` + strings.Repeat("a", limit-len(`{"v":""}`)) + `"}` + ending
			r := PReader{buf: bufio.NewReaderSize(strings.NewReader(data), 2*jsonSplitSize)}
			got, err := r.nextSplit(skip, nil)
			if err != nil && err != io.EOF {
				t.Fatalf("skip=%d ending=%q: %v", skip, ending, err)
			}
			if string(got) != data {
				t.Fatalf("skip=%d ending=%q: valid boundary record changed", skip, ending)
			}
		}
	}
}

func TestJSONLinesManyShortRecordsRemainSupported(t *testing.T) {
	r := NewPReader(io.NopCloser(strings.NewReader(strings.Repeat("{\"value\":1}\n", 30000))), &ReaderArgs{})
	defer r.Close()
	n := 0
	for {
		_, err := r.Read(nil)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 30000 {
		t.Fatalf("read %d records, want 30000", n)
	}
}

func TestJSONLineLimitProtectsStreamingParsers(t *testing.T) {
	for _, ending := range []string{"", "\n", "\r\n"} {
		valid := strings.Repeat("a", 1<<20) + ending
		got, err := io.ReadAll(NewLineLimitReader(strings.NewReader(valid)))
		if err != nil || string(got) != valid {
			t.Fatalf("valid boundary changed: %v", err)
		}
	}
	input := &countedJSONInput{Reader: strings.NewReader(strings.Repeat("a", 4<<20))}
	_, err := io.Copy(io.Discard, NewLineLimitReader(input))
	var apiError interface{ ErrorCode() string }
	if !errors.As(err, &apiError) || apiError.ErrorCode() != "OverMaxRecordSize" {
		t.Fatalf("expected bounded-record error, got %v", err)
	}
	if input.read.Load() > (1<<20)+(32<<10) {
		t.Fatal("streaming parser read unbounded input")
	}
	valid := strings.Repeat("{\"v\":1}\n", 30000)
	got, err := io.ReadAll(NewLineLimitReader(strings.NewReader(valid)))
	if err != nil || string(got) != valid {
		t.Fatalf("short records changed: %v", err)
	}
}
