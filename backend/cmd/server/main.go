package main

import (
	"log"
	"net/http"
	"os"
	"strings"

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

	// Warn on known-insecure defaults so operators catch misconfigurations early.
	if v := os.Getenv("JWT_SECRET"); v == "" || v == "change-me-in-production" {
		log.Println("WARNING: JWT_SECRET is not set or uses the default value — set a strong random secret before exposing this service")
	}
	if v := os.Getenv("MONGO_PASSWORD"); v == "" || v == "socialmedia_secret" {
		log.Println("WARNING: MONGO_PASSWORD is not set or uses the default value — set a strong password before exposing this service")
	}

	db, err := database.Connect(cfg.MongoURI, cfg.MongoDBName)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer db.Close()
	log.Println("Connected to MongoDB")

	// Start scheduler
	scheduler := services.NewScheduler(db, cfg.UploadDir)
	scheduler.Start()
	defer scheduler.Stop()

	r := gin.Default()

	// CORS — restrict to the configured APP_URL; additional origins can be
	// appended via the CORS_ORIGINS env var (comma-separated).
	// AllowCredentials is intentionally omitted: auth is header-based (Bearer
	// tokens), so cookie-credential sharing is not required.
	allowedOrigins := []string{cfg.AppURL}
	if extra := os.Getenv("CORS_ORIGINS"); extra != "" {
		for _, o := range strings.Split(extra, ",") {
			if o = strings.TrimSpace(o); o != "" {
				allowedOrigins = append(allowedOrigins, o)
			}
		}
	}
	r.Use(cors.New(cors.Config{
		AllowOrigins:  allowedOrigins,
		AllowMethods:  []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:  []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders: []string{"Content-Length"},
	}))

	// Handlers
	authHandler := &handlers.AuthHandler{DB: db, Secret: cfg.JWTSecret}
	postHandler := &handlers.PostHandler{DB: db, UploadDir: cfg.UploadDir}
	suffixHandler := &handlers.SuffixHandler{DB: db}
	mentionHandler := &handlers.MentionHandler{DB: db}
	bskyService := &services.BlueskyService{DB: db, UploadDir: cfg.UploadDir}
	igService := &services.InstagramService{DB: db}
	twService := &services.TwitterService{DB: db, UploadDir: cfg.UploadDir}
	mastodonService := &services.MastodonService{DB: db, UploadDir: cfg.UploadDir}
	threadsService := &services.ThreadsService{DB: db}
	linkedInService := &services.LinkedInService{DB: db, UploadDir: cfg.UploadDir}
	adminHandler := &handlers.AdminHandler{DB: db, Bluesky: bskyService, Instagram: igService, Twitter: twService, Mastodon: mastodonService, Threads: threadsService, LinkedIn: linkedInService, UploadDir: cfg.UploadDir}
	inboxHandler := &handlers.InboxHandler{DB: db, Instagram: igService, Bluesky: bskyService}
	conventionHandler := &handlers.ConventionHandler{DB: db, UploadDir: cfg.UploadDir}
	bggHandler := &handlers.BGGHandler{DB: db, UploadDir: cfg.UploadDir}

	robotsTxt := func(c *gin.Context) {
		c.Data(http.StatusOK, "text/plain", []byte("User-agent: *\nAllow: /\n"))
	}
	r.GET("/robots.txt", robotsTxt)
	r.HEAD("/robots.txt", robotsTxt)

	// Public routes
	api := r.Group("/api")
	{
		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)
		api.GET("/auth/registration-status", authHandler.RegistrationStatus)
		api.GET("/auth/instagram/callback", adminHandler.InstagramCallback)
		api.GET("/auth/mastodon/callback", adminHandler.MastodonCallback)
		api.GET("/auth/linkedin/callback", adminHandler.LinkedInCallback)
		api.GET("/uploads/:filename", postHandler.ServeImage)
		api.HEAD("/uploads/:filename", postHandler.ServeImage)
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
		auth.PUT("/auth/password", authHandler.UpdatePassword)
		auth.POST("/auth/api-token", authHandler.GenerateAPIToken)

		// Posts
		auth.GET("/posts", postHandler.List)
		auth.POST("/posts", postHandler.Create)
		auth.GET("/posts/:id", postHandler.Get)
		auth.PUT("/posts/:id", postHandler.Update)
		auth.DELETE("/posts/:id", postHandler.Delete)
		auth.PATCH("/posts/:id/reschedule", postHandler.Reschedule)
		auth.POST("/posts/:id/retry", postHandler.Retry)
		auth.POST("/posts/:id/retry-news", postHandler.RetryEpisodeNews)
		auth.POST("/upload", postHandler.UploadImage)
		auth.POST("/upload-from-url", postHandler.UploadFromURL)
		auth.GET("/accounts", adminHandler.ListActiveAccounts)
		auth.POST("/generate-text", adminHandler.GenerateText)

		// BGG integration
		auth.GET("/bgg/fetch", bggHandler.FetchGame)
		auth.POST("/dashboard/ai-insights", adminHandler.DashboardInsights)
		auth.GET("/dashboard/stats", adminHandler.DashboardStats)

		// Feed
		auth.GET("/inbox/feed", inboxHandler.GetFeed)

		// Watermarks (team/user scoped)
		auth.GET("/watermarks", adminHandler.ListWatermarks)
		auth.POST("/watermarks", adminHandler.UploadWatermark)
		auth.DELETE("/watermarks/:id", adminHandler.DeleteWatermark)

		// Suffixes (team/user scoped)
		auth.GET("/suffixes", suffixHandler.List)
		auth.POST("/suffixes", suffixHandler.Create)
		auth.PUT("/suffixes/:id", suffixHandler.Update)
		auth.DELETE("/suffixes/:id", suffixHandler.Delete)

		// Mentions (team/user scoped)
		auth.GET("/mentions", mentionHandler.List)
		auth.POST("/mentions", mentionHandler.Create)
		auth.PUT("/mentions/:id", mentionHandler.Update)
		auth.DELETE("/mentions/:id", mentionHandler.Delete)

		// Convention mode
		auth.GET("/convention/queues", conventionHandler.ListQueues)
		auth.POST("/convention/queues", conventionHandler.CreateQueue)
		auth.GET("/convention/queues/:id", conventionHandler.GetQueue)
		auth.PUT("/convention/queues/:id", conventionHandler.UpdateQueue)
		auth.DELETE("/convention/queues/:id", conventionHandler.DeleteQueue)
		auth.POST("/convention/queues/:id/items", conventionHandler.AddItem)
		auth.PUT("/convention/queues/:id/items/:iid", conventionHandler.UpdateItem)
		auth.PUT("/convention/queues/:id/items/:iid/image", conventionHandler.ReplaceItemImage)
		auth.DELETE("/convention/queues/:id/items/:iid", conventionHandler.DeleteItem)
		auth.POST("/convention/queues/:id/items/:iid/analyze", conventionHandler.AnalyzeItem)
		auth.POST("/convention/queues/:id/analyze-all", conventionHandler.AnalyzeAll)
		auth.POST("/convention/queues/:id/reorder", conventionHandler.ReorderItems)
		auth.GET("/convention/queues/:id/preview", conventionHandler.PreviewSchedule)
		auth.POST("/convention/queues/:id/schedule", conventionHandler.ScheduleItems)
	}

	// Admin routes (global admin only)
	admin := auth.Group("/admin", middleware.AdminRequired())
	{
		admin.GET("/accounts", adminHandler.ListAccounts)
		admin.POST("/accounts/bluesky", adminHandler.AddBlueskyAccount)
		admin.POST("/accounts/twitter", adminHandler.AddTwitterAccount)
		admin.POST("/accounts/mastodon", adminHandler.AddMastodonAccount)
		admin.POST("/accounts/threads", adminHandler.AddThreadsAccount)
		admin.POST("/accounts/linkedin", adminHandler.AddLinkedInAccount)
		admin.DELETE("/accounts/:id", adminHandler.DeleteAccount)
		admin.PATCH("/accounts/:id/toggle", adminHandler.ToggleAccount)
		admin.PATCH("/accounts/:id/team", adminHandler.AssignAccountTeam)
		admin.GET("/settings", adminHandler.GetSettings)
		admin.PUT("/settings", adminHandler.UpdateSettings)
		admin.GET("/instagram/auth-url", adminHandler.InstagramAuthURL)
		admin.GET("/mastodon/auth-url", adminHandler.MastodonAuthURL)
		admin.GET("/linkedin/auth-url", adminHandler.LinkedInAuthURL)
		admin.GET("/users", adminHandler.ListUsers)
		admin.POST("/users", adminHandler.CreateUser)
		admin.DELETE("/users/:id", adminHandler.DeleteUser)
		admin.PATCH("/users/:id/role", adminHandler.UpdateUserRole)
		admin.POST("/users/:id/reset-password", adminHandler.ResetUserPassword)
		admin.GET("/teams", adminHandler.ListTeams)
		admin.POST("/teams", adminHandler.CreateTeam)
		admin.DELETE("/teams/:id", adminHandler.DeleteTeam)
		admin.PUT("/teams/:id/members", adminHandler.SetTeamMembers)
		admin.POST("/teams/:id/token", adminHandler.GenerateTeamToken)
		admin.GET("/teams/:id/settings", bggHandler.AdminGetTeamSettings)
		admin.PUT("/teams/:id/settings", bggHandler.AdminUpdateTeamSettings)
	}

	// Team self-creation: team admin flag required, no existing team needed
	auth.POST("/team/setup", middleware.TeamAdminFlagRequired(), adminHandler.TeamAdminCreateTeam)

	// Team admin routes (team admin or global admin with a team)
	teamAdmin := auth.Group("/team", middleware.TeamAdminRequired())
	{
		teamAdmin.GET("/accounts", adminHandler.TeamListAccounts)
		teamAdmin.POST("/accounts/bluesky", adminHandler.TeamAddBlueskyAccount)
		teamAdmin.POST("/accounts/twitter", adminHandler.TeamAddTwitterAccount)
		teamAdmin.POST("/accounts/mastodon", adminHandler.TeamAddMastodonAccount)
		teamAdmin.POST("/accounts/threads", adminHandler.TeamAddThreadsAccount)
		teamAdmin.POST("/accounts/linkedin", adminHandler.TeamAddLinkedInAccount)
		teamAdmin.DELETE("/accounts/:id", adminHandler.TeamDeleteAccount)
		teamAdmin.PATCH("/accounts/:id/toggle", adminHandler.TeamToggleAccount)
		teamAdmin.GET("/instagram/auth-url", adminHandler.TeamInstagramAuthURL)
		teamAdmin.GET("/mastodon/auth-url", adminHandler.TeamMastodonAuthURL)
		teamAdmin.GET("/linkedin/auth-url", adminHandler.TeamLinkedInAuthURL)
		teamAdmin.GET("/members", adminHandler.TeamListMembers)
		teamAdmin.POST("/members", adminHandler.TeamAddMember)
		teamAdmin.DELETE("/members/:id", adminHandler.TeamRemoveMember)

		// Team-scoped BGG settings
		teamAdmin.GET("/settings", bggHandler.GetTeamSettings)
		teamAdmin.PUT("/settings", bggHandler.UpdateTeamSettings)
	}

	// Serve embedded frontend (SPA with index.html fallback)
	serveFrontend(r)

	log.Printf("Server starting on port %s", cfg.APIPort)
	r.Run(":" + cfg.APIPort)
}
