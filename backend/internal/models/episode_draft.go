package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type EpisodeDraft struct {
	ID                primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID            primitive.ObjectID  `bson:"userId" json:"userId"`
	TeamID            *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	EpisodeNumber     string              `bson:"episodeNumber,omitempty" json:"episodeNumber,omitempty"`
	EpisodeTitle      string              `bson:"episodeTitle,omitempty" json:"episodeTitle,omitempty"`
	EpisodeType       string              `bson:"episodeType,omitempty" json:"episodeType,omitempty"`
	Summary           string              `bson:"summary,omitempty" json:"summary,omitempty"`
	EpisodeDate       string              `bson:"episodeDate,omitempty" json:"episodeDate,omitempty"`
	GameNamePublisher string              `bson:"gameNamePublisher,omitempty" json:"gameNamePublisher,omitempty"`
	LinkPublisher     string              `bson:"linkPublisher,omitempty" json:"linkPublisher,omitempty"`
	LinkBGG           string              `bson:"linkBGG,omitempty" json:"linkBGG,omitempty"`
	Rules             string              `bson:"rules,omitempty" json:"rules,omitempty"`
	Scene             string              `bson:"scene,omitempty" json:"scene,omitempty"`
	IntroText         string              `bson:"introText,omitempty" json:"introText,omitempty"`
	ImageURLs         []string            `bson:"imageUrls,omitempty" json:"imageUrls,omitempty"`
	AddSocialPosting  bool                `bson:"addSocialPosting" json:"addSocialPosting"`
	Content           string              `bson:"content,omitempty" json:"content,omitempty"`
	Platforms         []Platform          `bson:"platforms,omitempty" json:"platforms,omitempty"`
	ScheduledAt       string              `bson:"scheduledAt,omitempty" json:"scheduledAt,omitempty"`
	Tags              []string            `bson:"tags,omitempty" json:"tags,omitempty"`
	Status            PostStatus          `bson:"status,omitempty" json:"status,omitempty"`
	FooterIDs         map[string]string   `bson:"footerIds,omitempty" json:"footerIds,omitempty"`
	ContentOverrides  map[string]string   `bson:"contentOverrides,omitempty" json:"contentOverrides,omitempty"`
	AccountIDs        map[string]string   `bson:"accountIds,omitempty" json:"accountIds,omitempty"`
	FirstComment      string              `bson:"firstComment,omitempty" json:"firstComment,omitempty"`
	PostType          PostType            `bson:"postType,omitempty" json:"postType,omitempty"`
	CreatedAt         time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt         time.Time           `bson:"updatedAt" json:"updatedAt"`
}
