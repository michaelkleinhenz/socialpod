// Package objectstore stores objects (the full-system backups) in an
// S3-compatible bucket.
//
// It speaks the S3 REST API directly (AWS Signature V4, path-style addressing)
// using only the standard library, so any S3-compatible endpoint works
// (AWS, MinIO, Railway/Tigris buckets, …).
package objectstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNotFound is returned by Get when the object does not exist.
var ErrNotFound = errors.New("objectstore: object not found")

// Store is the basic object API.
type Store interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	Get(ctx context.Context, key string) (data []byte, contentType string, err error)
	Delete(ctx context.Context, key string) error
}

// ── S3 implementation ────────────────────────────────────────────────────────

// S3 is a Store backed by an S3-compatible bucket.
type S3 struct {
	endpoint  *url.URL
	region    string
	bucket    string
	accessKey string
	secretKey string
	client    *http.Client
}

// NewS3FromEnv builds an S3 store from AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY,
// AWS_DEFAULT_REGION, AWS_ENDPOINT_URL and AWS_S3_BUCKET_NAME. It returns
// (nil, nil) when the configuration is absent, so the app can run without a bucket.
func NewS3FromEnv() (*S3, error) {
	bucket := os.Getenv("AWS_S3_BUCKET_NAME")
	access := os.Getenv("AWS_ACCESS_KEY_ID")
	secret := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if bucket == "" || access == "" || secret == "" {
		return nil, nil
	}
	region := os.Getenv("AWS_DEFAULT_REGION")
	if region == "" {
		region = "us-east-1"
	}
	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", region)
	}
	return NewS3(endpoint, region, bucket, access, secret)
}

// NewS3 creates an S3 store for the given endpoint and credentials.
func NewS3(endpoint, region, bucket, accessKey, secretKey string) (*S3, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("objectstore: invalid endpoint %q", endpoint)
	}
	return &S3{
		endpoint: u, region: region, bucket: bucket,
		accessKey: accessKey, secretKey: secretKey,
		client: &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func (s *S3) objectURL(key string) *url.URL {
	u := *s.endpoint
	u.Path = strings.TrimRight(s.endpoint.Path, "/") + "/" + s.bucket + "/" + key
	return &u
}

func (s *S3) do(ctx context.Context, method, key, contentType string, body []byte) (*http.Response, error) {
	u := s.objectURL(key)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	s.sign(req, body, time.Now().UTC())
	return s.client.Do(req)
}

// ObjectInfo describes one object returned by List.
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// PutReader uploads size bytes from r without buffering them in memory. r is
// read twice (once to hash the payload for SigV4, once to send it).
func (s *S3) PutReader(ctx context.Context, key, contentType string, r io.ReadSeeker, size int64) error {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.objectURL(key).String(), io.NopCloser(r))
	if err != nil {
		return err
	}
	req.ContentLength = size
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	s.signHash(req, hex.EncodeToString(h.Sum(nil)), time.Now().UTC())
	resp, err := s.longClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return s3Error("put", resp)
	}
	return nil
}

// GetReader streams an object. The caller must close the returned body.
func (s *S3) GetReader(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.objectURL(key).String(), nil)
	if err != nil {
		return nil, 0, err
	}
	s.sign(req, nil, time.Now().UTC())
	resp, err := s.longClient().Do(req)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, 0, ErrNotFound
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		return nil, 0, s3Error("get", resp)
	}
	return resp.Body, resp.ContentLength, nil
}

// longClient has no overall timeout, for multi-gigabyte transfers; callers
// bound the transfer through the request context.
func (s *S3) longClient() *http.Client {
	c := *s.client
	c.Timeout = 0
	return &c
}

type listResult struct {
	Contents []struct {
		Key          string    `xml:"Key"`
		Size         int64     `xml:"Size"`
		LastModified time.Time `xml:"LastModified"`
	} `xml:"Contents"`
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
}

// List returns all objects whose key starts with prefix (ListObjectsV2).
func (s *S3) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var out []ObjectInfo
	token := ""
	for {
		q := url.Values{}
		q.Set("list-type", "2")
		q.Set("prefix", prefix)
		if token != "" {
			q.Set("continuation-token", token)
		}
		u := *s.endpoint
		u.Path = strings.TrimRight(s.endpoint.Path, "/") + "/" + s.bucket
		u.RawQuery = canonicalQuery(q)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		s.sign(req, nil, time.Now().UTC())
		resp, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode/100 != 2 {
			err := s3Error("list", resp)
			resp.Body.Close()
			return nil, err
		}
		var lr listResult
		err = xml.NewDecoder(resp.Body).Decode(&lr)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("objectstore: s3 list: %w", err)
		}
		for _, c := range lr.Contents {
			out = append(out, ObjectInfo{Key: c.Key, Size: c.Size, LastModified: c.LastModified})
		}
		if !lr.IsTruncated || lr.NextContinuationToken == "" {
			return out, nil
		}
		token = lr.NextContinuationToken
	}
}

// canonicalQuery encodes q per SigV4 rules (sorted keys, strict encoding).
func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		for _, v := range q[k] {
			parts = append(parts, uriEncode(k, false)+"="+uriEncode(v, false))
		}
	}
	return strings.Join(parts, "&")
}

func (s *S3) Put(ctx context.Context, key, contentType string, data []byte) error {
	resp, err := s.do(ctx, http.MethodPut, key, contentType, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return s3Error("put", resp)
	}
	return nil
}

func (s *S3) Get(ctx context.Context, key string) ([]byte, string, error) {
	resp, err := s.do(ctx, http.MethodGet, key, "", nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", ErrNotFound
	}
	if resp.StatusCode/100 != 2 {
		return nil, "", s3Error("get", resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	return data, resp.Header.Get("Content-Type"), err
}

func (s *S3) Delete(ctx context.Context, key string) error {
	resp, err := s.do(ctx, http.MethodDelete, key, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return s3Error("delete", resp)
	}
	return nil
}

type s3ErrorBody struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

func s3Error(op string, resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var e s3ErrorBody
	_ = xml.Unmarshal(b, &e)
	return fmt.Errorf("objectstore: s3 %s failed: %s %s %s", op, resp.Status, e.Code, e.Message)
}

// ── AWS Signature V4 ─────────────────────────────────────────────────────────

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// uriEncode percent-encodes per SigV4 rules (keeping "/" when path is true).
func uriEncode(s string, path bool) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			sb.WriteByte(c)
		case c == '/' && path:
			sb.WriteByte(c)
		default:
			fmt.Fprintf(&sb, "%%%02X", c)
		}
	}
	return sb.String()
}

func (s *S3) sign(req *http.Request, body []byte, now time.Time) {
	s.signHash(req, sha256Hex(body), now)
}

func (s *S3) signHash(req *http.Request, payloadHash string, now time.Time) {
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	host := req.URL.Host
	headers := map[string]string{
		"host":                 host,
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           amzDate,
	}
	if ct := req.Header.Get("Content-Type"); ct != "" {
		headers["content-type"] = ct
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k + ":" + strings.TrimSpace(headers[k]) + "\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonical := strings.Join([]string{
		req.Method,
		uriEncode(req.URL.Path, true),
		req.URL.RawQuery, // already canonical (see canonicalQuery)
		canonHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := date + "/" + s.region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex([]byte(canonical))

	k := hmacSHA256([]byte("AWS4"+s.secretKey), date)
	k = hmacSHA256(k, s.region)
	k = hmacSHA256(k, "s3")
	k = hmacSHA256(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(k, toSign))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.accessKey, scope, signedHeaders, sig))
}

// ── In-memory fake (tests) ───────────────────────────────────────────────────

// Memory is an in-memory Store for tests.
type Memory struct {
	mu      sync.Mutex
	objects map[string]memObj
}

type memObj struct {
	data        []byte
	contentType string
	modified    time.Time
}

func NewMemory() *Memory { return &Memory{objects: map[string]memObj{}} }

func (m *Memory) Put(_ context.Context, key, contentType string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = memObj{append([]byte(nil), data...), contentType, time.Now()}
	return nil
}

func (m *Memory) Get(_ context.Context, key string) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objects[key]
	if !ok {
		return nil, "", ErrNotFound
	}
	return append([]byte(nil), o.data...), o.contentType, nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

func (m *Memory) PutReader(ctx context.Context, key, contentType string, r io.ReadSeeker, _ int64) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return m.Put(ctx, key, contentType, data)
}

func (m *Memory) GetReader(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	data, _, err := m.Get(ctx, key)
	if err != nil {
		return nil, 0, err
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

func (m *Memory) List(_ context.Context, prefix string) ([]ObjectInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []ObjectInfo
	for k, o := range m.objects {
		if strings.HasPrefix(k, prefix) {
			out = append(out, ObjectInfo{Key: k, Size: int64(len(o.data)), LastModified: o.modified})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Keys returns all stored keys (test helper).
func (m *Memory) Keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.objects))
	for k := range m.objects {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
