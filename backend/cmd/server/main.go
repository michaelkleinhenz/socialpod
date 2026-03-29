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

	// Serve uploads
	r.Static("/api/uploads", cfg.UploadDir)

	// Handlers
	authHandler := &handlers.AuthHandler{DB: db, Secret: cfg.JWTSecret}
	postHandler := &handlers.PostHandler{DB: db, UploadDir: cfg.UploadDir}
	igService := &services.InstagramService{DB: db}
	adminHandler := &handlers.AdminHandler{DB: db, Instagram: igService}

	// Public routes
	api := r.Group("/api")
	{
		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)
		api.GET("/auth/registration-status", authHandler.RegistrationStatus)
		api.GET("/auth/instagram/callback", adminHandler.InstagramCallback)
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
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
	}

	// Serve embedded frontend (SPA with index.html fallback)
	serveFrontend(r)

	log.Printf("Server starting on port %s", cfg.APIPort)
	r.Run(":" + cfg.APIPort)
}
