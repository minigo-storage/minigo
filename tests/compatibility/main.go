// Copyright 2026 Minigo contributors. AGPL-3.0-or-later.
// This is a destructive fixture generator for a NEW isolated test server only.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const dataBucket = "compat-data"
const versionBucket = "compat-versions"
const lockBucket = "compat-lock"
const iamUser = "compat-reader"
const iamPolicy = "compat-read"

type object struct {
	Bucket      string            `json:"bucket"`
	Key         string            `json:"key"`
	Size        int64             `json:"size"`
	SHA256      string            `json:"sha256"`
	VersionID   string            `json:"versionId,omitempty"`
	ETag        string            `json:"etag"`
	ContentType string            `json:"contentType"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}
type entry struct {
	Bucket   string    `json:"bucket"`
	Key      string    `json:"key"`
	Version  string    `json:"version,omitempty"`
	Size     int64     `json:"size"`
	ETag     string    `json:"etag"`
	Modified time.Time `json:"modified"`
	Delete   bool      `json:"deleteMarker"`
	Latest   bool      `json:"latest"`
}
type pending struct {
	ID   string `json:"uploadId"`
	ETag string `json:"partEtag"`
}
type manifest struct {
	Created   time.Time           `json:"created"`
	Objects   []object            `json:"objects"`
	Inventory []entry             `json:"inventory"`
	Pending   *pending            `json:"pendingMultipart,omitempty"`
	Listings  map[string][]string `json:"listings"`
}
type result struct {
	Phase           string    `json:"phase"`
	Passed          bool      `json:"passed"`
	Objects         int       `json:"objects"`
	VerifiedBytes   int64     `json:"verifiedBytes"`
	DurationSeconds float64   `json:"durationSeconds"`
	Error           string    `json:"error,omitempty"`
	Completed       time.Time `json:"completed"`
}

var ctx context.Context
var client *minio.Client
var admin *madmin.AdminClient
var core minio.Core
var endpoint, access, secret string

func main() {
	mode := flag.String("mode", "verify", "seed, verify, write, or crash-write")
	path := flag.String("manifest", "", "fixture manifest path")
	out := flag.String("result", "", "phase result JSON path")
	phase := flag.String("phase", "", "descriptive phase")
	small := flag.Int("small", 10000, "small objects under a shared prefix")
	large := flag.Int64("large-bytes", 5*1024*1024*1024+1024*1024, "multipart object size")
	readyPath := flag.String("ready-file", "", "signal file for in-flight crash injection")
	flag.Parse()
	if *path == "" || *out == "" {
		log.Fatal("manifest and result paths required")
	}
	endpoint = os.Getenv("COMPAT_ENDPOINT")
	access, secret = os.Getenv("MINIO_ROOT_USER"), os.Getenv("MINIO_ROOT_PASSWORD")
	if endpoint == "" || access == "" || secret == "" || os.Getenv("COMPAT_IAM_SECRET") == "" {
		log.Fatal("test environment not set")
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()
	var err error
	client, err = minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Secure: false})
	if err != nil {
		log.Fatal(err)
	}
	core = minio.Core{Client: client}
	admin, err = madmin.New(endpoint, access, secret, false)
	if err != nil {
		log.Fatal(err)
	}
	start := time.Now()
	m := manifest{}
	switch *mode {
	case "seed":
		err = seed(&m, *small, *large)
	case "crash-write":
		err = interruptedWrite(*readyPath)
	case "verify", "write":
		var data []byte
		data, err = os.ReadFile(*path)
		if err == nil {
			err = json.Unmarshal(data, &m)
		}
		if err == nil && *mode == "verify" {
			err = verify(&m)
		}
		if err == nil && *mode == "write" {
			err = writeAfterSwap(&m)
		}
	default:
		err = fmt.Errorf("unknown mode %s", *mode)
	}
	if err == nil && (*mode == "seed" || *mode == "write") {
		err = save(*path, m)
	}
	r := result{Phase: *phase, Passed: err == nil, Objects: len(m.Objects), DurationSeconds: time.Since(start).Seconds(), Completed: time.Now().UTC()}
	if *mode == "verify" && err == nil {
		for _, o := range m.Objects {
			r.VerifiedBytes += o.Size
		}
	}
	if err != nil {
		r.Error = err.Error()
	}
	if saveErr := save(*out, r); saveErr != nil {
		log.Fatal(saveErr)
	}
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%s passed: %d stored object versions in %.1fs", *phase, len(m.Objects), r.DurationSeconds)
}

func save(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(path+".tmp", append(b, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// A repeatable 64KiB pattern makes a >5GiB upload bounded in RAM and disk.
// Every key has a different pattern; SHA256 is computed over the complete stream.
type patternReader struct {
	block     []byte
	offset    int
	remaining int64
}

func pattern(key string, size int64) *patternReader {
	b := make([]byte, 65536)
	for i := 0; i < len(b); i += sha256.Size {
		s := sha256.Sum256([]byte(fmt.Sprintf("minigo-compat:%s:%d", key, i)))
		copy(b[i:], s[:])
	}
	return &patternReader{block: b, remaining: size}
}
func (r *patternReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n := 0
	for n < len(p) {
		k := copy(p[n:], r.block[r.offset:])
		n += k
		r.offset = (r.offset + k) % len(r.block)
	}
	r.remaining -= int64(n)
	return n, nil
}

func put(bucket, key string, size int64, metadata, tags map[string]string) (object, error) {
	h := sha256.New()
	typeName := "application/octet-stream"
	opts := minio.PutObjectOptions{ContentType: typeName, UserMetadata: metadata, UserTags: tags, PartSize: 64 * 1024 * 1024, NumThreads: 2}
	u, err := client.PutObject(ctx, bucket, key, io.TeeReader(pattern(key, size), h), size, opts)
	return object{Bucket: bucket, Key: key, Size: size, SHA256: hex.EncodeToString(h.Sum(nil)), VersionID: u.VersionID, ETag: u.ETag, ContentType: typeName, Metadata: metadata, Tags: tags}, err
}

func seed(m *manifest, small int, large int64) error {
	if small < 1 || large < 64*1024*1024 {
		return errors.New("at least one small object and a 64MiB large object required")
	}
	buckets, err := client.ListBuckets(ctx)
	if err != nil {
		return err
	}
	if len(buckets) != 0 {
		return errors.New("seed refused: server already has buckets; use an isolated empty test volume")
	}
	m.Created = time.Now().UTC()
	for _, b := range []string{dataBucket, versionBucket, lockBucket} {
		if err := client.MakeBucket(ctx, b, minio.MakeBucketOptions{ObjectLocking: b == lockBucket}); err != nil {
			return err
		}
	}
	if err := client.SetBucketVersioning(ctx, versionBucket, minio.BucketVersioningConfiguration{Status: "Enabled"}); err != nil {
		return err
	}
	add := func(bucket, key string, size int64, meta, tags map[string]string) error {
		o, err := put(bucket, key, size, meta, tags)
		if err == nil {
			m.Objects = append(m.Objects, o)
		}
		return err
	}
	log.Printf("seeding %d small objects under one prefix", small)
	var mu sync.Mutex
	err = parallel(small, func(i int) error {
		o, err := put(dataBucket, fmt.Sprintf("many-small/object-%06d.bin", i), int64(i%4097), nil, nil)
		if err == nil {
			mu.Lock()
			m.Objects = append(m.Objects, o)
			mu.Unlock()
		}
		return err
	})
	if err != nil {
		return err
	}
	keys := []string{"empty", "unicode/雪/☃️-café-مرحبا.txt", "special/space + percent% question? hash# ampersand& equals=.txt", "prefix/file", "prefix/file/child", "nested/" + strings.Repeat("a/", 100) + "leaf", "max-key/" + strings.Repeat("b", 250) + "/" + strings.Repeat("c", 250) + "/" + strings.Repeat("d", 250) + "/" + strings.Repeat("e", 131) + "/" + strings.Repeat("f", 131)}
	for i, k := range keys {
		size := int64(100 + i)
		if i == 0 {
			size = 0
		}
		if len(k) > 1024 {
			return fmt.Errorf("test key too long: %d", len(k))
		}
		if err := add(dataBucket, k, size, map[string]string{"test-origin": "official-minio", "compat-case": fmt.Sprint(i)}, map[string]string{"suite": "minigo", "case": fmt.Sprint(i)}); err != nil {
			return fmt.Errorf("key %q (%d bytes): %w", k, len(k), err)
		}
	}
	log.Printf("streaming large object: %d bytes", large)
	if err := add(dataBucket, "large/multipart-boundary.bin", large, map[string]string{"test-origin": "official-minio"}, map[string]string{"suite": "minigo"}); err != nil {
		return err
	}
	for i := 0; i < 3; i++ {
		if err := add(versionBucket, "history.txt", int64(4096+i), map[string]string{"revision": fmt.Sprint(i)}, nil); err != nil {
			return err
		}
	}
	if err := client.RemoveObject(ctx, versionBucket, "history.txt", minio.RemoveObjectOptions{}); err != nil {
		return err
	}
	if err := add(versionBucket, "live.txt", 8192, nil, nil); err != nil {
		return err
	}
	if err := add(lockBucket, "protected.txt", 16384, nil, nil); err != nil {
		return err
	}
	locked := m.Objects[len(m.Objects)-1]
	on := minio.LegalHoldEnabled
	if err := client.PutObjectLegalHold(ctx, lockBucket, locked.Key, minio.PutObjectLegalHoldOptions{VersionID: locked.VersionID, Status: &on}); err != nil {
		return err
	}
	if err := admin.AddUser(ctx, iamUser, os.Getenv("COMPAT_IAM_SECRET")); err != nil {
		return err
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject","s3:GetObjectVersion","s3:ListBucket"],"Resource":["arn:aws:s3:::compat-data","arn:aws:s3:::compat-data/*"]}]}`
	if err := admin.AddCannedPolicy(ctx, iamPolicy, []byte(policy)); err != nil {
		return err
	}
	if _, err := admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{User: iamUser, Policies: []string{iamPolicy}}); err != nil {
		return err
	}
	// A persisted part must be resumable after changing the server image.
	id, err := core.NewMultipartUpload(ctx, dataBucket, "pending/resumed.bin", minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return err
	}
	part, err := core.PutObjectPart(ctx, dataBucket, "pending/resumed.bin", id, 1, pattern("pending-part-one", 8*1024*1024), 8*1024*1024, minio.PutObjectPartOptions{})
	if err != nil {
		return err
	}
	m.Pending = &pending{ID: id, ETag: part.ETag}
	m.Inventory, err = inventory()
	if err != nil {
		return err
	}
	m.Listings, err = currentListings()
	return err
}

func parallel(n int, fn func(int) error) error {
	jobs := make(chan int)
	var wg sync.WaitGroup
	var once sync.Once
	var first error
	for worker := 0; worker < 12; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if err := fn(i); err != nil {
					once.Do(func() { first = err })
				}
			}
		}()
	}
	for i := 0; i < n; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return first
}

func inventory() ([]entry, error) {
	var all []entry
	for _, b := range []string{dataBucket, versionBucket, lockBucket} {
		for o := range client.ListObjects(ctx, b, minio.ListObjectsOptions{Recursive: true, WithVersions: true, MaxKeys: 317}) {
			if o.Err != nil {
				return nil, o.Err
			}
			all = append(all, entry{Bucket: b, Key: o.Key, Version: o.VersionID, Size: o.Size, ETag: o.ETag, Modified: o.LastModified, Delete: o.IsDeleteMarker, Latest: o.IsLatest})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		return a.Bucket+"\x00"+a.Key+"\x00"+a.Version < b.Bucket+"\x00"+b.Key+"\x00"+b.Version
	})
	return all, nil
}

func currentListings() (map[string][]string, error) {
	all := map[string][]string{}
	for _, v1 := range []bool{true, false} {
		api := "v2"
		if v1 {
			api = "v1"
		}
		for o := range client.ListObjects(ctx, dataBucket, minio.ListObjectsOptions{Recursive: true, UseV1: v1, MaxKeys: 173}) {
			if o.Err != nil {
				return nil, o.Err
			}
			all[api] = append(all[api], o.Key)
		}
		sort.Strings(all[api])
	}
	return all, nil
}

func verify(m *manifest) error {
	got, err := inventory()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(got, m.Inventory) {
		return fmt.Errorf("inventory changed: got %d entries, expected %d (keys/versions/ETags/timestamps/delete markers must match exactly)", len(got), len(m.Inventory))
	}
	if _, err := client.StatObject(ctx, dataBucket, "crash/uncommitted.bin", minio.StatObjectOptions{}); minio.ToErrorResponse(err).Code != "NoSuchKey" {
		return fmt.Errorf("unfinished multipart object became visible: %v", err)
	}
	v, err := client.GetBucketVersioning(ctx, versionBucket)
	if err != nil {
		return err
	}
	if !v.Enabled() {
		return errors.New("versioning no longer enabled")
	}
	if err := parallel(len(m.Objects), func(i int) error { return verifyObject(m.Objects[i]) }); err != nil {
		return err
	}
	if _, err := client.StatObject(ctx, versionBucket, "history.txt", minio.StatObjectOptions{}); minio.ToErrorResponse(err).Code != "NoSuchKey" {
		return fmt.Errorf("latest delete marker not respected: %v", err)
	}
	for _, o := range m.Objects {
		if o.Bucket == lockBucket {
			hold, err := client.GetObjectLegalHold(ctx, o.Bucket, o.Key, minio.GetObjectLegalHoldOptions{VersionID: o.VersionID})
			if err != nil {
				return err
			}
			if hold == nil || *hold != minio.LegalHoldEnabled {
				return errors.New("legal hold lost")
			}
			err = client.RemoveObject(ctx, o.Bucket, o.Key, minio.RemoveObjectOptions{VersionID: o.VersionID, GovernanceBypass: true})
			// The cached MinIO baseline returns InvalidRequest for WORM protection.
			if minio.ToErrorResponse(err).Code != "InvalidRequest" {
				return fmt.Errorf("legal hold deletion returned code %q: %v", minio.ToErrorResponse(err).Code, err)
			}
			if _, err := client.StatObject(ctx, o.Bucket, o.Key, minio.StatObjectOptions{VersionID: o.VersionID}); err != nil {
				return fmt.Errorf("legal hold object missing after rejected deletion: %w", err)
			}
		}
	}
	if _, err := admin.ServerInfo(ctx); err != nil {
		return fmt.Errorf("admin server info: %w", err)
	}
	u, err := admin.GetUserInfo(ctx, iamUser)
	if err != nil {
		return err
	}
	if u.Status != madmin.AccountEnabled {
		return errors.New("IAM user disabled")
	}
	if _, err := admin.InfoCannedPolicy(ctx, iamPolicy); err != nil {
		return err
	}
	reader, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(iamUser, os.Getenv("COMPAT_IAM_SECRET"), ""), Secure: false})
	if err != nil {
		return err
	}
	if _, err := reader.StatObject(ctx, dataBucket, "empty", minio.StatObjectOptions{}); err != nil {
		return fmt.Errorf("IAM allowed read failed: %w", err)
	}
	_, err = reader.PutObject(ctx, dataBucket, "must-not-exist", strings.NewReader("denied"), 6, minio.PutObjectOptions{})
	if minio.ToErrorResponse(err).Code != "AccessDenied" {
		return fmt.Errorf("read-only IAM allowed write: %v", err)
	}
	if m.Pending != nil {
		parts, err := core.ListObjectParts(ctx, dataBucket, "pending/resumed.bin", m.Pending.ID, 0, 100)
		if err != nil {
			return err
		}
		if len(parts.ObjectParts) != 1 || strings.Trim(parts.ObjectParts[0].ETag, `"`) != strings.Trim(m.Pending.ETag, `"`) {
			return fmt.Errorf("pending multipart part lost or changed: expected ETag %q, got %+v", m.Pending.ETag, parts.ObjectParts)
		}
	}
	// Preserve actual baseline listing behavior, including its file/prefix
	// collision quirk. Every object is also fetched and hashed independently.
	lists, err := currentListings()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(lists, m.Listings) {
		return errors.New("V1/V2 listings changed from baseline")
	}
	return nil
}

type slowReader struct {
	r        io.Reader
	ready    string
	signaled bool
}

func (r *slowReader) Read(p []byte) (int, error) {
	if !r.signaled {
		if err := os.WriteFile(r.ready, []byte("multipart request body started\n"), 0600); err != nil {
			return 0, err
		}
		r.signaled = true
	}
	time.Sleep(50 * time.Millisecond)
	if len(p) > 65536 {
		p = p[:65536]
	}
	return r.r.Read(p)
}

func interruptedWrite(ready string) error {
	if ready == "" {
		return errors.New("crash-write requires ready-file")
	}
	crashCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	key := "crash/uncommitted.bin"
	id, err := core.NewMultipartUpload(crashCtx, dataBucket, key, minio.PutObjectOptions{})
	if err != nil {
		return err
	}
	if _, err := core.PutObjectPart(crashCtx, dataBucket, key, id, 1, pattern("crash-part-one", 8*1024*1024), 8*1024*1024, minio.PutObjectPartOptions{}); err != nil {
		return err
	}
	reader := &slowReader{r: pattern("crash-part-two", 64*1024*1024), ready: ready}
	_, err = core.PutObjectPart(crashCtx, dataBucket, key, id, 2, reader, 64*1024*1024, minio.PutObjectPartOptions{})
	if !reader.signaled {
		return fmt.Errorf("multipart body did not begin: %v", err)
	}
	if err == nil {
		return errors.New("multipart request finished without the intended interruption")
	}
	log.Printf("in-flight multipart request interrupted as expected: %v", err)
	return nil
}

func verifyObject(o object) error {
	r, err := client.GetObject(ctx, o.Bucket, o.Key, minio.GetObjectOptions{VersionID: o.VersionID})
	if err != nil {
		return err
	}
	defer r.Close()
	s, err := r.Stat()
	if err != nil {
		return fmt.Errorf("stat %q: %w", o.Key, err)
	}
	if s.Size != o.Size || s.ETag != o.ETag || s.ContentType != o.ContentType {
		return fmt.Errorf("size/etag/type changed for %q", o.Key)
	}
	for k, v := range o.Metadata {
		if s.Metadata.Get("X-Amz-Meta-"+k) != v {
			return fmt.Errorf("metadata %s changed for %q", k, o.Key)
		}
	}
	h := sha256.New()
	n, err := io.CopyBuffer(h, r, make([]byte, 256*1024))
	if err != nil {
		return fmt.Errorf("read %q: %w", o.Key, err)
	}
	if n != o.Size || hex.EncodeToString(h.Sum(nil)) != o.SHA256 {
		return fmt.Errorf("SHA256/length mismatch for %q version %s", o.Key, o.VersionID)
	}
	if len(o.Tags) > 0 {
		t, err := client.GetObjectTagging(ctx, o.Bucket, o.Key, minio.GetObjectTaggingOptions{VersionID: o.VersionID})
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(t.ToMap(), o.Tags) {
			return fmt.Errorf("tags changed for %q", o.Key)
		}
	}
	if o.Size > 64*1024*1024 {
		// Cross a multipart part boundary, the 4GiB boundary, and the final byte.
		for _, start := range []int64{0, 64*1024*1024 - 17, 4*1024*1024*1024 - 17, o.Size - 33} {
			if start < 0 || start+32 >= o.Size {
				continue
			}
			opts := minio.GetObjectOptions{VersionID: o.VersionID}
			if err := opts.SetRange(start, start+32); err != nil {
				return err
			}
			rangeReader, err := client.GetObject(ctx, o.Bucket, o.Key, opts)
			if err != nil {
				return err
			}
			b, readErr := io.ReadAll(rangeReader)
			rangeReader.Close()
			if readErr != nil {
				return readErr
			}
			p := pattern(o.Key, 33)
			p.offset = int(start % int64(len(p.block)))
			expected, _ := io.ReadAll(p)
			if !bytes.Equal(b, expected) {
				return fmt.Errorf("range mismatch %q offset %d", o.Key, start)
			}
		}
	}
	return nil
}

func writeAfterSwap(m *manifest) error {
	if m.Pending == nil {
		return errors.New("no pending multipart to resume")
	}
	beforeObjects := len(m.Objects)
	key := "pending/resumed.bin"
	part, err := core.PutObjectPart(ctx, dataBucket, key, m.Pending.ID, 2, pattern("pending-part-two", 1048577), 1048577, minio.PutObjectPartOptions{})
	if err != nil {
		return err
	}
	u, err := core.CompleteMultipartUpload(ctx, dataBucket, key, m.Pending.ID, []minio.CompletePart{{PartNumber: 1, ETag: m.Pending.ETag}, {PartNumber: 2, ETag: part.ETag}}, minio.PutObjectOptions{})
	if err != nil {
		return err
	}
	h := sha256.New()
	io.Copy(h, pattern("pending-part-one", 8*1024*1024))
	io.Copy(h, pattern("pending-part-two", 1048577))
	m.Objects = append(m.Objects, object{Bucket: dataBucket, Key: key, Size: 8*1024*1024 + 1048577, SHA256: hex.EncodeToString(h.Sum(nil)), ETag: u.ETag, ContentType: "application/octet-stream"})
	m.Pending = nil
	for i := 0; i < 32; i++ {
		o, err := put(dataBucket, fmt.Sprintf("after-swap/object-%02d.bin", i), int64(16384+i), map[string]string{"test-origin": "minigo"}, map[string]string{"phase": "after-swap"})
		if err != nil {
			return err
		}
		m.Objects = append(m.Objects, o)
	}
	// A new version of a pre-existing key must preserve its original version.
	o, err := put(versionBucket, "live.txt", 8193, map[string]string{"test-origin": "minigo"}, nil)
	if err != nil {
		return err
	}
	m.Objects = append(m.Objects, o)
	updated, err := inventory()
	if err != nil {
		return err
	}
	if len(updated) != len(m.Inventory)+len(m.Objects)-beforeObjects {
		return errors.New("unexpected inventory count after writes")
	}
	identity := func(e entry) string { return e.Bucket + "\x00" + e.Key + "\x00" + e.Version }
	index := make(map[string]entry, len(updated))
	for _, e := range updated {
		index[identity(e)] = e
	}
	for _, e := range m.Inventory {
		// The old version remains byte-for-byte identical but is no longer latest.
		if e.Bucket == versionBucket && e.Key == "live.txt" {
			e.Latest = false
		}
		if index[identity(e)] != e {
			return fmt.Errorf("existing inventory entry changed during writes: %q version %s", e.Key, e.Version)
		}
	}
	lists, err := currentListings()
	if err != nil {
		return err
	}
	for _, api := range []string{"v1", "v2"} {
		expected := append([]string(nil), m.Listings[api]...)
		for _, o := range m.Objects[beforeObjects:] {
			if o.Bucket == dataBucket {
				expected = append(expected, o.Key)
			}
		}
		sort.Strings(expected)
		if !reflect.DeepEqual(lists[api], expected) {
			return fmt.Errorf("existing %s listing changed during writes", api)
		}
	}
	m.Inventory = updated
	m.Listings = lists
	return nil
}
