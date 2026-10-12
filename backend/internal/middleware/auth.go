package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Claims struct {
	UserID  string `json:"userId"`
	IsAdmin bool   `json:"isAdmin"`
	jwt.RegisteredClaims
}

func GenerateToken(userID string, isAdmin bool, secret string) (string, error) {
	claims := Claims{
		UserID:  userID,
		IsAdmin: isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(72 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func AuthRequired(secret string, db *database.MongoDB) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
			c.Abort()
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if authenticate(ctx, c, tokenStr, secret, db) {
			c.Next()
			return
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		c.Abort()
	}
}

// authenticate resolves a JWT, user API token or team API token and sets the
// identity keys on the Gin context. OAuth access tokens are not accepted here:
// they are valid for the MCP endpoint only (see MCPAuthRequired).
func authenticate(ctx context.Context, c *gin.Context, tokenStr, secret string, db *database.MongoDB) bool {
	// Try JWT first
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err == nil && token.Valid {
		c.Set("userId", claims.UserID)
		c.Set("isAdmin", claims.IsAdmin)
		c.Set("isTeamAdmin", false)
		// Look up user to get fresh team membership and isTeamAdmin flag
		if uid, uerr := primitive.ObjectIDFromHex(claims.UserID); uerr == nil {
			var u models.User
			if uerr = db.Users().FindOne(ctx, bson.M{"_id": uid}).Decode(&u); uerr == nil {
				if u.TeamID != nil {
					c.Set("teamId", u.TeamID.Hex())
				}
				c.Set("isTeamAdmin", u.IsTeamAdmin)
			}
		}
		return true
	}

	if tokenStr == "" {
		return false
	}

	// Try user API token
	var user models.User
	err = db.Users().FindOne(ctx, bson.M{"apiToken": tokenStr}).Decode(&user)
	if err == nil {
		c.Set("userId", user.ID.Hex())
		c.Set("isAdmin", user.IsAdmin)
		c.Set("isTeamAdmin", user.IsTeamAdmin)
		if user.TeamID != nil {
			c.Set("teamId", user.TeamID.Hex())
		}
		return true
	}

	// Try team API token
	var team models.Team
	err = db.Teams().FindOne(ctx, bson.M{"apiToken": tokenStr}).Decode(&team)
	if err == nil {
		c.Set("teamId", team.ID.Hex())
		c.Set("isAdmin", false)
		c.Set("isTeamAdmin", false)
		// Use a sentinel userId for team-token auth
		c.Set("userId", team.ID.Hex())
		c.Set("isTeamToken", true)
		return true
	}

	return false
}

// MCPAuthRequired authenticates requests to the MCP endpoint. It accepts
// everything AuthRequired does plus OAuth access tokens issued to MCP clients
// such as Claude.ai and ChatGPT connectors. Whatever the credential, the MCP
// server exposes user features only: admin and team-admin flags are always
// cleared, so an admin's session carries no extra rights here.
//
// An unauthenticated request gets a 401 with a WWW-Authenticate header that
// points to the protected resource metadata (RFC 9728), which is how MCP
// clients discover the OAuth authorization server.
func MCPAuthRequired(secret string, db *database.MongoDB, resourceMetadataURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		challenge := fmt.Sprintf(`Bearer resource_metadata="%s"`, resourceMetadataURL)

		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.Header("WWW-Authenticate", challenge)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
			c.Abort()
			return
		}
		tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		ok := false
		if strings.HasPrefix(tokenStr, OAuthAccessTokenPrefix) {
			if user, _, err := LookupOAuthAccessToken(ctx, db, tokenStr); err == nil {
				c.Set("userId", user.ID.Hex())
				if user.TeamID != nil {
					c.Set("teamId", user.TeamID.Hex())
				}
				c.Set("isOAuth", true)
				ok = true
			}
		} else {
			ok = authenticate(ctx, c, tokenStr, secret, db)
		}

		if !ok {
			c.Header("WWW-Authenticate", challenge+`, error="invalid_token"`)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		c.Set("isAdmin", false)
		c.Set("isTeamAdmin", false)
		c.Next()
	}
}

func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin, exists := c.Get("isAdmin")
		if !exists || !isAdmin.(bool) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// TeamAdminFlagRequired allows access to users who have the isTeamAdmin flag set,
// regardless of whether they are currently assigned to a team. Use this for
// endpoints a team admin needs before they have a team (e.g. team self-creation).
func TeamAdminFlagRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		isTeamAdmin, exists := c.Get("isTeamAdmin")
		if exists && isTeamAdmin.(bool) {
			c.Next()
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "Team admin access required"})
		c.Abort()
	}
}

// TeamAdminRequired allows access to users who are team admins or global admins,
// but only if they are assigned to a team (so they have a scope to operate in).
func TeamAdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin, _ := c.Get("isAdmin")
		isTeamAdmin, _ := c.Get("isTeamAdmin")
		_, hasTeam := c.Get("teamId")

		if (isAdmin.(bool) || isTeamAdmin.(bool)) && hasTeam {
			c.Next()
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "Team admin access required"})
		c.Abort()
	}
}
