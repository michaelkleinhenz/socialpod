package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"socialmedia/internal/config"
	"socialmedia/internal/database"
	"socialmedia/internal/handlers"
	"socialmedia/internal/middleware"
	"socialmedia/internal/objectstore"
	"socialmedia/internal/services"
	"socialmedia/internal/uploadstore"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// dynamicManifest serves a web app manifest whose start_url is supplied via the
// ?start_url= query param, so an individual page can make "Add to Home Screen"
// create a shortcut that reopens that page. The param is constrained to a
// same-origin absolute path (must start with a single "/") to avoid pointing
// the shortcut at another origin; anything else falls back to the app root.
func dynamicManifest(c *gin.Context) {
	startURL := c.Query("start_url")
	if !strings.HasPrefix(startURL, "/") || strings.HasPrefix(startURL, "//") {
		startURL = "/"
	}

	manifest := map[string]any{
		"name":             "SocialPod",
		"short_name":       "SocialPod",
		"description":      "Schedule and manage your social media posts",
		"start_url":        startURL,
		"scope":            "/",
		"display":          "standalone",
		"background_color": "#0f172a",
		"theme_color":      "#6366f1",
		"icons": []map[string]any{
			{"src": "/favicon.svg", "type": "image/svg+xml", "sizes": "any", "purpose": "any"},
		},
	}

	body, _ := json.Marshal(manifest)
	c.Data(http.StatusOK, "application/manifest+json", body)
}

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

	// Uploaded files and full-system backups share the S3-compatible bucket
	// (AWS_* env vars): uploads under uploads/, backups under backups/.
	s3, s3Err := objectstore.NewS3FromEnv()
	switch {
	case s3Err != nil:
		log.Printf("WARNING: S3 storage misconfigured (%v); uploads are stored in MongoDB and backups are disabled", s3Err)
	case s3 != nil:
		uploadstore.Configure(s3)
		log.Printf("Upload storage: S3 bucket %s", os.Getenv("AWS_S3_BUCKET_NAME"))
		// Uploads stored in MongoDB before the bucket move out in the
		// background; until then they are still read from their records.
		go func() {
			n, err := uploadstore.MigrateToBucket(context.Background(), db)
			if err != nil {
				log.Printf("Upload storage: moving uploads from MongoDB to S3: %v (%d moved, the rest are retried on the next start)", err, n)
			} else if n > 0 {
				log.Printf("Upload storage: moved %d uploads from MongoDB to S3", n)
			}
		}()
	default:
		log.Println("WARNING: upload storage: no S3 bucket configured (set AWS_S3_BUCKET_NAME, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY); uploads are stored in MongoDB")
	}

	// Watermarks used to have a collection of their own; they are media
	// library images now.
	if n, err := handlers.MigrateWatermarksToMedia(context.Background(), db, cfg.UploadDir); err != nil {
		log.Printf("WARNING: moving watermarks into the media library: %v (%d moved, the rest are retried on the next start)", err, n)
	} else if n > 0 {
		log.Printf("Moved %d watermarks into the media library", n)
	}

	// Gallery photos and items of deleted convention queues, left over from
	// before a queue's deletion reliably took them along.
	go func() {
		if n, err := handlers.PurgeOrphanedConventionData(context.Background(), db, cfg.UploadDir); err != nil {
			log.Printf("WARNING: deleting leftovers of deleted convention queues: %v", err)
		} else if n > 0 {
			log.Printf("Deleted %d gallery photos and items of deleted convention queues", n)
		}
	}()

	// Start scheduler
	scheduler := services.NewScheduler(db, cfg.UploadDir)
	scheduler.Start()
	defer scheduler.Stop()

	r := gin.Default()

	// CORS — restrict to the configured APP_URL; additional origins can be
	// appended via the CORS_ORIGINS env var (comma-separated).
	// Browser extensions (chrome-extension://, moz-extension://) are allowed
	// automatically so the MCP Chrome extension can reach /api/mcp.
	// AllowCredentials is intentionally omitted: auth is header-based (Bearer
	// tokens), so cookie-credential sharing is not required.
	allowedOrigins := map[string]bool{cfg.AppURL: true}
	if extra := os.Getenv("CORS_ORIGINS"); extra != "" {
		for _, o := range strings.Split(extra, ",") {
			if o = strings.TrimSpace(o); o != "" {
				allowedOrigins[o] = true
			}
		}
	}
	r.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool {
			if allowedOrigins[origin] {
				return true
			}
			return strings.HasPrefix(origin, "chrome-extension://") ||
				strings.HasPrefix(origin, "moz-extension://")
		},
		AllowMethods:  []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:  []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders: []string{"Content-Length", "WWW-Authenticate"},
	}))

	// Handlers
	authHandler := &handlers.AuthHandler{DB: db, Secret: cfg.JWTSecret}
	postHandler := &handlers.PostHandler{DB: db, UploadDir: cfg.UploadDir}
	footerHandler := &handlers.FooterHandler{DB: db}
	mentionHandler := &handlers.MentionHandler{DB: db}
	bskyService := &services.BlueskyService{DB: db, UploadDir: cfg.UploadDir}
	igService := &services.InstagramService{DB: db}
	twService := &services.TwitterService{DB: db, UploadDir: cfg.UploadDir}
	mastodonService := &services.MastodonService{DB: db, UploadDir: cfg.UploadDir}
	threadsService := &services.ThreadsService{DB: db}
	linkedInService := &services.LinkedInService{DB: db, UploadDir: cfg.UploadDir}
	youtubeService := &services.YouTubeService{DB: db, UploadDir: cfg.UploadDir}
	adminHandler := &handlers.AdminHandler{DB: db, Bluesky: bskyService, Instagram: igService, Twitter: twService, Mastodon: mastodonService, Threads: threadsService, LinkedIn: linkedInService, YouTube: youtubeService, UploadDir: cfg.UploadDir}
	inboxHandler := &handlers.InboxHandler{DB: db, Instagram: igService, Bluesky: bskyService}
	conventionHandler := &handlers.ConventionHandler{DB: db, UploadDir: cfg.UploadDir}
	conventionHandler.StartAutoPoster()
	handlers.StartTemporaryUploadCleanup(db, cfg.UploadDir)
	defer conventionHandler.StopAutoPoster()
	bggHandler := &handlers.BGGHandler{DB: db, UploadDir: cfg.UploadDir}
	publisherHandleHandler := &handlers.PublisherHandleHandler{DB: db}
	newsHandler := &handlers.NewsHandler{DB: db, UploadDir: cfg.UploadDir}
	episodeHandler := &handlers.EpisodeHandler{DB: db, UploadDir: cfg.UploadDir}
	emailService := &services.EmailService{DB: db}
	inviteHandler := &handlers.InviteHandler{DB: db, Email: emailService, AppURL: cfg.AppURL, JWTSecret: cfg.JWTSecret}
	growthHandler := &handlers.GrowthHandler{DB: db, Scheduler: scheduler, UploadDir: cfg.UploadDir}
	transcribeHandler := &handlers.TranscribeHandler{DB: db}
	mediaHandler := &handlers.MediaHandler{DB: db, UploadDir: cfg.UploadDir}
	clipartHandler := handlers.NewClipartHandler(db, cfg.UploadDir, cfg.IconifyAPIURL)
	handlers.SetAppBaseURL(cfg.AppURL)
	mcpHandler := &handlers.MCPHandler{DB: db, UploadDir: cfg.UploadDir, BGG: bggHandler, Scheduler: scheduler}
	captureHandler := &handlers.CaptureHandler{DB: db, UploadDir: cfg.UploadDir, BGG: bggHandler}
	oauthHandler := handlers.NewOAuthHandler(db, cfg.AppURL)
	agentHandler := &handlers.AgentHandler{DB: db, MCP: mcpHandler}
	agentHandler.StartScheduler()
	defer agentHandler.StopScheduler()

	// Full-system backups go to an S3-compatible bucket (AWS_* env vars).
	backupHandler := &handlers.SystemBackupHandler{DB: db, UploadDir: cfg.UploadDir, Agents: agentHandler, OAuth: oauthHandler}
	if s3 != nil {
		backupHandler.Store = s3
		log.Printf("Backup storage: S3 bucket %s", os.Getenv("AWS_S3_BUCKET_NAME"))
	} else {
		log.Println("Backup storage: not configured; backups disabled")
	}
	backupHandler.StartScheduler()
	defer backupHandler.StopScheduler()

	robotsTxt := func(c *gin.Context) {
		c.Data(http.StatusOK, "text/plain", []byte("User-agent: *\nAllow: /\n"))
	}
	r.GET("/robots.txt", robotsTxt)
	r.HEAD("/robots.txt", robotsTxt)

	// Dynamic web app manifest. iOS Safari's "Add to Home Screen" reads the
	// manifest's start_url but — unlike Android Chrome — refuses blob:/data:
	// URLs for <link rel="manifest">, so a client-generated manifest is
	// ignored and the shortcut falls back to the static start_url "/". This
	// endpoint serves a real, fetchable manifest whose start_url is taken from
	// the ?start_url= query param, letting a page point the manifest at itself.
	r.GET("/manifest.webmanifest", dynamicManifest)

	// OAuth 2.1 / OpenID Connect discovery for remote MCP clients (Claude.ai
	// connectors, ChatGPT, Claude Code). Each document is also served under the
	// path-suffixed form (…/api/mcp) that clients derive from the MCP URL.
	r.GET("/.well-known/oauth-protected-resource", oauthHandler.ProtectedResourceMetadata)
	r.GET("/.well-known/oauth-protected-resource/api/mcp", oauthHandler.ProtectedResourceMetadata)
	for _, base := range []string{"/.well-known/oauth-authorization-server", "/.well-known/openid-configuration"} {
		r.GET(base, oauthHandler.AuthorizationServerMetadata)
		r.GET(base+"/api/mcp", oauthHandler.AuthorizationServerMetadata)
	}

	// Public routes
	api := r.Group("/api")
	{
		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)
		api.GET("/auth/registration-status", authHandler.RegistrationStatus)
		api.GET("/auth/instagram/callback", adminHandler.InstagramCallback)
		api.GET("/auth/mastodon/callback", adminHandler.MastodonCallback)
		api.GET("/auth/linkedin/callback", adminHandler.LinkedInCallback)
		api.GET("/auth/youtube/callback", adminHandler.YouTubeCallback)
		api.GET("/uploads/:filename", postHandler.ServeImage)
		// Single-use upload URLs (create_upload_url): the token is the
		// authentication.
		api.PUT("/upload/once/:token", postHandler.UploadOnce)
		api.POST("/upload/once/:token", postHandler.UploadOnce)
		api.HEAD("/uploads/:filename", postHandler.ServeImage)
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		api.GET("/webhooks/instagram", adminHandler.InstagramWebhookVerify)
		api.POST("/webhooks/instagram", adminHandler.InstagramWebhookEvent)
		api.GET("/settings/public", adminHandler.GetPublicSettings)
		api.GET("/invites/info", inviteHandler.GetInviteInfo)
		api.POST("/invites/accept", inviteHandler.AcceptInvite)

		// OAuth server endpoints called by MCP clients. The authorization
		// endpoint itself is the SPA page /oauth/authorize.
		api.POST("/oauth/register", oauthHandler.Register)
		api.POST("/oauth/token", oauthHandler.Token)
		api.POST("/oauth/revoke", oauthHandler.Revoke)
		api.GET("/oauth/userinfo", oauthHandler.UserInfo)
		api.POST("/oauth/userinfo", oauthHandler.UserInfo)
		api.GET("/oauth/jwks", oauthHandler.JWKS)
	}

	// Authenticated routes
	auth := api.Group("", middleware.AuthRequired(cfg.JWTSecret, db))
	{
		auth.GET("/auth/me", authHandler.Me)
		auth.PUT("/auth/password", authHandler.UpdatePassword)
		auth.POST("/auth/api-token", authHandler.GenerateAPIToken)
		auth.GET("/account-warnings", authHandler.AccountWarnings)

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
		// The image editor's clips and sounds for a video: temporary until
		// something references them (uploads_temp.go).
		auth.POST("/upload/temp", postHandler.UploadTemp)
		auth.POST("/upload/release", postHandler.ReleaseTemp)
		auth.POST("/upload-from-url", postHandler.UploadFromURL)
		auth.GET("/accounts", adminHandler.ListActiveAccounts)
		auth.POST("/generate-text", adminHandler.GenerateText)
		auth.POST("/news/submit", newsHandler.Submit)
		auth.GET("/news/drafts", newsHandler.ListDrafts)
		auth.POST("/news/drafts", newsHandler.SaveDraft)
		auth.GET("/news/drafts/:id", newsHandler.GetDraft)
		auth.PUT("/news/drafts/:id", newsHandler.UpdateDraft)
		auth.DELETE("/news/drafts", newsHandler.DeleteDrafts)
		auth.DELETE("/news/drafts/:id", newsHandler.DeleteDraft)
		auth.POST("/news/drafts/:id/post", newsHandler.PostDraft)
		auth.POST("/episode/submit", episodeHandler.Submit)
		auth.GET("/episode/drafts", episodeHandler.ListDrafts)
		auth.POST("/episode/drafts", episodeHandler.SaveDraft)
		auth.GET("/episode/drafts/:id", episodeHandler.GetDraft)
		auth.PUT("/episode/drafts/:id", episodeHandler.UpdateDraft)
		auth.DELETE("/episode/drafts", episodeHandler.DeleteDrafts)
		auth.DELETE("/episode/drafts/:id", episodeHandler.DeleteDraft)
		auth.POST("/episode/drafts/:id/post", episodeHandler.PostDraft)

		// Team settings (read-only for all team members — sidebar plugin visibility)
		auth.GET("/team/settings", bggHandler.GetTeamSettings)

		// BGG integration
		auth.GET("/bgg/fetch", bggHandler.FetchGame)
		auth.POST("/dashboard/ai-insights", adminHandler.DashboardInsights)

		// Instagram growth: goal tracker, weekly report, Insights
		auth.GET("/growth/settings", growthHandler.GetSettings)
		auth.GET("/growth/instagram", growthHandler.Overview)
		auth.POST("/growth/refresh", growthHandler.RefreshInsights)
		auth.POST("/growth/rewrite-instagram", growthHandler.RewriteInstagram)
		auth.POST("/transcribe", transcribeHandler.Transcribe)
		auth.POST("/posts/:id/insights", growthHandler.RefreshPostInsights)
		auth.GET("/dashboard/stats", adminHandler.DashboardStats)

		// Feed
		auth.GET("/inbox/feed", inboxHandler.GetFeed)

		// Watermarks: the media library's images marked as watermarks,
		// kept as their own endpoints for API clients.
		auth.GET("/watermarks", mediaHandler.ListWatermarks)
		auth.POST("/watermarks", mediaHandler.UploadWatermark)
		auth.PUT("/watermarks/:id", mediaHandler.UpdateWatermark)
		auth.DELETE("/watermarks/:id", mediaHandler.DeleteWatermark)

		// Clipart for the image editor (Iconify, OpenClipart)
		auth.GET("/clipart/search", clipartHandler.Search)
		auth.POST("/clipart/openclipart/import", clipartHandler.ImportOpenClipart)

		// Media library
		auth.GET("/media", mediaHandler.List)
		auth.POST("/media", mediaHandler.Create)
		auth.POST("/media/colors", mediaHandler.CreateColor)
		auth.POST("/media/templates", mediaHandler.CreateTemplate)
		auth.PUT("/media/templates/:id", mediaHandler.UpdateTemplate)
		auth.PUT("/media/:id", mediaHandler.Update)
		auth.DELETE("/media/:id", mediaHandler.Delete)

		// Image editor drafts: unfinished work, the user's own.
		auth.GET("/image-drafts", mediaHandler.ListImageDrafts)
		auth.GET("/image-drafts/:id", mediaHandler.GetImageDraft)
		auth.POST("/image-drafts", mediaHandler.CreateImageDraft)
		auth.PUT("/image-drafts/:id", mediaHandler.UpdateImageDraft)
		auth.DELETE("/image-drafts/:id", mediaHandler.DeleteImageDraft)

		// Footers (team/user scoped)
		auth.GET("/footers", footerHandler.List)
		auth.POST("/footers", footerHandler.Create)
		auth.PUT("/footers/:id", footerHandler.Update)
		auth.DELETE("/footers/:id", footerHandler.Delete)

		// Mentions (team/user scoped)
		auth.GET("/mentions", mentionHandler.List)
		auth.POST("/mentions", mentionHandler.Create)
		auth.PUT("/mentions/:id", mentionHandler.Update)
		auth.DELETE("/mentions/:id", mentionHandler.Delete)
		auth.GET("/mentions/export", mentionHandler.Export)
		auth.POST("/mentions/import", mentionHandler.Import)

		// Agents: scheduled prompts that run the user's MCP tools
		auth.GET("/agents/settings", agentHandler.GetSettings)
		auth.PUT("/agents/settings", agentHandler.UpdateSettings)
		auth.GET("/agents/models", agentHandler.ListModels)
		auth.GET("/agents", agentHandler.List)
		auth.POST("/agents", agentHandler.Create)
		auth.PUT("/agents/:id", agentHandler.Update)
		auth.DELETE("/agents/:id", agentHandler.Delete)
		auth.POST("/agents/:id/run", agentHandler.Run)
		auth.POST("/agents/:id/stop", agentHandler.Stop)
		auth.GET("/agents/:id/runs", agentHandler.ListRuns)
		auth.GET("/agents/:id/runs/:runId", agentHandler.GetRun)

		// Chrome extension capture endpoint
		auth.POST("/capture", captureHandler.Capture)

		// OAuth consent (called by the /oauth/authorize page) and the
		// Profile page's list of connected apps.
		auth.GET("/oauth/authorize", oauthHandler.AuthorizeInfo)
		auth.POST("/oauth/authorize", oauthHandler.AuthorizeDecision)
		auth.GET("/oauth/grants", oauthHandler.ListGrants)
		auth.DELETE("/oauth/grants/:clientId", oauthHandler.RevokeGrant)

		// Convention mode
		auth.GET("/convention/queues", conventionHandler.ListQueues)
		auth.POST("/convention/queues", conventionHandler.CreateQueue)
		auth.GET("/convention/queues/:id", conventionHandler.GetQueue)
		auth.PUT("/convention/queues/:id", conventionHandler.UpdateQueue)
		auth.DELETE("/convention/queues/:id", conventionHandler.DeleteQueue)
		auth.POST("/convention/queues/:id/items", conventionHandler.AddItem)
		auth.POST("/convention/queues/:id/items/gallery", conventionHandler.AddGalleryItem)
		auth.POST("/convention/queues/:id/items/bgg", conventionHandler.AddBGGItems)
		auth.PUT("/convention/queues/:id/items/:iid", conventionHandler.UpdateItem)
		auth.PUT("/convention/queues/:id/items/:iid/image", conventionHandler.ReplaceItemImage)
		auth.DELETE("/convention/queues/:id/items/:iid", conventionHandler.DeleteItem)
		auth.POST("/convention/queues/:id/items/:iid/images", conventionHandler.AddItemImage)
		auth.PUT("/convention/queues/:id/items/:iid/images", conventionHandler.SetItemImages)
		auth.POST("/convention/queues/:id/items/:iid/analyze", conventionHandler.AnalyzeItem)
		auth.POST("/convention/queues/:id/items/:iid/post-now", conventionHandler.PostItemNow)
		auth.POST("/convention/queues/:id/analyze-all", conventionHandler.AnalyzeAll)
		auth.GET("/convention/queues/:id/preview", conventionHandler.PreviewSchedule)
		auth.POST("/convention/queues/:id/schedule", conventionHandler.ScheduleItems)
		auth.POST("/convention/queues/:id/gallery", conventionHandler.AddGalleryPhotos)
		auth.POST("/convention/queues/:id/gallery/move", conventionHandler.MoveGalleryPhotos)
		auth.DELETE("/convention/queues/:id/gallery/:pid", conventionHandler.DeleteGalleryPhoto)
		auth.PUT("/convention/queues/:id/gallery/:pid/image", conventionHandler.ReplaceGalleryPhoto)
		auth.POST("/convention/queues/:id/reel/caption", conventionHandler.QuickReelCaption)
		auth.POST("/convention/queues/:id/reel", conventionHandler.PublishQuickReel)
	}

	// MCP server (Streamable HTTP transport). It has its own auth: besides
	// JWTs and API tokens it accepts OAuth access tokens, and it exposes user
	// features only — admin rights are dropped even for an admin.
	mcp := api.Group("/mcp", middleware.MCPAuthRequired(cfg.JWTSecret, db, oauthHandler.ResourceMetadataURL()))
	{
		mcp.GET("", mcpHandler.HandleGet)
		mcp.POST("", mcpHandler.Handle)
		mcp.DELETE("", mcpHandler.HandleDelete)
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
		admin.POST("/accounts/:id/test", adminHandler.TestAccount)
		admin.GET("/accounts/:id/reauth-url", adminHandler.ReauthAccountURL)
		admin.GET("/settings", adminHandler.GetSettings)
		admin.PUT("/settings", adminHandler.UpdateSettings)
		admin.GET("/instagram/auth-url", adminHandler.InstagramAuthURL)
		admin.GET("/mastodon/auth-url", adminHandler.MastodonAuthURL)
		admin.GET("/linkedin/auth-url", adminHandler.LinkedInAuthURL)
		admin.GET("/youtube/auth-url", adminHandler.YouTubeAuthURL)
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
		admin.GET("/teams/:id/plugins", adminHandler.GetTeamPlugins)
		admin.PUT("/teams/:id/plugins", adminHandler.UpdateTeamPlugins)

		// Team invitations (admin)
		admin.GET("/teams/:id/invites", inviteHandler.ListTeamInvites)
		admin.POST("/teams/:id/invites", inviteHandler.CreateInvite)
		admin.DELETE("/teams/:id/invites/:inviteId", inviteHandler.DeleteInvite)

		// Publisher handle catalog (global, admin-managed)
		// Storage maintenance: scan for uploads nothing references any more,
		// and delete them when asked explicitly.
		admin.GET("/uploads/orphans", adminHandler.SweepUploads)
		admin.POST("/uploads/sweep", adminHandler.SweepUploads)

		// Full-system backups (S3 bucket, encrypted ZIP)
		admin.GET("/backups/config", backupHandler.GetConfig)
		admin.PUT("/backups/config", backupHandler.UpdateConfig)
		admin.GET("/backups", backupHandler.List)
		admin.POST("/backups", backupHandler.RunNow)
		admin.POST("/backups/restore-upload", backupHandler.RestoreUpload)
		admin.GET("/backups/:name/download", backupHandler.Download)
		admin.POST("/backups/:name/restore", backupHandler.RestoreStored)
		admin.DELETE("/backups/:name", backupHandler.Delete)

		admin.GET("/publisher-handles", publisherHandleHandler.List)
		admin.POST("/publisher-handles", publisherHandleHandler.Create)
		admin.PUT("/publisher-handles/:id", publisherHandleHandler.Update)
		admin.DELETE("/publisher-handles/:id", publisherHandleHandler.Delete)
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
		teamAdmin.POST("/accounts/:id/test", adminHandler.TeamTestAccount)
		teamAdmin.GET("/accounts/:id/reauth-url", adminHandler.ReauthAccountURL)
		teamAdmin.GET("/instagram/auth-url", adminHandler.TeamInstagramAuthURL)
		teamAdmin.GET("/mastodon/auth-url", adminHandler.TeamMastodonAuthURL)
		teamAdmin.GET("/linkedin/auth-url", adminHandler.TeamLinkedInAuthURL)
		teamAdmin.GET("/youtube/auth-url", adminHandler.TeamYouTubeAuthURL)
		teamAdmin.GET("/members", adminHandler.TeamListMembers)
		teamAdmin.POST("/members", adminHandler.TeamAddMember)
		teamAdmin.DELETE("/members/:id", adminHandler.TeamRemoveMember)

		// Team invitations
		teamAdmin.GET("/invites", inviteHandler.TeamListInvites)
		teamAdmin.POST("/invites", inviteHandler.TeamCreateInvite)
		teamAdmin.DELETE("/invites/:inviteId", inviteHandler.TeamDeleteInvite)

		// Team-scoped BGG settings (GET moved to auth group for sidebar visibility)
		teamAdmin.PUT("/settings", bggHandler.UpdateTeamSettings)
		// Instagram growth settings: goal, weekly plan, caption tone
		teamAdmin.PUT("/growth", growthHandler.UpdateSettings)
	}

	// Serve embedded frontend (SPA with index.html fallback)
	serveFrontend(r)

	log.Printf("Server starting on port %s", cfg.APIPort)
	r.Run(":" + cfg.APIPort)
}
