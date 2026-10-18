package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UploadToken is a single-use upload URL handed to an MCP client, which
// sends the file there itself (curl) instead of passing it through the
// conversation as base64. Only the token's hash is stored. The file it
// accepts is fixed when it is made: Filename is the stored file's name, so
// its URL is known up front.
type UploadToken struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	TokenHash   string              `bson:"tokenHash" json:"-"`
	UserID      primitive.ObjectID  `bson:"userId" json:"userId"`
	TeamID      *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	Filename    string              `bson:"filename" json:"filename"`
	ContentType string              `bson:"contentType" json:"contentType"`
	// Used is set when an upload claims the token; a failed upload gives it
	// back.
	Used      bool       `bson:"used" json:"used"`
	UsedAt    *time.Time `bson:"usedAt,omitempty" json:"usedAt,omitempty"`
	CreatedAt time.Time  `bson:"createdAt" json:"createdAt"`
	ExpiresAt time.Time  `bson:"expiresAt" json:"expiresAt"`
}
