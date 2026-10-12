package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// OAuthClient is an MCP client (Claude, ChatGPT, ...) registered through
// OAuth 2.0 Dynamic Client Registration (RFC 7591). Clients are public by
// default; one that registers with a secret-based auth method gets a secret,
// stored only as a hash.
type OAuthClient struct {
	ID                      primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	ClientID                string             `bson:"clientId" json:"client_id"`
	ClientSecretHash        string             `bson:"clientSecretHash,omitempty" json:"-"`
	ClientName              string             `bson:"clientName" json:"client_name,omitempty"`
	ClientURI               string             `bson:"clientUri,omitempty" json:"client_uri,omitempty"`
	LogoURI                 string             `bson:"logoUri,omitempty" json:"logo_uri,omitempty"`
	RedirectURIs            []string           `bson:"redirectUris" json:"redirect_uris"`
	TokenEndpointAuthMethod string             `bson:"tokenEndpointAuthMethod" json:"token_endpoint_auth_method"`
	GrantTypes              []string           `bson:"grantTypes" json:"grant_types"`
	CreatedAt               time.Time          `bson:"createdAt" json:"-"`
}

// OAuthAuthCode is a single-use authorization code, bound to the PKCE
// challenge and redirect URI of the request that produced it.
type OAuthAuthCode struct {
	ID                  primitive.ObjectID `bson:"_id,omitempty"`
	CodeHash            string             `bson:"codeHash"`
	ClientID            string             `bson:"clientId"`
	UserID              primitive.ObjectID `bson:"userId"`
	RedirectURI         string             `bson:"redirectUri"`
	CodeChallenge       string             `bson:"codeChallenge"`
	CodeChallengeMethod string             `bson:"codeChallengeMethod"`
	Scope               string             `bson:"scope"`
	Resource            string             `bson:"resource,omitempty"`
	Nonce               string             `bson:"nonce,omitempty"`
	AuthTime            time.Time          `bson:"authTime"`
	ExpiresAt           time.Time          `bson:"expiresAt"`
}

const (
	OAuthTokenAccess  = "access"
	OAuthTokenRefresh = "refresh"
)

// OAuthToken is an issued access or refresh token. Only the SHA-256 of the
// token is stored. OAuth access tokens are accepted by the MCP endpoint only,
// never by the rest of the API, and never carry admin rights.
type OAuthToken struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	TokenHash string             `bson:"tokenHash"`
	Kind      string             `bson:"kind"`
	ClientID  string             `bson:"clientId"`
	UserID    primitive.ObjectID `bson:"userId"`
	Scope     string             `bson:"scope"`
	Resource  string             `bson:"resource,omitempty"`
	CreatedAt time.Time          `bson:"createdAt"`
	ExpiresAt time.Time          `bson:"expiresAt"`
}

// OAuthSigningKey is the RSA key that signs OpenID Connect ID tokens. Its
// public half is published at the JWKS endpoint.
type OAuthSigningKey struct {
	ID            primitive.ObjectID `bson:"_id,omitempty"`
	KID           string             `bson:"kid"`
	PrivateKeyPEM string             `bson:"privateKeyPem"`
	CreatedAt     time.Time          `bson:"createdAt"`
}
