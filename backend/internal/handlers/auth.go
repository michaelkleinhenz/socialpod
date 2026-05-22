package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/middleware"
	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	DB     *database.MongoDB
	Secret string
}

type RegisterInput struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Name     string `json:"name" binding:"required"`
}

type LoginInput struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var input RegisterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First user is always allowed (becomes admin)
	totalUsers, _ := h.DB.Users().CountDocuments(ctx, bson.M{})
	isFirstUser := totalUsers == 0

	if !isFirstUser {
		// Check if self-registration is allowed
		var settings models.AppSettings
		err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)
		if err == nil && !settings.AllowSelfRegistration {
			c.JSON(http.StatusForbidden, gin.H{"error": "Self-registration is disabled. Contact an administrator."})
			return
		}
	}

	// Check if user exists
	count, _ := h.DB.Users().CountDocuments(ctx, bson.M{"email": input.Email})
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Email already registered"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	user := models.User{
		Email:     input.Email,
		Password:  string(hash),
		Name:      input.Name,
		IsAdmin:   isFirstUser,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	result, err := h.DB.Users().InsertOne(ctx, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	user.ID = result.InsertedID.(primitive.ObjectID)
	token, _ := middleware.GenerateToken(user.ID.Hex(), user.IsAdmin, h.Secret)

	c.JSON(http.StatusCreated, gin.H{
		"token": token,
		"user":  user,
	})
}

func (h *AuthHandler) RegistrationStatus(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	totalUsers, _ := h.DB.Users().CountDocuments(ctx, bson.M{})
	if totalUsers == 0 {
		c.JSON(http.StatusOK, gin.H{"allowed": true, "firstUser": true})
		return
	}

	var settings models.AppSettings
	err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)
	allowed := err != nil || settings.AllowSelfRegistration

	c.JSON(http.StatusOK, gin.H{"allowed": allowed, "firstUser": false})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var input LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := h.DB.Users().FindOne(ctx, bson.M{"email": input.Email}).Decode(&user)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	token, _ := middleware.GenerateToken(user.ID.Hex(), user.IsAdmin, h.Secret)

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  user,
	})
}

func (h *AuthHandler) Me(c *gin.Context) {
	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Team tokens set isTeamToken — return team info instead of a user lookup.
	if isTeam, ok := c.Get("isTeamToken"); ok && isTeam.(bool) {
		var team models.Team
		if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": objID}).Decode(&team); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"id":       team.ID,
			"name":     team.Name,
			"isAdmin":  false,
			"teamId":   team.ID,
			"teamName": team.Name,
		})
		return
	}

	var user models.User
	err := h.DB.Users().FindOne(ctx, bson.M{"_id": objID}).Decode(&user)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Include team name if user belongs to a team
	response := gin.H{
		"id":          user.ID,
		"email":       user.Email,
		"name":        user.Name,
		"isAdmin":     user.IsAdmin,
		"isTeamAdmin": user.IsTeamAdmin,
		"teamId":      user.TeamID,
		"createdAt":   user.CreatedAt,
		"updatedAt":   user.UpdatedAt,
	}
	if user.APIToken != "" {
		response["apiToken"] = user.APIToken
	}
	if user.TeamID != nil {
		var team models.Team
		if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": *user.TeamID}).Decode(&team); err == nil {
			response["teamName"] = team.Name
		}
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) UpdatePassword(c *gin.Context) {
	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	var input struct {
		CurrentPassword string `json:"currentPassword" binding:"required"`
		NewPassword     string `json:"newPassword" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	if err := h.DB.Users().FindOne(ctx, bson.M{"_id": objID}).Decode(&user); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.CurrentPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	h.DB.Users().UpdateOne(ctx, bson.M{"_id": objID}, bson.M{
		"$set": bson.M{"password": string(hash), "updatedAt": time.Now()},
	})

	c.JSON(http.StatusOK, gin.H{"message": "Password updated successfully"})
}

func (h *AuthHandler) GenerateAPIToken(c *gin.Context) {
	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	bytes := make([]byte, 32)
	rand.Read(bytes)
	apiToken := "sm_" + hex.EncodeToString(bytes)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h.DB.Users().UpdateOne(ctx, bson.M{"_id": objID}, bson.M{
		"$set": bson.M{"apiToken": apiToken, "updatedAt": time.Now()},
	})

	c.JSON(http.StatusOK, gin.H{"apiToken": apiToken})
}
