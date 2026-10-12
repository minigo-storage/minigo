// Copyright 2026 Minigo contributors. Licensed under AGPL-3.0-or-later.
package json

import "io"

// NewLineLimitReader bounds each decompressed JSON physical line before a
// third-party streaming parser can accumulate it. It preserves the input bytes.
func NewLineLimitReader(r io.Reader) io.Reader { return &lineLimitReader{source: r} }

type lineLimitReader struct {
	source io.Reader
	length int
	err    error
}

func (r *lineLimitReader) Read(p []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	if len(p) > 32<<10 {
		p = p[:32<<10]
	}
	n, err := r.source.Read(p)
	for i, b := range p[:n] {
		if b == '\n' {
			r.length = 0
		} else {
			r.length++
			if r.length > maxJSONLineSize+1 || (r.length == maxJSONLineSize+1 && b != '\r') {
				r.err = errJSONRecordTooLarge()
				return i, r.err
			}
		}
	}
	r.err = err
	return n, err
}
