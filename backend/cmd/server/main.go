package main

import (
	"log"
	"net/http"

	"socialmedia/internal/config"
	"socialmedia/internal/database"
	"socialmedia/internal/handlers"
	"socialmedia/internal/middleware"
	"socialmedia/internal/services"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.MongoURI, cfg.MongoDBName)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer db.Close()
	log.Println("Connected to MongoDB")

	// Start scheduler
	scheduler := services.NewScheduler(db)
	scheduler.Start()
	defer scheduler.Stop()

	r := gin.Default()

	// CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// Handlers
	authHandler := &handlers.AuthHandler{DB: db, Secret: cfg.JWTSecret}
	postHandler := &handlers.PostHandler{DB: db, UploadDir: cfg.UploadDir}
	suffixHandler := &handlers.SuffixHandler{DB: db}
	bskyService := &services.BlueskyService{DB: db, UploadDir: cfg.UploadDir}
	igService := &services.InstagramService{DB: db}
	adminHandler := &handlers.AdminHandler{DB: db, Bluesky: bskyService, Instagram: igService}

	// Public routes
	api := r.Group("/api")
	{
		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)
		api.GET("/auth/registration-status", authHandler.RegistrationStatus)
		api.GET("/auth/instagram/callback", adminHandler.InstagramCallback)
		api.GET("/uploads/:filename", postHandler.ServeImage)
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		api.GET("/webhooks/instagram", adminHandler.InstagramWebhookVerify)
		api.POST("/webhooks/instagram", adminHandler.InstagramWebhookEvent)
		api.GET("/settings/public", adminHandler.GetPublicSettings)
	}

	// Authenticated routes
	auth := api.Group("", middleware.AuthRequired(cfg.JWTSecret, db))
	{
		auth.GET("/auth/me", authHandler.Me)
		auth.POST("/auth/api-token", authHandler.GenerateAPIToken)

		// Posts
		auth.GET("/posts", postHandler.List)
		auth.POST("/posts", postHandler.Create)
		auth.GET("/posts/:id", postHandler.Get)
		auth.PUT("/posts/:id", postHandler.Update)
		auth.DELETE("/posts/:id", postHandler.Delete)
		auth.PATCH("/posts/:id/reschedule", postHandler.Reschedule)
		auth.POST("/upload", postHandler.UploadImage)
		auth.POST("/upload-from-url", postHandler.UploadFromURL)
		auth.GET("/accounts", adminHandler.ListActiveAccounts)
		auth.POST("/generate-text", adminHandler.GenerateText)

		// Suffixes
		auth.GET("/suffixes", suffixHandler.List)
		auth.POST("/suffixes", suffixHandler.Create)
		auth.PUT("/suffixes/:id", suffixHandler.Update)
		auth.DELETE("/suffixes/:id", suffixHandler.Delete)
	}

	// Admin routes
	admin := auth.Group("/admin", middleware.AdminRequired())
	{
		admin.GET("/accounts", adminHandler.ListAccounts)
		admin.POST("/accounts/bluesky", adminHandler.AddBlueskyAccount)
		admin.DELETE("/accounts/:id", adminHandler.DeleteAccount)
		admin.PATCH("/accounts/:id/toggle", adminHandler.ToggleAccount)
		admin.GET("/settings", adminHandler.GetSettings)
		admin.PUT("/settings", adminHandler.UpdateSettings)
		admin.GET("/instagram/auth-url", adminHandler.InstagramAuthURL)
		admin.GET("/users", adminHandler.ListUsers)
		admin.POST("/users", adminHandler.CreateUser)
		admin.DELETE("/users/:id", adminHandler.DeleteUser)
		admin.GET("/teams", adminHandler.ListTeams)
		admin.POST("/teams", adminHandler.CreateTeam)
		admin.DELETE("/teams/:id", adminHandler.DeleteTeam)
		admin.PUT("/teams/:id/members", adminHandler.SetTeamMembers)
		admin.POST("/teams/:id/token", adminHandler.GenerateTeamToken)
	}

	// Serve embedded frontend (SPA with index.html fallback)
	serveFrontend(r)

	log.Printf("Server starting on port %s", cfg.APIPort)
	r.Run(":" + cfg.APIPort)
}
