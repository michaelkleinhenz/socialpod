package backup

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const pass = "correct horse battery staple"

// memStore is an in-memory Store for tests.
type memStore map[string][]bson.Raw

func (m memStore) seed(collection string, doc bson.D) {
	raw, err := bson.Marshal(doc)
	if err != nil {
		panic(err)
	}
	m[collection] = append(m[collection], raw)
}

func (m memStore) CollectionNames(context.Context) ([]string, error) {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

func (m memStore) Each(_ context.Context, c string, fn func(bson.Raw) error) error {
	for _, d := range m[c] {
		if err := fn(d); err != nil {
			return err
		}
	}
	return nil
}

func (m memStore) Count(_ context.Context, c string) (int64, error) { return int64(len(m[c])), nil }

func (m memStore) Clear(_ context.Context, c string) error {
	if _, ok := m[c]; ok {
		m[c] = nil
	}
	return nil
}

func (m memStore) Insert(_ context.Context, c string, docs []interface{}) error {
	for _, d := range docs {
		raw, err := bson.Marshal(d)
		if err != nil {
			return err
		}
		m[c] = append(m[c], raw)
	}
	return nil
}

func TestAESZipRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, pass)
	big := bytes.Repeat([]byte("socialpod "), 10000)
	if err := w.WriteFile("a.txt", big, time.Now(), true); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile("b.bin", []byte{1, 2, 3}, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile("empty", nil, time.Now(), true); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.Bytes(), []byte("socialpod socialpod")) {
		t.Fatal("plaintext leaked into archive")
	}

	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), pass)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := r.ReadFile("a.txt"); err != nil || !bytes.Equal(got, big) {
		t.Fatalf("a.txt: %v", err)
	}
	if got, err := r.ReadFile("b.bin"); err != nil || !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Fatalf("b.bin: %v %v", got, err)
	}
	if got, err := r.ReadFile("empty"); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}

	bad, _ := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "wrong passphrase!!")
	if _, err := bad.ReadFile("a.txt"); !errors.Is(err, ErrWrongPassphrase) && !errors.Is(err, ErrCorrupt) {
		t.Fatalf("wrong passphrase: got %v", err)
	}

	// Flip a ciphertext byte: the MAC must catch it.
	tampered := append([]byte(nil), buf.Bytes()...)
	idx := bytes.Index(tampered, []byte("b.bin")) + len("b.bin") + 11 + 16 + 2
	tampered[idx] ^= 0xff
	tr, _ := NewReader(bytes.NewReader(tampered), int64(len(tampered)), pass)
	if _, err := tr.ReadFile("b.bin"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("tampered: got %v", err)
	}
}

func TestCreateRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	src := memStore{}
	uid := primitive.NewObjectID()
	created := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
	src.seed("users", bson.D{{Key: "_id", Value: uid}, {Key: "email", Value: "a@b.c"}, {Key: "password", Value: "$2a$hash"}, {Key: "createdAt", Value: created}})
	src.seed("settings", bson.D{{Key: "openRouterApiKey", Value: "sk-secret"}, {Key: "n", Value: int32(7)}, {Key: "f", Value: 1.5}})
	img := bytes.Repeat([]byte{0x89, 'P', 'N', 'G', 0}, 1000)
	src.seed("uploads", bson.D{{Key: "filename", Value: "a.png"}, {Key: "data", Value: img}})
	for i := 0; i < 1200; i++ {
		src.seed("posts", bson.D{{Key: "n", Value: i}, {Key: "content", Value: "text"}})
	}

	var buf bytes.Buffer
	m, err := Create(ctx, src, &buf, pass, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if m.Collections["posts"] != 1200 || m.Collections["uploads"] != 1 {
		t.Fatalf("manifest: %+v", m)
	}

	// The target already has unrelated data that must be replaced, and a
	// collection the backup doesn't know, which must be emptied.
	dst := memStore{}
	dst.seed("users", bson.D{{Key: "email", Value: "intruder@x"}})
	dst.seed("stray", bson.D{{Key: "x", Value: 1}})
	dst.seed("backup_config", bson.D{{Key: "passphrase", Value: "keep me"}})

	zr, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), pass)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, dst, zr, RestoreOptions{KeepIfPresent: []string{"backup_config"}}); err != nil {
		t.Fatal(err)
	}
	if RestoreInProgress() {
		t.Fatal("restore flag left set")
	}
	if len(dst["users"]) != 1 {
		t.Fatalf("users: %d", len(dst["users"]))
	}
	var u struct {
		ID        primitive.ObjectID `bson:"_id"`
		CreatedAt time.Time          `bson:"createdAt"`
	}
	if err := bson.Unmarshal(dst["users"][0], &u); err != nil || u.ID != uid || !u.CreatedAt.Equal(created) {
		t.Fatalf("user not restored with its ObjectID and date: %+v %v", u, err)
	}
	var s bson.M
	bson.Unmarshal(dst["settings"][0], &s)
	if s["openRouterApiKey"] != "sk-secret" || s["n"] != int32(7) || s["f"] != 1.5 {
		t.Fatalf("settings types not preserved: %#v", s)
	}
	var up struct {
		Data []byte `bson:"data"`
	}
	if err := bson.Unmarshal(dst["uploads"][0], &up); err != nil || !bytes.Equal(up.Data, img) {
		t.Fatalf("upload bytes not restored: %v", err)
	}
	if len(dst["posts"]) != 1200 {
		t.Fatalf("posts: %d", len(dst["posts"]))
	}
	if len(dst["stray"]) != 0 {
		t.Fatalf("stray collection not cleared: %d", len(dst["stray"]))
	}
	var bc bson.M
	bson.Unmarshal(dst["backup_config"][0], &bc)
	if bc["passphrase"] != "keep me" {
		t.Fatalf("backup_config should be kept: %v", bc)
	}

	// Wrong passphrase: nothing may be touched.
	dst2 := memStore{}
	dst2.seed("users", bson.D{{Key: "email", Value: "stay@x"}})
	zr2, _ := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "not the passphrase")
	if _, err := Restore(ctx, dst2, zr2, RestoreOptions{}); err == nil {
		t.Fatal("expected error for wrong passphrase")
	}
	if len(dst2["users"]) != 1 {
		t.Fatal("data modified despite failed validation")
	}
}
