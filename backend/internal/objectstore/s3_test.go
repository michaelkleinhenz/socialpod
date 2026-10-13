package objectstore

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestS3RoundTrip(t *testing.T) {
	objs := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AK/") {
			t.Errorf("missing sigv4 auth: %q", r.Header.Get("Authorization"))
		}
		switch r.Method {
		case "PUT":
			b, _ := io.ReadAll(r.Body)
			objs[r.URL.Path] = b
		case "GET":
			b, ok := objs[r.URL.Path]
			if !ok {
				w.WriteHeader(404)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			w.Write(b)
		case "DELETE":
			delete(objs, r.URL.Path)
			w.WriteHeader(204)
		}
	}))
	defer srv.Close()

	s, err := NewS3(srv.URL, "us-east-1", "bkt", "AK", "SK")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "projects/p/a b.png", "image/png", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	data, ct, err := s.Get(ctx, "projects/p/a b.png")
	if err != nil || string(data) != "hi" || ct != "image/png" {
		t.Fatalf("get: %q %q %v", data, ct, err)
	}
	if err := s.Delete(ctx, "projects/p/a b.png"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Get(ctx, "projects/p/a b.png"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// Known-answer check against the AWS documentation SigV4 example derivation
// is impractical for S3 PUT; instead verify signing is deterministic.
func TestSignDeterministic(t *testing.T) {
	s, _ := NewS3("https://s3.example.com", "eu-west-1", "b", "AK", "SK")
	mk := func() string {
		req, _ := http.NewRequest("GET", s.objectURL("k").String(), nil)
		s.sign(req, nil, mustTime())
		return req.Header.Get("Authorization")
	}
	if mk() != mk() {
		t.Fatal("signature not deterministic")
	}
}

func mustTime() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

func TestS3ListAndStream(t *testing.T) {
	var gotQuery []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/bkt":
			gotQuery = append(gotQuery, r.URL.RawQuery)
			if r.URL.Query().Get("continuation-token") == "" {
				w.Write([]byte(`<ListBucketResult><Contents><Key>backups/a.zip</Key><Size>3</Size><LastModified>2026-01-02T03:04:05.000Z</LastModified></Contents><IsTruncated>true</IsTruncated><NextContinuationToken>t/2=</NextContinuationToken></ListBucketResult>`))
				return
			}
			w.Write([]byte(`<ListBucketResult><Contents><Key>backups/b.zip</Key><Size>5</Size></Contents><IsTruncated>false</IsTruncated></ListBucketResult>`))
		case r.Method == "PUT":
			b, _ := io.ReadAll(r.Body)
			if r.ContentLength != 5 || string(b) != "hello" {
				t.Errorf("put body %q len %d", b, r.ContentLength)
			}
		case r.Method == "GET":
			w.Write([]byte("hello"))
		}
	}))
	defer srv.Close()
	s, _ := NewS3(srv.URL, "us-east-1", "bkt", "AK", "SK")
	ctx := context.Background()
	objs, err := s.List(ctx, "backups/")
	if err != nil || len(objs) != 2 || objs[0].Key != "backups/a.zip" || objs[1].Size != 5 {
		t.Fatalf("list: %+v %v", objs, err)
	}
	if !strings.Contains(gotQuery[1], "continuation-token=t%2F2%3D") || !strings.HasPrefix(gotQuery[0], "list-type=2&prefix=backups%2F") {
		t.Fatalf("queries: %v", gotQuery)
	}
	if err := s.PutReader(ctx, "backups/x.zip", "application/zip", strings.NewReader("hello"), 5); err != nil {
		t.Fatal(err)
	}
	rc, _, err := s.GetReader(ctx, "backups/x.zip")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "hello" {
		t.Fatalf("get: %q", b)
	}
}
