package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SocialAccount struct {
	ID       primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	TeamID   *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	Platform Platform            `bson:"platform" json:"platform"`
	AccountName  string             `bson:"accountName" json:"accountName"`
	DisplayName  string             `bson:"displayName" json:"displayName"`
	AccessToken  string             `bson:"accessToken" json:"-"`
	RefreshToken string             `bson:"refreshToken,omitempty" json:"-"`
	TokenExpiry  time.Time          `bson:"tokenExpiry,omitempty" json:"tokenExpiry,omitempty"`
	// Bluesky-specific
	AppPassword  string `bson:"appPassword,omitempty" json:"-"`
	DID          string `bson:"did,omitempty" json:"did,omitempty"`
	PDSHost      string `bson:"pdsHost,omitempty" json:"pdsHost,omitempty"`
	// Instagram-specific
	IGUserID     string `bson:"igUserId,omitempty" json:"igUserId,omitempty"`
	// Twitter/X-specific
	ConsumerKey       string `bson:"consumerKey,omitempty" json:"-"`
	ConsumerSecret    string `bson:"consumerSecret,omitempty" json:"-"`
	AccessTokenSecret string `bson:"accessTokenSecret,omitempty" json:"-"`
	TwitterUserID     string `bson:"twitterUserId,omitempty" json:"twitterUserId,omitempty"`
	// Mastodon-specific
	MastodonInstance string `bson:"mastodonInstance,omitempty" json:"mastodonInstance,omitempty"`
	// Threads-specific
	ThreadsUserID string `bson:"threadsUserId,omitempty" json:"threadsUserId,omitempty"`
	// LinkedIn-specific
	LinkedInPersonURN string `bson:"linkedinPersonUrn,omitempty" json:"linkedinPersonUrn,omitempty"`
	AvatarURL    string `bson:"avatarUrl,omitempty" json:"avatarUrl,omitempty"`
	IsActive     bool   `bson:"isActive" json:"isActive"`
	CreatedAt    time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time `bson:"updatedAt" json:"updatedAt"`
}

type AppSettings struct {
	ID                    primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	AppURL                string             `bson:"appUrl" json:"appUrl"`
	InstagramAppID        string             `bson:"instagramAppId" json:"instagramAppId"`
	InstagramAppSecret    string             `bson:"instagramAppSecret" json:"-"`
	WebhookVerifyToken    string             `bson:"webhookVerifyToken" json:"webhookVerifyToken"`
	AdobeExpressClientID  string             `bson:"adobeExpressClientId" json:"adobeExpressClientId"`
	AllowSelfRegistration bool               `bson:"allowSelfRegistration" json:"allowSelfRegistration"`
	ImprintHTML           string             `bson:"imprintHtml" json:"imprintHtml"`
	CookieBannerEnabled   bool               `bson:"cookieBannerEnabled" json:"cookieBannerEnabled"`
	CookieBannerText      string             `bson:"cookieBannerText" json:"cookieBannerText"`
	OpenRouterAPIKey      string             `bson:"openRouterApiKey" json:"-"`
	OpenRouterModel       string             `bson:"openRouterModel" json:"openRouterModel"`
	AILanguage            string             `bson:"aiLanguage" json:"aiLanguage"`
	BGGAPIToken           string             `bson:"bggApiToken" json:"-"`
	UpdatedAt             time.Time          `bson:"updatedAt" json:"updatedAt"`
}
