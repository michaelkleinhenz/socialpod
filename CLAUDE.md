# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Docker (primary workflow)
```bash
make up          # Build and start all services (MongoDB + app)
make down        # Stop all services
make logs        # Follow container logs
make clean       # Stop and remove volumes (destructive — deletes data)
```

### Local development (requires MongoDB running separately)
```bash
make mongo       # Start only MongoDB in Docker
make dev         # Build frontend, embed it, run Go server locally
make dev-frontend  # Run Vite dev server standalone (hot reload, proxies to backend)
```

### Backend
```bash
# From backend/
go vet ./...
go test ./... -race -count=1
go test ./path/to/package -run TestName -race  # run a single test

# Backend requires the frontend dist to exist before build/run
mkdir -p backend/cmd/server/dist && echo '<!doctype html>' > backend/cmd/server/dist/index.html
```

### Frontend
```bash
# From frontend/
npm ci
npm run build        # tsc + vite build
npm run lint         # eslint
npx tsc --noEmit     # type check only
```

### Chrome extension
```bash
make chrome-extension   # Build socialpod-capture.zip for Chrome Web Store upload
```

## Architecture

### Single-binary deployment
The React frontend is built and copied into `backend/cmd/server/dist/`, then embedded at compile time via `//go:embed dist/*` in `frontend.go`. The Go binary serves both the REST API (`/api/*`) and the SPA (all other routes fall back to `index.html`). A single port (default 8080) handles everything.

The build sequence: `npm run build` → `cp -r frontend/dist backend/cmd/server/dist` → `go build`.

### Backend (`backend/`)
Go module `socialmedia`, using Gin as the HTTP framework.

- `cmd/server/main.go` — wires all dependencies; defines four route groups: public `/api`, authenticated `/api` (JWT/API-token required), admin-only `/api/admin`, team-admin `/api/team`
- `internal/config/` — loads all config from environment variables
- `internal/database/mongo.go` — `MongoDB` wrapper that exposes typed collection accessors (`Posts()`, `Users()`, `Teams()`, etc.) and creates indexes on startup
- `internal/models/` — BSON-tagged Go structs for each MongoDB collection (`post.go`, `user.go`, `team.go`, `social_account.go`, `footer.go`, `mention.go`, `convention.go`, `upload.go`, `watermark.go`, `team_invite.go`, `publisher_handle.go`, `news_draft.go`, `episode_draft.go`)
- `internal/handlers/` — one file per handler group (`auth.go`, `posts.go`, `admin.go`, `inbox.go`, `footers.go`, `convention.go`, `mentions.go`, `invite.go`, `bgg.go`, `news.go`, `episode.go`, `publisher_handles.go`, `mcp.go`, `capture.go`)
- `internal/middleware/auth.go` — `AuthRequired` tries three token types in order: JWT → user API token (`sm_...`) → team API token (`st_...`); sets `userId`, `isAdmin`, `isTeamAdmin`, `teamId` on the Gin context. The same bearer token authenticates both REST API and MCP requests.
- `internal/services/` — platform-specific: `bluesky.go`, `instagram.go`, `twitter.go`, `mastodon.go`, `threads.go`, `linkedin.go`, `youtube.go`; infrastructure: `scheduler.go`, `imageutil.go`, `email.go`

The scheduler (`services/Scheduler`) runs every 30 seconds, queries for posts where `status == "scheduled"` and `scheduledAt <= now`, and publishes them. Footers are fetched from the DB and appended at publish time (not stored on the post itself).

Convention queues have their own 30-second background loop (`ConventionHandler.StartAutoPoster` in `handlers/convention.go`). Approved queue items are a *set*, not pre-scheduled: whenever a queue is active and inside its date window and its `nextPostAt` is due, the loop picks one approved item at random, creates a `scheduled` post for it (which the shared scheduler then publishes), marks the item consumed, and rolls `nextPostAt` forward by the schedule gap. `POST /convention/queues/:id/schedule` is a manual "post one random item now" trigger.

### MCP server (`handlers/mcp.go`)
An MCP (Model Context Protocol) server is available at `POST /api/mcp` using the Streamable HTTP transport. It exposes all user-level operations as MCP tools: posts CRUD, footers, mentions, watermarks, accounts, profile, news drafts, and episode drafts. Authentication uses the same bearer token (`sm_...`) as the REST API — one token works for both interfaces.

MCP clients (Claude Code, OpenCode, etc.) configure the server URL and an `Authorization: Bearer <token>` header. The MCP handler implements the JSON-RPC 2.0 protocol with `initialize`, `ping`, `tools/list`, and `tools/call` methods. Each tool performs the same database queries as the corresponding REST handler, respecting team/user scoping.

MCP draft tools: `create_news_draft`, `list_news_drafts`, `get_news_draft`, `update_news_draft`, `delete_news_draft`, `post_news_draft`, `create_episode_draft`, `list_episode_drafts`, `get_episode_draft`, `update_episode_draft`, `delete_episode_draft`, `post_episode_draft`. Post drafts (post/story/reel) are regular posts created with `status: "draft"` via `create_post`.

MCP image uploads: the `upload_image` tool accepts base64-encoded image data and a filename, stores the file (disk + MongoDB), and returns its URL. All create/update tools (`create_post`, `update_post`, `create_news_draft`, `update_news_draft`, `create_episode_draft`, `update_episode_draft`) also accept an `images` parameter — an array of `{data, filename}` objects with base64-encoded data — for inline image upload in a single tool call. Uploaded image URLs are merged with any `imageUrls` passed in the same call. Supported types: jpg, jpeg, png, gif, webp, mp4, mov.

### Frontend (`frontend/`)
React 19 + TypeScript + Vite. No state management library — auth state lives in `AuthContext`, everything else is local component state fetched via the `ApiClient`.

- `src/services/api.ts` — single `ApiClient` class that wraps all `fetch` calls; token stored in `localStorage`; redirects to `/login` on 401
- `src/contexts/AuthContext.tsx` — global auth state (`user`, `loading`, `login`, `logout`, `refreshUser`)
- `src/types/index.ts` — all shared TypeScript types (`Post`, `User`, `Team`, `SocialAccount`, etc.)
- `src/App.tsx` — route definitions; `ProtectedRoute` enforces `adminOnly`/`teamAdminOnly` flags

Components are organized by feature under `src/components/` (Calendar, PostEditor, Admin, Inbox, Footers, News, Episode, etc.).

### Draft system
Three draft types exist across the application:

- **Post/Story/Reel drafts** — regular `Post` documents with `status: "draft"`. Created through the PostEditor (Calendar page). The Calendar page has a "Drafts" tab that lists all draft posts across types, with edit/schedule/delete actions. MCP: use `create_post` with `status: "draft"`.
- **News drafts** — stored in the `news_drafts` collection (`models/news_draft.go`). Managed through the News page's Create/Drafts tabs. REST: `GET/POST /api/news/drafts`, `GET/PUT/DELETE /api/news/drafts/:id`, `POST /api/news/drafts/:id/post`, `DELETE /api/news/drafts?posted=true` (bulk). MCP: `create_news_draft`, `list_news_drafts`, `get_news_draft`, `update_news_draft`, `delete_news_draft`, `post_news_draft`.
- **Episode drafts** — stored in the `episode_drafts` collection (`models/episode_draft.go`). Managed through the Episodes page's Create/Drafts tabs. REST: `GET/POST /api/episode/drafts`, `GET/PUT/DELETE /api/episode/drafts/:id`, `POST /api/episode/drafts/:id/post`, `DELETE /api/episode/drafts?posted=true` (bulk). MCP: `create_episode_draft`, `list_episode_drafts`, `get_episode_draft`, `update_episode_draft`, `delete_episode_draft`, `post_episode_draft`.

News and episode drafts follow the same lifecycle: create → list/browse → edit → publish (sends to webhook + optionally creates a social post + marks the draft `posted`). Drafts are submitted as `multipart/form-data` with `data` (JSON) and optional `images` fields.

Publishing never deletes a draft. It sets `posted: true` and `postedAt`, which moves the entry from the News/Episodes page's Drafts tab to its Posted tab; `GET /api/news/drafts?posted=true` (same for `/episode/`) and the MCP `list_*_drafts` tools with `posted: true` return that list, and every list defaults to open drafts only. Drafts saved before this existed have no `posted` field and count as open. The News and Episodes pages' Posted tabs each have a "Remove All" button that clears that list in one call (`DELETE /api/news/drafts?posted=true`, same for `/episode/`); the same endpoint without `posted=true` clears the open drafts instead. Submitting from the form (`POST /api/news/submit`, `POST /api/episode/submit`) may carry a `draftId`: the submitted content is then written back onto that draft before it is marked posted, so the Posted entry records what actually went out rather than the last saved state. A submit without `draftId` — one that was never a draft — creates no entry.

### Post create/update API contract
Posts are submitted as `multipart/form-data` with two fields: `data` (JSON string of the post object) and `images` (zero or more binary files). This applies to both the REST API and the frontend `ApiClient.createPost`/`updatePost` methods.

### Upload lifecycle
Every uploaded image is stored twice — on disk in `UPLOAD_DIR` and as bytes in the `uploads` collection — and referenced by URL (`/api/uploads/<file>`) from whichever document owns it.

`handlers/uploads_cleanup.go` frees those files again. Deleting a post, a news or episode draft (single or bulk), a watermark, or a convention queue item or queue removes the images it held; updating one of those documents frees the images the update dropped, as does submitting a draft whose image changed. Nothing is deleted blindly: `cleanupUploads` first checks every collection/field that can hold an upload URL (`uploadReferences`) and keeps any file still pointed at, because the same URL is routinely shared — submitting a draft gives its image to the post it creates, and a consumed convention queue item hands its image to the post it spawns. Cleanup is best-effort and never fails the request it follows; a reference check that errors keeps the files.

**When a new model starts storing image URLs, add it to `uploadReferences`** — otherwise a delete elsewhere will free files that model still needs.

### Authorization model
Three roles with distinct Gin context keys:
- **Global admin** (`isAdmin=true`) — full access to `/api/admin/*`
- **Team admin** (`isTeamAdmin=true` + `teamId` set) — access to `/api/team/*`; can manage their own team's accounts and members
- **Regular user** — access to `/api/posts`, `/api/footers`, `/api/watermarks`, `/api/inbox`

Posts and footers are scoped: if the user has a `teamId`, queries filter by team; otherwise by `userId`.

### Unified token authentication
A single bearer token (`sm_...` prefix for users, `st_...` for teams) authenticates all external access: REST API calls (`/api/*`) and MCP requests (`/api/mcp`). Tokens are generated on the Profile page and passed as `Authorization: Bearer <token>`. Regenerating a token invalidates the previous one everywhere.

### Chrome extension (`chrome-extension/`)
A Manifest V3 Chrome extension that captures the current browser tab (screenshot + extracted page images) and sends the data to a SocialPod server's `/api/capture` endpoint to create a draft. Supports news and post draft types — episode drafts are created in the web UI only. Users configure their SocialPod server URL and API token in the extension popup. `make chrome-extension` produces a `socialpod-capture.zip` ready for Chrome Web Store upload.

Game titles in captured text always appear in plain double quotes (`"Wingspan"`) — house style across all published material. `gameNameQuotingRule` is appended to every capture system prompt, the team's `agentSystemPrompt` override included (`buildSystemPrompt`). For a BGG capture the exact title is known, so `enforceQuotedGameName` additionally quotes it in the generated text: it normalises typographic quotes, skips mentions inside a longer word or a URL, and is idempotent. Two fields are deliberately exempt: `gameNamePublisher` (a structured field the Episode form fills as `Name (Publisher)`) and a `newsTagline` that would exceed its 30-character chapter-title limit with the quotes added.

Captured images are normally narrowed down by a vision model (`selectBestImage`), but BoardGameGeek links bypass that entirely: a BGG game URL (`bggGameIDFromURL`) takes the cover from the BGG XML API — or, if that API is unavailable, from the game page's `og:image` — and runs it through `BGGHandler.downloadAndProcess`, the same letterbox + overlay pipeline as the UI's "import from BGG" button. The overlay matches what the corresponding form would use (`bggOverlayType`): the team's BGG watermark for news and post captures, the news episode overlay for episode captures. A BGG capture never falls back to the vision model for its image; if no cover can be fetched, the draft is created without one.

### n8n integration (`n8n-nodes-socialpod/`)
A pre-built n8n community node with a `dist/` directory already compiled. Install with `npm install --omit=dev` on the target machine — full `npm install` fails on Node < 22 due to the `isolated-vm` transitive dev dependency.

Supported resources: Post (create/get/list/update/delete/reschedule/retry), Footer (CRUD), Account (list), Mention (CRUD + export/import), Watermark (list/delete), AI Text (generate). All seven platforms are supported: Bluesky, Instagram, Twitter/X, Mastodon, Threads, LinkedIn, YouTube.

## Environment
Copy `.env.example` to `.env` and set at minimum `JWT_SECRET` (strong random string) and `MONGO_PASSWORD` before exposing the service. `APP_URL` must be publicly reachable for Instagram OAuth and webhooks to work.
