package handlers

import (
	"testing"
	"time"

	"socialmedia/internal/models"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestRadarQueueEntries(t *testing.T) {
	scheduled := models.Post{ID: primitive.NewObjectID(), Status: models.PostStatusScheduled, ScheduledAt: time.Date(2026, 10, 16, 18, 0, 0, 0, time.UTC)}
	published := models.Post{ID: primitive.NewObjectID(), Status: models.PostStatusPublished}
	gone := primitive.NewObjectID()

	free := models.NewsDraft{ID: primitive.NewObjectID(), NewsTagline: "free"}
	inScheduled := models.NewsDraft{ID: primitive.NewObjectID(), NewsTagline: "scheduled", RadarPostID: &scheduled.ID}
	inPublished := models.NewsDraft{ID: primitive.NewObjectID(), NewsTagline: "published", RadarPostID: &published.ID}
	inGone := models.NewsDraft{ID: primitive.NewObjectID(), NewsTagline: "gone", RadarPostID: &gone}

	got := radarQueueEntries(
		[]models.NewsDraft{free, inScheduled, inPublished, inGone},
		map[primitive.ObjectID]models.Post{scheduled.ID: scheduled, published.ID: published},
	)
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	if got[0].ID != free.ID || got[0].RadarPost != nil {
		t.Errorf("first entry should be the free draft without a post, got %+v", got[0])
	}
	if got[1].ID != inScheduled.ID || got[1].RadarPost == nil {
		t.Fatalf("second entry should be the scheduled draft with its post, got %+v", got[1])
	}
	if rp := got[1].RadarPost; rp.ID != scheduled.ID.Hex() || rp.Status != models.PostStatusScheduled || !rp.ScheduledAt.Equal(scheduled.ScheduledAt) {
		t.Errorf("wrong post ref: %+v", rp)
	}
}
