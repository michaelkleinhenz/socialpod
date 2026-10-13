package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// BackupConfig is the singleton document configuring full-system backups
// (collection backup_config). A restore keeps the current one, so restoring
// an old backup never brings back an old passphrase or schedule.
type BackupConfig struct {
	ID primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	// Frequency is one of: off, hourly, 6h, 12h, daily, weekly.
	Frequency string `bson:"frequency" json:"frequency"`
	// Retention is the number of backups kept in the bucket.
	Retention int `bson:"retention" json:"retention"`
	// Passphrase encrypts the backup archives (never serialised).
	Passphrase    string     `bson:"passphrase" json:"-"`
	LastAttemptAt *time.Time `bson:"lastAttemptAt,omitempty" json:"lastAttemptAt,omitempty"`
	LastSuccessAt *time.Time `bson:"lastSuccessAt,omitempty" json:"lastSuccessAt,omitempty"`
	LastError     string     `bson:"lastError,omitempty" json:"lastError,omitempty"`
	LastName      string     `bson:"lastName,omitempty" json:"lastName,omitempty"`
	UpdatedAt     time.Time  `bson:"updatedAt" json:"updatedAt"`
}
