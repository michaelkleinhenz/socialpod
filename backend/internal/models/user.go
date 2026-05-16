package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	ID        primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Email     string              `bson:"email" json:"email"`
	Password  string              `bson:"password" json:"-"`
	Name      string              `bson:"name" json:"name"`
	IsAdmin     bool                `bson:"isAdmin" json:"isAdmin"`
	IsTeamAdmin bool                `bson:"isTeamAdmin" json:"isTeamAdmin"`
	TeamID      *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	APIToken  string              `bson:"apiToken,omitempty" json:"apiToken,omitempty"`
	CreatedAt time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time           `bson:"updatedAt" json:"updatedAt"`
}
