// Copyright 2026 Minigo contributors. AGPL-3.0-or-later.
package csv

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
)

type countedCSVInput struct {
	io.Reader
	read atomic.Int64
}

func (r *countedCSVInput) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read.Add(int64(n))
	return n, err
}

func TestCSVUnterminatedLineBoundedRead(t *testing.T) {
	const recordLimit = 1 << 20
	for _, header := range []string{"NONE", "USE", "IGNORE"} {
		for _, compressed := range []bool{false, true} {
			name := header + "/plain"
			if compressed {
				name = header + "/gzip"
			}
			t.Run(name, func(t *testing.T) {
				payload := strings.Repeat("a", 4*recordLimit)
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
				counted := &countedCSVInput{Reader: input}
				var args ReaderArgs
				if err := xml.Unmarshal([]byte("<CSV><FileHeaderInfo>"+header+"</FileHeaderInfo></CSV>"), &args); err != nil {
					t.Fatal(err)
				}
				r, err := NewReader(io.NopCloser(counted), &args)
				if r != nil {
					defer r.Close()
				}
				if err == nil {
					_, err = r.Read(nil)
				}
				var apiError interface{ ErrorCode() string }
				if !errors.As(err, &apiError) || apiError.ErrorCode() != "OverMaxRecordSize" {
					t.Errorf("wanted OverMaxRecordSize, got %v", err)
				}
				if n := counted.read.Load(); n > recordLimit+2*csvSplitSize {
					t.Errorf("read %d decompressed bytes; maximum bounded lookahead is %d", n, recordLimit+2*csvSplitSize)
				}
			})
		}
	}
}

func TestCSVLineLimitBoundary(t *testing.T) {
	const recordLimit = 1 << 20
	for _, skip := range []int{0, csvSplitSize} {
		for _, ending := range []string{"", "\n", "\r\n"} {
			data := strings.Repeat("a", recordLimit) + ending
			r := Reader{buf: bufio.NewReaderSize(strings.NewReader(data), 2*csvSplitSize)}
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

func TestCSVManyShortRecordsRemainSupported(t *testing.T) {
	data := strings.Repeat("left,right\n", 30000)
	var args ReaderArgs
	if err := xml.Unmarshal([]byte("<CSV/>"), &args); err != nil {
		t.Fatal(err)
	}
	r, err := NewReader(io.NopCloser(strings.NewReader(data)), &args)
	if err != nil {
		t.Fatal(err)
	}
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
