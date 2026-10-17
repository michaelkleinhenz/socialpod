package uploadstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"socialmedia/internal/objectstore"
)

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{
		"123.jpg": true, "radar-1-01.jpg": true,
		"": false, ".": false, "..": false, "a/b.jpg": false, "../x.jpg": false,
	} {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}

// A read from the bucket is cached on disk, and the cached copy is served
// next, without the bucket. Neither touches the database.
func TestReadFromBucketCachesOnDisk(t *testing.T) {
	ctx := context.Background()
	mem := objectstore.NewMemory()
	Configure(mem)
	defer Configure(nil)
	dir := t.TempDir()

	data := []byte("\x89PNG\r\n\x1a\nrest")
	if err := PutBucket(ctx, "1.png", "", data); err != nil {
		t.Fatal(err)
	}
	if keys := mem.Keys(); len(keys) != 1 || keys[0] != "uploads/1.png" {
		t.Fatalf("bucket keys: %v", keys)
	}

	got, ct, err := Read(ctx, nil, dir, "1.png")
	if err != nil || !bytes.Equal(got, data) || ct != "image/png" {
		t.Fatalf("read from bucket: %q %q %v", got, ct, err)
	}
	if p, ok := CachePath(dir, "1.png"); !ok || filepath.Base(p) != "1.png" {
		t.Fatal("not cached on disk")
	}

	mem.Delete(ctx, Key("1.png")) //nolint:errcheck
	got, _, err = Read(ctx, nil, dir, "1.png")
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("read from cache: %q %v", got, err)
	}

	// No temporary files are left next to the cached one.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("upload dir holds %d entries, want 1", len(entries))
	}
}

func TestListBucket(t *testing.T) {
	ctx := context.Background()
	mem := objectstore.NewMemory()
	Configure(mem)
	defer Configure(nil)
	mem.Put(ctx, "uploads/a.jpg", "image/jpeg", []byte("a"))        //nolint:errcheck
	mem.Put(ctx, "backups/x.zip", "application/zip", []byte("zip")) //nolint:errcheck

	objs, err := ListBucket(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 || objs["a.jpg"].Size != 1 {
		t.Fatalf("ListBucket: %+v", objs)
	}
}

// Videos and sounds never go into a database record: without a bucket they
// are refused before anything is stored.
func TestSaveRefusesMediaWithoutBucket(t *testing.T) {
	Configure(nil)
	for _, ct := range []string{"video/mp4", "video/quicktime", "audio/mpeg"} {
		if err := Save(context.Background(), nil, t.TempDir(), "1.bin", ct, []byte("x")); !errors.Is(err, ErrNeedsBucket) {
			t.Errorf("%s: err = %v, want ErrNeedsBucket", ct, err)
		}
	}
}
