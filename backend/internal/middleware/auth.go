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
			c.Next()
			return
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
			c.Next()
			return
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
			c.Next()
			return
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		c.Abort()
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
