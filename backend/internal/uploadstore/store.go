// Package uploadstore keeps the bytes of uploaded files (images, videos,
// fonts, template previews).
//
// With a bucket configured (AWS_S3_BUCKET_NAME and credentials, the same
// bucket the backups use) the bytes live in it under uploads/<filename>, and
// the uploads collection keeps only a record of each file: name, content type,
// size, time. Without a bucket the record carries the bytes itself, as every
// record did before the bucket existed; MigrateToBucket moves those out once a
// bucket is configured.
//
// UPLOAD_DIR is a cache in front of either: a file is written there when it is
// stored and again whenever it is read from the bucket or the database, and
// it may vanish at any time (an ephemeral container filesystem).
package uploadstore

import (
	"context"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"
	"socialmedia/internal/objectstore"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Prefix is the key prefix of uploads in the bucket.
const Prefix = "uploads/"

// StorageBucket marks a record whose bytes are in the bucket.
const StorageBucket = "s3"

// ErrNotFound is returned when an upload exists nowhere.
var ErrNotFound = errors.New("upload not found")

// ErrNeedsBucket is returned for a video or a sound stored without a bucket:
// they are too large for a database record (16 MB at most) and would bloat
// the database and every backup.
var ErrNeedsBucket = errors.New("videos and sounds need the S3 bucket (set AWS_S3_BUCKET_NAME and its credentials)")

// Meta is what a record carries besides the file itself.
type Meta struct {
	Temporary bool
	OwnerID   string
}

// isMedia reports whether a content type is a video or a sound, which are
// only ever stored in the bucket.
func isMedia(contentType string) bool {
	return strings.HasPrefix(contentType, "video/") || strings.HasPrefix(contentType, "audio/")
}

// Bucket is the object API the store needs; objectstore.S3 and
// objectstore.Memory implement it.
type Bucket interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, string, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]objectstore.ObjectInfo, error)
}

// bucket is set once at startup, before any request is served.
var bucket Bucket

// Configure sets the bucket uploads are stored in; nil keeps them in the
// database.
func Configure(b Bucket) { bucket = b }

// UsesBucket reports whether uploads go to the bucket.
func UsesBucket() bool { return bucket != nil }

// Key is the bucket key of an upload.
func Key(filename string) string { return Prefix + filename }

// ValidName reports whether filename is a plain file name, safe to join to
// UPLOAD_DIR and to use as a key.
func ValidName(filename string) bool {
	return filename != "" && filename != "." && filename != ".." && filepath.Base(filename) == filename
}

// Save stores an upload under filename: the bytes in the bucket (or in the
// record without one), the record in the uploads collection, and a copy in
// dir. It fails if the bytes could not be stored durably; the disk copy is
// best-effort.
//
// Videos and sounds are refused without a bucket (ErrNeedsBucket).
func Save(ctx context.Context, db *database.MongoDB, dir, filename, contentType string, data []byte) error {
	return SaveWith(ctx, db, dir, filename, contentType, data, Meta{})
}

// SaveWith is Save with the record's meta data.
func SaveWith(ctx context.Context, db *database.MongoDB, dir, filename, contentType string, data []byte, meta Meta) error {
	if !ValidName(filename) {
		return fmt.Errorf("invalid upload name %q", filename)
	}
	if bucket == nil && isMedia(contentType) {
		return ErrNeedsBucket
	}
	rec := models.Upload{
		Filename:    filename,
		ContentType: contentType,
		Size:        int64(len(data)),
		CreatedAt:   time.Now(),
		Temporary:   meta.Temporary,
		OwnerID:     meta.OwnerID,
	}
	if bucket != nil {
		if err := bucket.Put(ctx, Key(filename), contentType, data); err != nil {
			return fmt.Errorf("storing %s in the bucket: %w", filename, err)
		}
		rec.Storage = StorageBucket
	} else {
		rec.Data = data
	}
	if _, err := db.Uploads().InsertOne(ctx, rec); err != nil {
		if bucket != nil {
			bucket.Delete(ctx, Key(filename)) //nolint:errcheck
		}
		return fmt.Errorf("recording %s: %w", filename, err)
	}
	writeCache(dir, filename, data)
	return nil
}

// CachePath returns the path of filename's copy in dir if there is one.
func CachePath(dir, filename string) (string, bool) {
	if dir == "" || !ValidName(filename) {
		return "", false
	}
	p := filepath.Join(dir, filename)
	if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
		return p, true
	}
	return "", false
}

// Read returns an upload's bytes and content type: from dir if cached there,
// else from the bucket, else from the record (uploads stored before the
// bucket, or without one). A read from the bucket or the record is cached in
// dir.
func Read(ctx context.Context, db *database.MongoDB, dir, filename string) ([]byte, string, error) {
	if !ValidName(filename) {
		return nil, "", ErrNotFound
	}
	if p, ok := CachePath(dir, filename); ok {
		if data, err := os.ReadFile(p); err == nil {
			return data, contentTypeOf(filename, data), nil
		}
	}
	if bucket != nil {
		data, ct, err := bucket.Get(ctx, Key(filename))
		if err == nil {
			writeCache(dir, filename, data)
			if ct == "" || ct == "application/octet-stream" {
				ct = contentTypeOf(filename, data)
			}
			return data, ct, nil
		}
		if !errors.Is(err, objectstore.ErrNotFound) {
			log.Printf("[Uploads] reading %s from the bucket: %v", filename, err)
		}
	}
	var rec models.Upload
	if err := db.Uploads().FindOne(ctx, bson.M{"filename": filename}).Decode(&rec); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	if len(rec.Data) == 0 {
		return nil, "", ErrNotFound
	}
	writeCache(dir, filename, rec.Data)
	ct := rec.ContentType
	if ct == "" {
		ct = contentTypeOf(filename, rec.Data)
	}
	return rec.Data, ct, nil
}

// Delete removes uploads everywhere they are kept and returns the names it
// removed. A file the bucket would not delete keeps its record, so a later
// sweep still finds it.
func Delete(ctx context.Context, db *database.MongoDB, dir string, filenames []string) ([]string, []error) {
	var errs []error
	removed := make([]string, 0, len(filenames))
	for _, name := range filenames {
		if !ValidName(name) {
			continue
		}
		if bucket != nil {
			if err := bucket.Delete(ctx, Key(name)); err != nil && !errors.Is(err, objectstore.ErrNotFound) {
				errs = append(errs, fmt.Errorf("deleting %s from the bucket: %w", name, err))
				continue
			}
		}
		if dir != "" {
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("deleting %s from disk: %w", name, err))
			}
		}
		removed = append(removed, name)
	}
	if len(removed) > 0 {
		if _, err := db.Uploads().DeleteMany(ctx, bson.M{"filename": bson.M{"$in": removed}}); err != nil {
			errs = append(errs, fmt.Errorf("deleting %d upload records: %w", len(removed), err))
		}
	}
	return removed, errs
}

// ListBucket returns the uploads in the bucket, keyed by filename; nil
// without a bucket.
func ListBucket(ctx context.Context) (map[string]objectstore.ObjectInfo, error) {
	if bucket == nil {
		return nil, nil
	}
	objs, err := bucket.List(ctx, Prefix)
	if err != nil {
		return nil, err
	}
	out := make(map[string]objectstore.ObjectInfo, len(objs))
	for _, o := range objs {
		name := o.Key[len(Prefix):]
		if ValidName(name) {
			out[name] = o
		}
	}
	return out, nil
}

// PutBucket writes bytes to the bucket under filename without touching the
// record (a restore brings the records back itself).
func PutBucket(ctx context.Context, filename, contentType string, data []byte) error {
	if bucket == nil {
		return errors.New("no bucket configured")
	}
	if !ValidName(filename) {
		return fmt.Errorf("invalid upload name %q", filename)
	}
	if contentType == "" {
		contentType = contentTypeOf(filename, data)
	}
	return bucket.Put(ctx, Key(filename), contentType, data)
}

// GetBucket reads filename's bytes from the bucket.
func GetBucket(ctx context.Context, filename string) ([]byte, string, error) {
	if bucket == nil {
		return nil, "", errors.New("no bucket configured")
	}
	return bucket.Get(ctx, Key(filename))
}

// MigrateToBucket moves the bytes of every record that still carries them
// into the bucket and strips them from the record. It is idempotent and does
// nothing without a bucket; a file that fails stays in its record and is
// tried again on the next run. It returns how many files it moved.
func MigrateToBucket(ctx context.Context, db *database.MongoDB) (int, error) {
	if bucket == nil {
		return 0, nil
	}
	// Records with bytes; the filter on a non-empty binary keeps empty data
	// fields of half-written records out.
	filter := bson.M{"data": bson.M{"$type": "binData"}, "storage": bson.M{"$ne": StorageBucket}}
	cur, err := db.Uploads().Find(ctx, filter, options.Find().SetBatchSize(8).SetNoCursorTimeout(true))
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)
	moved, failed := 0, 0
	for cur.Next(ctx) {
		var rec models.Upload
		if err := cur.Decode(&rec); err != nil {
			failed++
			continue
		}
		if !ValidName(rec.Filename) {
			continue
		}
		update := bson.M{"$unset": bson.M{"data": ""}, "$set": bson.M{"storage": StorageBucket}}
		if len(rec.Data) > 0 {
			ct := rec.ContentType
			if ct == "" {
				ct = contentTypeOf(rec.Filename, rec.Data)
			}
			if err := bucket.Put(ctx, Key(rec.Filename), ct, rec.Data); err != nil {
				failed++
				log.Printf("[Uploads] moving %s to the bucket: %v", rec.Filename, err)
				continue
			}
			update["$set"].(bson.M)["size"] = int64(len(rec.Data))
		}
		if _, err := db.Uploads().UpdateOne(ctx, bson.M{"_id": rec.ID}, update); err != nil {
			failed++
			log.Printf("[Uploads] updating the record of %s: %v", rec.Filename, err)
			continue
		}
		moved++
	}
	if err := cur.Err(); err != nil {
		return moved, err
	}
	if failed > 0 {
		return moved, fmt.Errorf("%d uploads could not be moved", failed)
	}
	return moved, nil
}

func writeCache(dir, filename string, data []byte) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	// Written under a temporary name and renamed, so a concurrent reader
	// never serves half a file.
	tmp, err := os.CreateTemp(dir, ".cache-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), filepath.Join(dir, filename)) != nil {
		os.Remove(tmp.Name()) //nolint:errcheck
	}
}

func contentTypeOf(filename string, data []byte) string {
	if ct := mime.TypeByExtension(filepath.Ext(filename)); ct != "" {
		return ct
	}
	return http.DetectContentType(data)
}
