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

## Architecture

### Single-binary deployment
The React frontend is built and copied into `backend/cmd/server/dist/`, then embedded at compile time via `//go:embed dist/*` in `frontend.go`. The Go binary serves both the REST API (`/api/*`) and the SPA (all other routes fall back to `index.html`). A single port (default 8080) handles everything.

The build sequence: `npm run build` → `cp -r frontend/dist backend/cmd/server/dist` → `go build`.

### Backend (`backend/`)
Go module `socialmedia`, using Gin as the HTTP framework.

- `cmd/server/main.go` — wires all dependencies; defines four route groups: public `/api`, authenticated `/api` (JWT/API-token required), admin-only `/api/admin`, team-admin `/api/team`
- `internal/config/` — loads all config from environment variables
- `internal/database/mongo.go` — `MongoDB` wrapper that exposes typed collection accessors (`Posts()`, `Users()`, `Teams()`, etc.) and creates indexes on startup
- `internal/models/` — BSON-tagged Go structs for each MongoDB collection (`post.go`, `user.go`, `team.go`, `social_account.go`, `suffix.go`, `mention.go`, `convention.go`, `upload.go`, `watermark.go`, `team_invite.go`, `publisher_handle.go`)
- `internal/handlers/` — one file per handler group (`auth.go`, `posts.go`, `admin.go`, `inbox.go`, `suffixes.go`, `convention.go`, `mentions.go`, `invite.go`, `bgg.go`, `news.go`, `episode.go`, `publisher_handles.go`)
- `internal/middleware/auth.go` — `AuthRequired` tries three token types in order: JWT → user API token (`sm_...`) → team API token (`st_...`); sets `userId`, `isAdmin`, `isTeamAdmin`, `teamId` on the Gin context
- `internal/services/` — platform-specific: `bluesky.go`, `instagram.go`, `twitter.go`, `mastodon.go`, `threads.go`, `linkedin.go`, `youtube.go`; infrastructure: `scheduler.go`, `imageutil.go`, `email.go`

The scheduler (`services/Scheduler`) runs every 30 seconds, queries for posts where `status == "scheduled"` and `scheduledAt <= now`, and publishes them. Suffixes are fetched from the DB and appended at publish time (not stored on the post itself).

Convention queues have their own 30-second background loop (`ConventionHandler.StartAutoPoster` in `handlers/convention.go`). Approved queue items are a *set*, not pre-scheduled: whenever a queue is active and inside its date window and its `nextPostAt` is due, the loop picks one approved item at random, creates a `scheduled` post for it (which the shared scheduler then publishes), marks the item consumed, and rolls `nextPostAt` forward by the schedule gap. `POST /convention/queues/:id/schedule` is a manual "post one random item now" trigger.

### Frontend (`frontend/`)
React 19 + TypeScript + Vite. No state management library — auth state lives in `AuthContext`, everything else is local component state fetched via the `ApiClient`.

- `src/services/api.ts` — single `ApiClient` class that wraps all `fetch` calls; token stored in `localStorage`; redirects to `/login` on 401
- `src/contexts/AuthContext.tsx` — global auth state (`user`, `loading`, `login`, `logout`, `refreshUser`)
- `src/types/index.ts` — all shared TypeScript types (`Post`, `User`, `Team`, `SocialAccount`, etc.)
- `src/App.tsx` — route definitions; `ProtectedRoute` enforces `adminOnly`/`teamAdminOnly` flags

Components are organized by feature under `src/components/` (Calendar, PostEditor, Admin, Inbox, Suffixes, etc.).

### Post create/update API contract
Posts are submitted as `multipart/form-data` with two fields: `data` (JSON string of the post object) and `images` (zero or more binary files). This applies to both the REST API and the frontend `ApiClient.createPost`/`updatePost` methods.

### Authorization model
Three roles with distinct Gin context keys:
- **Global admin** (`isAdmin=true`) — full access to `/api/admin/*`
- **Team admin** (`isTeamAdmin=true` + `teamId` set) — access to `/api/team/*`; can manage their own team's accounts and members
- **Regular user** — access to `/api/posts`, `/api/suffixes`, `/api/watermarks`, `/api/inbox`

Posts and suffixes are scoped: if the user has a `teamId`, queries filter by team; otherwise by `userId`.

### n8n integration (`n8n-nodes-socialpod/`)
A pre-built n8n community node with a `dist/` directory already compiled. Install with `npm install --omit=dev` on the target machine — full `npm install` fails on Node < 22 due to the `isolated-vm` transitive dev dependency.

Supported resources: Post (create/get/list/update/delete/reschedule/retry), Suffix (CRUD), Account (list), Mention (CRUD + export/import), Watermark (list/delete), AI Text (generate). All seven platforms are supported: Bluesky, Instagram, Twitter/X, Mastodon, Threads, LinkedIn, YouTube.

## Environment
Copy `.env.example` to `.env` and set at minimum `JWT_SECRET` (strong random string) and `MONGO_PASSWORD` before exposing the service. `APP_URL` must be publicly reachable for Instagram OAuth and webhooks to work.
