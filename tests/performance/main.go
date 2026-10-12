// Copyright 2026 Minigo contributors. AGPL-3.0-or-later.
// This driver creates data only on the disposable loopback server launched by
// scripts/benchmark-s3.sh. It never accepts an arbitrary production endpoint.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const bucket = "minigo-perf-fixture"

type measurement struct {
	Operation    string  `json:"operation"`
	Concurrency  int     `json:"concurrency"`
	Operations   int     `json:"operations"`
	Bytes        int64   `json:"bytes"`
	Seconds      float64 `json:"seconds"`
	OpsPerSecond float64 `json:"opsPerSecond"`
	MiBPerSecond float64 `json:"MiBPerSecond"`
	P50Millis    float64 `json:"p50Millis"`
	P95Millis    float64 `json:"p95Millis"`
}

// Check the actual outgoing dual-SHA requests. This prevents accidentally
// benchmarking SDK streaming chunk signatures or UNSIGNED-PAYLOAD instead.
type checkedTransport struct {
	http.RoundTripper
	dualRequests atomic.Int64
}

func (t *checkedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodPut && r.Header.Get("X-Amz-Checksum-Sha256") != "" {
		raw, err := hex.DecodeString(r.Header.Get("X-Amz-Content-Sha256"))
		if err != nil || len(raw) != sha256.Size || r.Header.Get("Authorization") == "" {
			return nil, fmt.Errorf("dual-SHA upload was not signed with a whole-payload SHA256")
		}
		t.dualRequests.Add(1)
	}
	return t.RoundTripper.RoundTrip(r)
}

func main() {
	out := flag.String("out", "", "result JSON path")
	smallCount := flag.Int("small-count", 2048, "number of 1 KiB fixture objects")
	flag.Parse()
	endpoint := os.Getenv("PERF_ENDPOINT")
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || os.Getenv("MINIGO_PERF_DISPOSABLE") != "1" {
		log.Fatal("use scripts/benchmark-s3.sh with a disposable loopback container")
	}
	if *out == "" || *smallCount < 1024 {
		log.Fatal("out is required and small-count must be at least 1024")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.MaxIdleConns, baseTransport.MaxIdleConnsPerHost = 64, 64
	transport := &checkedTransport{RoundTripper: baseTransport}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("MINIO_ROOT_USER"), os.Getenv("MINIO_ROOT_PASSWORD"), ""),
		Secure: false, Region: "us-east-1", Transport: transport,
	})
	check(err)
	defer baseTransport.CloseIdleConnections()
	core := minio.Core{Client: client}
	check(client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: "us-east-1"}))
	small, large := payload(1<<10), payload(16<<20)
	smallDigest, largeDigest := sha256.Sum256(small), sha256.Sum256(large)
	put := func(key string, data []byte, digest [32]byte, dual bool) error {
		opts := minio.PutObjectOptions{DisableContentSha256: true}
		if dual {
			opts.UserMetadata = map[string]string{"X-Amz-Checksum-Sha256": base64.StdEncoding.EncodeToString(digest[:])}
		}
		info, err := core.PutObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)), "", hex.EncodeToString(digest[:]), opts)
		if err == nil && (info.Size != int64(len(data)) || (dual && info.ChecksumSHA256 != opts.UserMetadata["X-Amz-Checksum-Sha256"])) {
			return fmt.Errorf("upload result checksum or size mismatch for %s", key)
		}
		return err
	}
	get := func(key string, wantSize int, want [32]byte) error {
		obj, err := client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
		if err != nil {
			return err
		}
		defer obj.Close()
		h := sha256.New()
		n, err := io.Copy(h, obj)
		if err == nil && (n != int64(wantSize) || !bytes.Equal(h.Sum(nil), want[:])) {
			return fmt.Errorf("download digest or size mismatch for %s", key)
		}
		return err
	}
	list := func() error {
		i := 0
		for obj := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: "small/", Recursive: true}) {
			if obj.Err != nil {
				return obj.Err
			}
			if obj.Key != fmt.Sprintf("small/%06d", i) || obj.Size != int64(len(small)) {
				return fmt.Errorf("unexpected listing entry %d: %q size=%d", i, obj.Key, obj.Size)
			}
			i++
		}
		if i != *smallCount {
			return fmt.Errorf("listing count %d, want %d", i, *smallCount)
		}
		return nil
	}
	_, err = measure("seed", 8, *smallCount, 0, func(i int) error {
		return put(fmt.Sprintf("small/%06d", i), small, smallDigest, false)
	})
	check(err)
	check(put("large/source", large, largeDigest, true))
	// Warm the exact paths and verify the entire fixture before measurements.
	_, err = measure("verify-seed", 8, *smallCount, 0, func(i int) error {
		return get(fmt.Sprintf("small/%06d", i), len(small), smallDigest)
	})
	check(err)
	check(list())
	check(list())
	check(get("large/source", len(large), largeDigest))
	var results []measurement
	for _, concurrency := range []int{1, 8} {
		for _, phase := range []struct {
			name  string
			count int
			bytes int64
			run   func(int) error
		}{
			{"small-get-1KiB", 1024, int64(len(small)), func(i int) error {
				return get(fmt.Sprintf("small/%06d", i%*smallCount), len(small), smallDigest)
			}},
			{"list-all-small", 16, 0, func(int) error { return list() }},
			{"large-get-16MiB", 16, int64(len(large)), func(int) error { return get("large/source", len(large), largeDigest) }},
			{"signed-dual-sha-put-16MiB", 16, int64(len(large)), func(i int) error {
				return put(fmt.Sprintf("put/c%d/%06d", concurrency, i), large, largeDigest, true)
			}},
		} {
			result, err := measure(phase.name, concurrency, phase.count, phase.bytes, phase.run)
			check(err)
			results = append(results, result)
			log.Printf("%s c=%d: %.1f ops/s, %.1f MiB/s, p95 %.2fms", phase.name, concurrency, result.OpsPerSecond, result.MiBPerSecond, result.P95Millis)
		}
		// Verify every written object's bytes outside the timed PUT phase.
		_, err = measure("verify-put", concurrency, 16, 0, func(i int) error {
			return get(fmt.Sprintf("put/c%d/%06d", concurrency, i), len(large), largeDigest)
		})
		check(err)
	}
	if transport.dualRequests.Load() < 33 {
		log.Fatal("missing verified dual-SHA wire requests")
	}
	report := struct {
		Passed       bool          `json:"passed"`
		Completed    time.Time     `json:"completed"`
		SmallObjects int           `json:"smallObjects"`
		SmallBytes   int           `json:"smallObjectBytes"`
		LargeBytes   int           `json:"largeObjectBytes"`
		DualRequests int64         `json:"verifiedDualSHARequests"`
		Scope        string        `json:"scope"`
		Results      []measurement `json:"results"`
	}{true, time.Now().UTC(), *smallCount, len(small), len(large), transport.dualRequests.Load(),
		"Single-node warm-cache S3 through Docker loopback; client latency includes SDK, network, and GET SHA256 verification; PUT digest precomputed and all uploaded bytes independently reread outside timing. List operation walks every page.", results}
	b, err := json.MarshalIndent(report, "", "  ")
	check(err)
	check(os.WriteFile(*out, append(b, '\n'), 0600))
}

func payload(size int) []byte {
	p := make([]byte, size)
	for i := 0; i < len(p); i += sha256.Size {
		sum := sha256.Sum256([]byte(fmt.Sprintf("minigo-performance:%d", i)))
		copy(p[i:], sum[:])
	}
	return p
}

func measure(name string, concurrency, operations int, bytesPerOp int64, run func(int) error) (measurement, error) {
	durations := make([]time.Duration, operations)
	var next atomic.Int64
	var firstError error
	var once sync.Once
	var wg sync.WaitGroup
	start := time.Now()
	for range concurrency {
		wg.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= operations {
					return
				}
				start := time.Now()
				err := run(i)
				durations[i] = time.Since(start)
				if err != nil {
					once.Do(func() { firstError = err })
				}
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	return measurement{name, concurrency, operations, int64(operations) * bytesPerOp, elapsed.Seconds(),
		float64(operations) / elapsed.Seconds(), float64(int64(operations)*bytesPerOp) / (1 << 20) / elapsed.Seconds(),
		float64(durations[(operations-1)/2]) / float64(time.Millisecond), float64(durations[(operations*95+99)/100-1]) / float64(time.Millisecond)}, firstError
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
