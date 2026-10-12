package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"go.mongodb.org/mongo-driver/bson"
)

// Prefixes of the opaque tokens the OAuth server issues. They keep OAuth
// tokens distinguishable from JWTs and API tokens (sm_/st_) at a glance.
const (
	OAuthAccessTokenPrefix  = "spa_"
	OAuthRefreshTokenPrefix = "spr_"
)

// HashOAuthToken returns the hex SHA-256 under which an OAuth code or token is
// stored; the plaintext is never persisted.
func HashOAuthToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// LookupOAuthAccessToken resolves an unexpired OAuth access token to its user.
// The user is read fresh so team membership changes and deletions take effect
// immediately.
func LookupOAuthAccessToken(ctx context.Context, db *database.MongoDB, token string) (*models.User, *models.OAuthToken, error) {
	var tok models.OAuthToken
	err := db.OAuthTokens().FindOne(ctx, bson.M{
		"tokenHash": HashOAuthToken(token),
		"kind":      models.OAuthTokenAccess,
		"expiresAt": bson.M{"$gt": time.Now()},
	}).Decode(&tok)
	if err != nil {
		return nil, nil, errors.New("invalid token")
	}
	var user models.User
	if err := db.Users().FindOne(ctx, bson.M{"_id": tok.UserID}).Decode(&user); err != nil {
		return nil, nil, errors.New("invalid token")
	}
	return &user, &tok, nil
}
