package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SocialAccount struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Platform     Platform           `bson:"platform" json:"platform"`
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
	UpdatedAt             time.Time          `bson:"updatedAt" json:"updatedAt"`
}
