package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Upload records one uploaded file. Its bytes are in the S3 bucket when
// Storage is "s3" (uploadstore); Data holds them only for uploads stored
// without a bucket, or before one was configured.
type Upload struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Filename    string             `bson:"filename" json:"filename"`
	ContentType string             `bson:"contentType" json:"contentType"`
	Data        []byte             `bson:"data,omitempty" json:"-"`
	Storage     string             `bson:"storage,omitempty" json:"-"`
	Size        int64              `bson:"size" json:"size"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
	// Temporary marks working material (a clip or a sound the image editor
	// uploaded for a video) that nothing owns yet. It is freed when the editor
	// lets go of it, or by the hourly cleanup a day after its upload, unless
	// something references it by then; a referenced one loses the mark.
	Temporary bool `bson:"temporary,omitempty" json:"-"`
	// OwnerID is the user who uploaded a temporary file, the only one who
	// may release it.
	OwnerID string `bson:"ownerId,omitempty" json:"-"`
}
