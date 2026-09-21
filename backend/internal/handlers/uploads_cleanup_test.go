package handlers

import (
	"testing"

	"socialmedia/internal/database"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestUploadFilename(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"stored upload", "/api/uploads/1700000000000000000.jpg", "1700000000000000000.jpg"},
		{"external url", "https://example.com/cover.jpg", ""},
		{"other local path", "/api/posts/123", ""},
		{"empty", "", ""},
		{"no filename", "/api/uploads/", ""},
		{"traversal", "/api/uploads/../../etc/passwd", ""},
		{"nested path", "/api/uploads/sub/dir.jpg", ""},
		{"dot", "/api/uploads/.", ""},
		{"dotdot", "/api/uploads/..", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := uploadFilename(tc.url); got != tc.want {
				t.Fatalf("uploadFilename(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

// Every collection/field that can hold an upload URL must be in the reference
// table, otherwise a delete elsewhere frees a file that is still in use.
func TestUploadReferencesCoverOwningCollections(t *testing.T) {
	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	db := &database.MongoDB{Client: client, Database: client.Database("test")}

	got := map[string]bool{}
	for _, ref := range uploadReferences {
		got[ref.collection(db).Name()+"."+ref.field] = true
	}

	want := []string{
		"posts.imageUrls",
		"news_drafts.imageUrls",
		"episode_drafts.imageUrls",
		"convention_queue_items.imageUrl",
		"convention_queue_items.imageUrls",
		"watermarks.url",
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("uploadReferences is missing %s — deleting a document would free files it still uses", w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("uploadReferences has %d entries, expected %d — update this test when a model starts storing image URLs", len(got), len(want))
	}
}
