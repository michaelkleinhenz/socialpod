package handlers

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"socialmedia/internal/database"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// uploadURLPrefix is the path every locally stored upload is served under.
const uploadURLPrefix = "/api/uploads/"

// uploadReference is one place an upload URL can be stored. Deleting a
// document that holds an image only frees the file once no reference is left,
// because the same URL is routinely shared: submitting a draft creates a post
// pointing at the draft's image, and a consumed convention queue item hands
// its image to the post it spawns.
type uploadReference struct {
	collection func(*database.MongoDB) *mongo.Collection
	field      string
}

// uploadReferences lists every collection/field pair that can hold an upload
// URL. Add to it whenever a new model starts storing image URLs, or deleting
// that model's documents will orphan files (or, worse, free files another
// document still needs).
var uploadReferences = []uploadReference{
	{func(db *database.MongoDB) *mongo.Collection { return db.Posts() }, "imageUrls"},
	{func(db *database.MongoDB) *mongo.Collection { return db.NewsDrafts() }, "imageUrls"},
	{func(db *database.MongoDB) *mongo.Collection { return db.EpisodeDrafts() }, "imageUrls"},
	{func(db *database.MongoDB) *mongo.Collection { return db.ConventionQueueItems() }, "imageUrl"},
	{func(db *database.MongoDB) *mongo.Collection { return db.ConventionQueueItems() }, "imageUrls"},
	{func(db *database.MongoDB) *mongo.Collection { return db.Watermarks() }, "url"},
}

// uploadFilename maps a stored image URL back to the file it refers to, or
// returns "" for anything that is not one of our own uploads (an external URL,
// or a path trying to escape the upload directory).
func uploadFilename(url string) string {
	if !strings.HasPrefix(url, uploadURLPrefix) {
		return ""
	}
	name := strings.TrimPrefix(url, uploadURLPrefix)
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		return ""
	}
	return name
}

// cleanupUploads deletes the stored file and the uploads record for every URL
// in urls that nothing references any more, and reports how many files it
// freed. Call it *after* the owning documents are gone, so the reference check
// sees what is left.
//
// Freeing storage is best-effort: failures are logged, never returned to the
// caller, because the delete the user asked for has already succeeded.
func cleanupUploads(db *database.MongoDB, uploadDir string, urls []string) int {
	candidates := make(map[string]string, len(urls)) // url -> filename
	for _, url := range urls {
		if filename := uploadFilename(url); filename != "" {
			candidates[url] = filename
		}
	}
	if len(candidates) == 0 {
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// One query per referencing field rather than one per file, so clearing a
	// long list (the Posted tab's "Remove All") stays a handful of queries.
	for _, ref := range uploadReferences {
		remaining := make([]string, 0, len(candidates))
		for url := range candidates {
			remaining = append(remaining, url)
		}
		if len(remaining) == 0 {
			break
		}
		values, err := ref.collection(db).Distinct(ctx, ref.field, bson.M{ref.field: bson.M{"$in": remaining}})
		if err != nil {
			log.Printf("upload cleanup: reference check on %s failed, keeping files: %v", ref.field, err)
			return 0
		}
		for _, v := range values {
			if url, ok := v.(string); ok {
				delete(candidates, url)
			}
		}
	}

	removed := 0
	filenames := make([]string, 0, len(candidates))
	for _, filename := range candidates {
		filenames = append(filenames, filename)
		if err := os.Remove(filepath.Join(uploadDir, filename)); err != nil && !os.IsNotExist(err) {
			log.Printf("upload cleanup: failed to delete file %s: %v", filename, err)
			continue
		}
		removed++
	}
	if _, err := db.Uploads().DeleteMany(ctx, bson.M{"filename": bson.M{"$in": filenames}}); err != nil {
		log.Printf("upload cleanup: failed to delete %d upload records: %v", len(filenames), err)
	}

	return removed
}

// previousUploadURLs reads the images a document holds before an update that
// replaces its image list, so the ones the update drops can be freed once it
// has run. It returns nothing when the update leaves images alone. The kept
// images need no filtering here: cleanupUploads skips whatever the updated
// document still points at.
func previousUploadURLs(ctx context.Context, coll *mongo.Collection, filter, update bson.M) []string {
	if _, replacing := update["imageUrls"]; !replacing {
		return nil
	}
	return collectUploadURLs(ctx, coll, filter)
}

// deleteOneAndCollectUploadURLs deletes the single document matching filter
// and returns the upload URLs it held, so the caller can free the files it
// owned. It reads every field a document stores images in ("imageUrls", the
// single "imageUrl" of a convention queue item, and a watermark's "url"); a
// collection without one simply yields nothing for it.
//
// An error means nothing matched and nothing was deleted. A document that
// deletes but fails to decode yields no URLs: the delete still stands, only
// its cleanup is lost.
func deleteOneAndCollectUploadURLs(ctx context.Context, coll *mongo.Collection, filter bson.M) ([]string, error) {
	res := coll.FindOneAndDelete(ctx, filter)
	if err := res.Err(); err != nil {
		return nil, err
	}

	var doc struct {
		ImageURL  string   `bson:"imageUrl"`
		ImageURLs []string `bson:"imageUrls"`
		URL       string   `bson:"url"`
	}
	if err := res.Decode(&doc); err != nil {
		log.Printf("upload cleanup: failed to read images off deleted document: %v", err)
		return nil, nil
	}

	urls := doc.ImageURLs
	if doc.ImageURL != "" {
		urls = append(urls, doc.ImageURL)
	}
	if doc.URL != "" {
		urls = append(urls, doc.URL)
	}
	return urls, nil
}

// collectUploadURLs gathers the image URLs of every document matching filter,
// so a bulk delete can free their files afterwards. It reads both fields a
// document may store images in ("imageUrls", plus the single "imageUrl" a
// convention queue item uses); a collection without one simply yields nothing
// for it. Errors only cost cleanup, so they are logged and whatever was read
// is returned.
func collectUploadURLs(ctx context.Context, coll *mongo.Collection, filter bson.M) []string {
	projection := bson.M{"imageUrl": 1, "imageUrls": 1}
	cursor, err := coll.Find(ctx, filter, options.Find().SetProjection(projection))
	if err != nil {
		log.Printf("upload cleanup: failed to list images for bulk delete: %v", err)
		return nil
	}
	defer cursor.Close(ctx)

	var docs []struct {
		ImageURL  string   `bson:"imageUrl"`
		ImageURLs []string `bson:"imageUrls"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		log.Printf("upload cleanup: failed to decode images for bulk delete: %v", err)
		return nil
	}

	var urls []string
	for _, d := range docs {
		if d.ImageURL != "" {
			urls = append(urls, d.ImageURL)
		}
		urls = append(urls, d.ImageURLs...)
	}
	return urls
}
