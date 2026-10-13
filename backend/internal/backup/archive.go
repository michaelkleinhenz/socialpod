// Package backup creates and restores full-system backups: every MongoDB
// collection (users, teams, settings incl. API keys and OAuth secrets, social
// accounts, posts, drafts, the uploads collection) and the uploaded files kept
// outside the database (the S3 bucket), bundled into one AES-encrypted ZIP.
// Uploads stored without a bucket carry their bytes in their uploads record.
//
// Archive layout:
//
//	manifest.json                  format version, counts, metadata
//	db/<collection>/<00001>.jsonl  documents as canonical Extended JSON, one per line
//	files/<filename>               uploaded files from outside the database
package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

const (
	// FormatName identifies SocialPod backup archives.
	FormatName = "socialpod-backup"
	// FormatVersion is bumped on incompatible layout changes.
	FormatVersion = 1

	chunkBytes      = 8 << 20 // JSONL bytes per archive entry before a new one starts
	insertBatchSize = 200
)

// Store is the database API backups need. MongoStore implements it.
type Store interface {
	CollectionNames(ctx context.Context) ([]string, error)
	// Each calls fn for every document of the collection.
	Each(ctx context.Context, collection string, fn func(doc bson.Raw) error) error
	Count(ctx context.Context, collection string) (int64, error)
	Clear(ctx context.Context, collection string) error
	Insert(ctx context.Context, collection string, docs []interface{}) error
}

var restoring atomic.Bool

// RestoreInProgress reports whether a restore is replacing the database right
// now. Background loops (publishing, convention queues, agents, growth) skip
// their run while it is, so they never act on half-restored data.
func RestoreInProgress() bool { return restoring.Load() }

// Files is where uploaded files live outside the database. A backup copies
// them in, a restore writes them back.
type Files interface {
	// Each calls fn with every file's name and bytes.
	Each(ctx context.Context, fn func(name string, data []byte) error) error
	Write(ctx context.Context, name string, data []byte) error
}

// Manifest describes the contents of a backup archive.
type Manifest struct {
	Format      string           `json:"format"`
	Version     int              `json:"version"`
	CreatedAt   time.Time        `json:"createdAt"`
	Kind        string           `json:"kind"`
	Collections map[string]int64 `json:"collections"`
	Chunks      map[string]int   `json:"chunks"`
	// Files lists the uploaded files stored under files/.
	Files []string `json:"files,omitempty"`
}

func chunkName(collection string, n int) string {
	return fmt.Sprintf("db/%s/%05d.jsonl", collection, n)
}

func fileEntry(name string) string { return "files/" + name }

func validFileName(n string) bool {
	return n != "" && n != "." && n != ".." && len(n) <= 255 && !strings.ContainsAny(n, "/\\\x00")
}

// Create writes a complete backup of the database, and of files when it is
// not nil, to w.
func Create(ctx context.Context, db Store, w io.Writer, passphrase, kind string, files Files) (*Manifest, error) {
	if passphrase == "" {
		return nil, fmt.Errorf("backup passphrase is not set")
	}
	now := time.Now().UTC()
	m := &Manifest{
		Format: FormatName, Version: FormatVersion, CreatedAt: now, Kind: kind,
		Collections: map[string]int64{}, Chunks: map[string]int{},
	}
	zw := NewWriter(w, passphrase)

	names, err := db.CollectionNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing collections: %w", err)
	}
	sort.Strings(names)
	for _, name := range names {
		if !validCollectionName(name) {
			continue
		}
		if err := dumpCollection(ctx, db, name, zw, now, m); err != nil {
			return nil, fmt.Errorf("dumping %s: %w", name, err)
		}
	}
	if files != nil {
		err := files.Each(ctx, func(name string, data []byte) error {
			if !validFileName(name) {
				return nil
			}
			// Images and videos are compressed already.
			if err := zw.WriteFile(fileEntry(name), data, now, false); err != nil {
				return err
			}
			m.Files = append(m.Files, name)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("copying uploaded files: %w", err)
		}
	}

	mj, _ := json.MarshalIndent(m, "", "  ")
	if err := zw.WriteFile("manifest.json", mj, now, true); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return m, nil
}

func dumpCollection(ctx context.Context, db Store, name string, zw *Writer, now time.Time, m *Manifest) error {
	var buf bytes.Buffer
	var count int64
	chunks := 0
	flush := func() error {
		if buf.Len() == 0 {
			return nil
		}
		chunks++
		if err := zw.WriteFile(chunkName(name, chunks), buf.Bytes(), now, true); err != nil {
			return err
		}
		buf.Reset()
		return nil
	}
	err := db.Each(ctx, name, func(doc bson.Raw) error {
		line, err := bson.MarshalExtJSON(doc, true, false)
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
		count++
		if buf.Len() >= chunkBytes {
			return flush()
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	m.Collections[name] = count
	m.Chunks[name] = chunks
	return nil
}

func validCollectionName(n string) bool {
	if n == "" || strings.HasPrefix(n, "system.") || len(n) > 120 {
		return false
	}
	for _, c := range n {
		if !(c == '_' || c == '-' || c == '.' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// ── Restore ──────────────────────────────────────────────────────────────────

// RestoreOptions configures Restore.
type RestoreOptions struct {
	// KeepIfPresent lists collections whose current contents win: they are
	// only restored from the archive when currently empty.
	KeepIfPresent []string
	// Files receives the archive's uploaded files. An archive holding files
	// is refused without it.
	Files Files
}

// RestoreResult summarises a restore.
type RestoreResult struct {
	Manifest    *Manifest        `json:"manifest"`
	Collections map[string]int64 `json:"collections"`
}

// ReadManifest returns the archive's manifest, verifying the passphrase in
// the process.
func ReadManifest(zr *Reader) (*Manifest, error) {
	data, err := zr.ReadFile("manifest.json")
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, ErrCorrupt
	}
	if m.Format != FormatName {
		return nil, fmt.Errorf("not a SocialPod backup")
	}
	if m.Version > FormatVersion {
		return nil, fmt.Errorf("backup format version %d is newer than this server supports (%d)", m.Version, FormatVersion)
	}
	for name := range m.Collections {
		if !validCollectionName(name) {
			return nil, fmt.Errorf("invalid collection name %q in backup", name)
		}
	}
	for _, name := range m.Files {
		if !validFileName(name) {
			return nil, fmt.Errorf("invalid file name %q in backup", name)
		}
	}
	return &m, nil
}

func parseChunk(data []byte) ([]interface{}, error) {
	var docs []interface{}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var d bson.D
		if err := bson.UnmarshalExtJSON(line, true, &d); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, nil
}

// Validate fully decrypts and parses the archive without touching the
// database. It returns the manifest when everything checks out.
func Validate(zr *Reader) (*Manifest, error) {
	m, err := ReadManifest(zr)
	if err != nil {
		return nil, err
	}
	for name, want := range m.Collections {
		var got int64
		for i := 1; i <= m.Chunks[name]; i++ {
			data, err := zr.ReadFile(chunkName(name, i))
			if err != nil {
				return nil, err
			}
			docs, err := parseChunk(data)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", chunkName(name, i), err)
			}
			got += int64(len(docs))
		}
		if got != want {
			return nil, fmt.Errorf("collection %s: expected %d documents, found %d", name, want, got)
		}
	}
	for _, name := range m.Files {
		if !zr.Has(fileEntry(name)) {
			return nil, fmt.Errorf("file %s is listed but missing", name)
		}
	}
	return m, nil
}

// Restore replaces the entire database contents with the archive. The archive
// is validated completely before anything is modified. Collections that exist
// in the database but not in the backup are emptied, so the result mirrors the
// backed-up state.
func Restore(ctx context.Context, db Store, zr *Reader, opts RestoreOptions) (*RestoreResult, error) {
	m, err := Validate(zr)
	if err != nil {
		return nil, err
	}
	if len(m.Files) > 0 && opts.Files == nil {
		return nil, fmt.Errorf("this backup holds %d uploaded files kept in S3; configure the bucket to restore it", len(m.Files))
	}
	restoring.Store(true)
	defer restoring.Store(false)

	// The files first: writing them changes nothing the current state uses
	// (upload names are unique), so a failure here leaves the database as it
	// was.
	for _, name := range m.Files {
		data, err := zr.ReadFile(fileEntry(name))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fileEntry(name), err)
		}
		if err := opts.Files.Write(ctx, name, data); err != nil {
			return nil, fmt.Errorf("restoring file %s: %w", name, err)
		}
	}

	keep := map[string]bool{}
	for _, n := range opts.KeepIfPresent {
		if c, err := db.Count(ctx, n); err == nil && c > 0 {
			keep[n] = true
		}
	}
	res := &RestoreResult{Manifest: m, Collections: map[string]int64{}}

	existing, err := db.CollectionNames(ctx)
	if err != nil {
		return nil, err
	}
	for _, name := range existing {
		if _, inBackup := m.Collections[name]; inBackup || keep[name] || !validCollectionName(name) {
			continue
		}
		if err := db.Clear(ctx, name); err != nil {
			return nil, fmt.Errorf("clearing %s: %w", name, err)
		}
	}

	names := make([]string, 0, len(m.Collections))
	for n := range m.Collections {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		if keep[name] {
			continue
		}
		if err := db.Clear(ctx, name); err != nil {
			return nil, fmt.Errorf("clearing %s: %w", name, err)
		}
		var n int64
		for i := 1; i <= m.Chunks[name]; i++ {
			data, err := zr.ReadFile(chunkName(name, i))
			if err != nil {
				return nil, err
			}
			docs, err := parseChunk(data)
			if err != nil {
				return nil, err
			}
			for start := 0; start < len(docs); start += insertBatchSize {
				end := min(start+insertBatchSize, len(docs))
				if err := db.Insert(ctx, name, docs[start:end]); err != nil {
					return nil, fmt.Errorf("restoring %s: %w", name, err)
				}
			}
			n += int64(len(docs))
		}
		res.Collections[name] = n
	}
	return res, nil
}
