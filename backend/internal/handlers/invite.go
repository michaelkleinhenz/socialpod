package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/middleware"
	"socialmedia/internal/models"
	"socialmedia/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

type InviteHandler struct {
	DB        *database.MongoDB
	Email     *services.EmailService
	AppURL    string
	JWTSecret string
}

func generateInviteToken() (raw string, hashed string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hashed = hex.EncodeToString(h[:])
	return raw, hashed, nil
}

func hashInviteToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (h *InviteHandler) CreateInvite(c *gin.Context) {
	teamIDStr := c.Param("id")
	teamID, err := primitive.ObjectIDFromHex(teamIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	var input struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	existingCount, _ := h.DB.Users().CountDocuments(ctx, bson.M{"email": input.Email})
	if existingCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "A user with this email already exists"})
		return
	}

	pendingCount, _ := h.DB.TeamInvites().CountDocuments(ctx, bson.M{
		"email":  input.Email,
		"teamId": teamID,
		"status": "pending",
		"expiresAt": bson.M{"$gt": time.Now()},
	})
	if pendingCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "An active invitation already exists for this email"})
		return
	}

	rawToken, tokenHash, err := generateInviteToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate invite token"})
		return
	}

	userID, _ := c.Get("userId")
	inviterID, _ := primitive.ObjectIDFromHex(userID.(string))

	invite := models.TeamInvite{
		TeamID:    teamID,
		Email:     input.Email,
		TokenHash: tokenHash,
		Status:    "pending",
		InvitedBy: inviterID,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	result, err := h.DB.TeamInvites().InsertOne(ctx, invite)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invitation"})
		return
	}
	invite.ID = result.InsertedID.(primitive.ObjectID)

	inviteURL := h.AppURL + "/invite?token=" + rawToken
	if emailErr := h.Email.SendInviteEmail(input.Email, team.Name, inviteURL); emailErr != nil {
		c.JSON(http.StatusCreated, gin.H{
			"invite":       invite,
			"emailError":   emailErr.Error(),
			"inviteUrl":    inviteURL,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"invite": invite})
}

func (h *InviteHandler) TeamCreateInvite(c *gin.Context) {
	teamIDRaw, hasTeam := c.Get("teamId")
	if !hasTeam || teamIDRaw.(string) == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "No team assigned"})
		return
	}

	var input struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	teamID, _ := primitive.ObjectIDFromHex(teamIDRaw.(string))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	existingCount, _ := h.DB.Users().CountDocuments(ctx, bson.M{"email": input.Email})
	if existingCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "A user with this email already exists"})
		return
	}

	pendingCount, _ := h.DB.TeamInvites().CountDocuments(ctx, bson.M{
		"email":  input.Email,
		"teamId": teamID,
		"status": "pending",
		"expiresAt": bson.M{"$gt": time.Now()},
	})
	if pendingCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "An active invitation already exists for this email"})
		return
	}

	rawToken, tokenHash, err := generateInviteToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate invite token"})
		return
	}

	userID, _ := c.Get("userId")
	inviterID, _ := primitive.ObjectIDFromHex(userID.(string))

	invite := models.TeamInvite{
		TeamID:    teamID,
		Email:     input.Email,
		TokenHash: tokenHash,
		Status:    "pending",
		InvitedBy: inviterID,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	result, err := h.DB.TeamInvites().InsertOne(ctx, invite)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invitation"})
		return
	}
	invite.ID = result.InsertedID.(primitive.ObjectID)

	inviteURL := h.AppURL + "/invite?token=" + rawToken
	if emailErr := h.Email.SendInviteEmail(input.Email, team.Name, inviteURL); emailErr != nil {
		c.JSON(http.StatusCreated, gin.H{
			"invite":       invite,
			"emailError":   emailErr.Error(),
			"inviteUrl":    inviteURL,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"invite": invite})
}

func (h *InviteHandler) ListTeamInvites(c *gin.Context) {
	teamIDStr := c.Param("id")
	teamID, err := primitive.ObjectIDFromHex(teamIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := h.DB.TeamInvites().Find(ctx, bson.M{"teamId": teamID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invitations"})
		return
	}
	defer cursor.Close(ctx)

	var invites []models.TeamInvite
	if err := cursor.All(ctx, &invites); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode invitations"})
		return
	}
	if invites == nil {
		invites = []models.TeamInvite{}
	}
	c.JSON(http.StatusOK, invites)
}

func (h *InviteHandler) TeamListInvites(c *gin.Context) {
	teamIDRaw, hasTeam := c.Get("teamId")
	if !hasTeam || teamIDRaw.(string) == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "No team assigned"})
		return
	}
	teamID, _ := primitive.ObjectIDFromHex(teamIDRaw.(string))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := h.DB.TeamInvites().Find(ctx, bson.M{"teamId": teamID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invitations"})
		return
	}
	defer cursor.Close(ctx)

	var invites []models.TeamInvite
	if err := cursor.All(ctx, &invites); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode invitations"})
		return
	}
	if invites == nil {
		invites = []models.TeamInvite{}
	}
	c.JSON(http.StatusOK, invites)
}

func (h *InviteHandler) DeleteInvite(c *gin.Context) {
	inviteIDStr := c.Param("inviteId")
	inviteID, err := primitive.ObjectIDFromHex(inviteIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invite ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := h.DB.TeamInvites().DeleteOne(ctx, bson.M{"_id": inviteID})
	if err != nil || res.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Invitation deleted"})
}

func (h *InviteHandler) TeamDeleteInvite(c *gin.Context) {
	teamIDRaw, hasTeam := c.Get("teamId")
	if !hasTeam || teamIDRaw.(string) == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "No team assigned"})
		return
	}
	teamID, _ := primitive.ObjectIDFromHex(teamIDRaw.(string))

	inviteIDStr := c.Param("inviteId")
	inviteID, err := primitive.ObjectIDFromHex(inviteIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invite ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := h.DB.TeamInvites().DeleteOne(ctx, bson.M{"_id": inviteID, "teamId": teamID})
	if err != nil || res.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Invitation deleted"})
}

func (h *InviteHandler) GetInviteInfo(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token is required"})
		return
	}

	tokenHash := hashInviteToken(token)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var invite models.TeamInvite
	err := h.DB.TeamInvites().FindOne(ctx, bson.M{
		"tokenHash": tokenHash,
		"status":    "pending",
	}).Decode(&invite)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found or already used"})
		return
	}

	if time.Now().After(invite.ExpiresAt) {
		c.JSON(http.StatusGone, gin.H{"error": "This invitation has expired"})
		return
	}

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": invite.TeamID}).Decode(&team); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"email":    invite.Email,
		"teamName": team.Name,
		"expiresAt": invite.ExpiresAt,
	})
}

func (h *InviteHandler) AcceptInvite(c *gin.Context) {
	var input struct {
		Token    string `json:"token" binding:"required"`
		Name     string `json:"name" binding:"required"`
		Password string `json:"password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tokenHash := hashInviteToken(input.Token)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var invite models.TeamInvite
	err := h.DB.TeamInvites().FindOne(ctx, bson.M{
		"tokenHash": tokenHash,
		"status":    "pending",
	}).Decode(&invite)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found or already used"})
		return
	}

	if time.Now().After(invite.ExpiresAt) {
		c.JSON(http.StatusGone, gin.H{"error": "This invitation has expired"})
		return
	}

	existingCount, _ := h.DB.Users().CountDocuments(ctx, bson.M{"email": invite.Email})
	if existingCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "A user with this email already exists"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	user := models.User{
		Email:     invite.Email,
		Password:  string(hash),
		Name:      input.Name,
		IsAdmin:   false,
		TeamID:    &invite.TeamID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	result, err := h.DB.Users().InsertOne(ctx, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}
	user.ID = result.InsertedID.(primitive.ObjectID)

	h.DB.TeamInvites().UpdateOne(ctx, bson.M{"_id": invite.ID}, bson.M{
		"$set": bson.M{"status": "accepted", "updatedAt": time.Now()},
	})

	jwtToken, _ := middleware.GenerateToken(user.ID.Hex(), user.IsAdmin, h.JWTSecret)

	c.JSON(http.StatusCreated, gin.H{
		"token": jwtToken,
		"user":  user,
	})
}
