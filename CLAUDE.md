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
- `internal/models/` — BSON-tagged Go structs for each MongoDB collection (`post.go`, `user.go`, `team.go`, `social_account.go`, `footer.go`, `mention.go`, `convention.go`, `upload.go`, `watermark.go`, `team_invite.go`, `publisher_handle.go`, `news_draft.go`, `episode_draft.go`, `oauth.go`, `growth.go`)
- `internal/handlers/` — one file per handler group (`auth.go`, `posts.go`, `admin.go`, `inbox.go`, `footers.go`, `convention.go`, `mentions.go`, `invite.go`, `bgg.go`, `news.go`, `episode.go`, `publisher_handles.go`, `mcp.go`, `mcp_growth.go`, `oauth.go`, `capture.go`, `growth.go`, `growth_stats.go`, `growth_ai.go`, `radar.go`)
- `internal/middleware/auth.go` — `AuthRequired` tries three token types in order: JWT → user API token (`sm_...`) → team API token (`st_...`); sets `userId`, `isAdmin`, `isTeamAdmin`, `teamId` on the Gin context. The same bearer token authenticates both REST API and MCP requests. `MCPAuthRequired` guards `/api/mcp`: it additionally accepts OAuth access tokens (`spa_...`, `middleware/oauth.go`) and drops admin flags.
- `internal/services/` — platform-specific: `bluesky.go`, `instagram.go`, `instagram_insights.go`, `twitter.go`, `mastodon.go`, `threads.go`, `linkedin.go`, `youtube.go`; infrastructure: `scheduler.go`, `growth_jobs.go`, `imageutil.go`, `radar_render.go`, `email.go`

The scheduler (`services/Scheduler`) runs every 30 seconds, queries for posts where `status == "scheduled"` and `scheduledAt <= now`, and publishes them. Footers are fetched from the DB and appended at publish time (not stored on the post itself).

Convention queues have their own 30-second background loop (`ConventionHandler.StartAutoPoster` in `handlers/convention.go`). Approved queue items are a *set*, not pre-scheduled: whenever a queue is active and inside its date window and its `nextPostAt` is due, the loop picks one approved item at random, creates a `scheduled` post for it (which the shared scheduler then publishes), marks the item consumed, and rolls `nextPostAt` forward by the schedule gap. `POST /convention/queues/:id/schedule` is a manual "post one random item now" trigger.

A queue's watermark (`watermarkId`) is composited onto each image by `postItem` at post time, never stored on the item. The queue page's crop button (next to the edit pencil on each item card) therefore opens the shared `ImageCropper` with `bakeWatermark={false}`: the overlay is drawn over the crop area so the framing can be judged, but the uploaded crop is clean, or the overlay would be stamped twice. Crop and edit both act on the gallery image currently shown; `PUT /convention/queues/:id/items/:iid/image` takes an `index` form field for that, keeps `imageUrl` mirroring `imageUrls[0]`, refuses scheduled items, and frees the replaced upload.

### Instagram growth (`handlers/growth*.go`, `handlers/radar.go`, `services/growth_jobs.go`)
Tools for growing an Instagram account against a follower goal. The Instagram login asks for `instagram_business_manage_insights` (`instagramAuthURL` in `admin.go`); accounts connected before that need a reconnect for Insights.

- **Follower snapshots** — the scheduler's growth loop (hourly, separate from the 30-second publishing loop) upserts one `follower_snapshots` document per Instagram account and UTC day (`followers_count` needs only `instagram_business_basic`).
- **Post Insights** — the same loop reads reach, views, shares, saves, likes, comments, follows and profile visits for Instagram posts published in the last 60 days and stores them on the post's Instagram result (`results.$.insights`), more often while a post is young. Instagram rejects a whole request when one metric does not apply to the media type, so feed posts, reels and stories ask for different sets (`instagram_insights.go`). A failed read keeps the old numbers and records the error.
- **Team settings** — `Team.Growth` (`models/growth.go`): the follower goal (target, start/end, optional start-count override, halfway review date + threshold, default midway and 40%), the weekly plan (recurring slots: weekday 0 = Monday, optional `HH:MM`, format reel/carousel/image/story/any, label), the single-image warning limit, the "Rewrite for Instagram" prompt, and the Radar layout. `GET /api/growth/settings` (any member), `PUT /api/team/growth` (team admin; frontend page `/admin/growth`).
- **Growth page** (`/growth`, `GET /api/growth/instagram?accountId=&start=&end=`) — goal tracker (`computeGoalProgress`: weekly net vs. required, halfway status `pending`/`ok`/`below`), the weekly report for the week the client sends (`buildWeeklyReport`: top 3 by shares + saves, per-format sends/saves per reach, weakest format) and ad candidates (`markAdCandidates`: reels and carousels of the last 30 days in the top 20% by shares + saves). `POST /api/growth/refresh` re-reads followers and the last 30 days of Insights; `POST /api/posts/:id/insights` one post. The pure computations live in `growth_stats.go` and are unit-tested.
- **Dashboard AI insights** append `instagramPerformanceSummary` (real numbers, goal progress) to the posting summary.
- **Calendar** — open weekly-plan slots render as dashed placeholders on today and future days (clicking one opens the editor at the slot's time and type), and each week gets a format-mix line with a warning above the single-image limit (`services/instagramGrowth.ts`). Both hide while a calendar filter is on. The calendar shows ISO week numbers (the ones the goal tracker uses): a `W<n>` tag on each Monday of the month grid, and in the title of the week and 3-day views.
- **PostEditor** — Instagram collaborators (max 3, pre-filled from the BGG publishers' Instagram handles, `publisherInstagramHandles`) and image tags (the @mentions of the Instagram caption, editable) are stored as `collaborators` / `userTags` on the post and sent as `collaborators` / `user_tags` when the container is created; tags go on a carousel's first image. Instagram refuses the whole container for an unknown username, so `createContainerWithExtras` retries without them and the result carries a `warning`. A caption checker (hook, closing question, share prompt, 3–5 hashtags, a tagged account, not identical to the Bluesky text) and "Rewrite for Instagram" (`POST /api/growth/rewrite-instagram`, team prompt or `DefaultInstagramRewritePrompt`) sit under the Instagram text. Published posts show their Insights.
- **Radar der Woche** — news drafts carry `radar` (the "Radar Post?" checkbox, which queues them) and `germanRelease` (`ja`/`nein`/`offen`, the slide badge). `GET /api/radar/queue` lists queued drafts (open and posted alike); `POST /api/radar/generate {draftIds}` (1–8) renders a 4:5 carousel — cover, one slide per news, closing — with Go fonts (`services/radar_render.go`), stores the slides as uploads and returns them with a caption and the handles to tag. The editor's "Radar Content" button puts that into the post (Instagram only, replacing text and images); nothing is posted automatically. Saving a post with `radarDraftIds` sets `radarPostId` on those drafts, which takes them out of the queue; deleting an unpublished post puts them back. The News page's Carousels tab lists the scheduled posts that carry Radar slides (`radarDraftIds`), with their slides and news titles; removing one deletes the post, which returns its news to the queue. A news item submitted with "Radar Post?" but without a draft gets a posted draft entry so it can reach the queue.

### MCP server (`handlers/mcp.go`)
An MCP (Model Context Protocol) server is available at `POST /api/mcp` using the Streamable HTTP transport. It exposes all user-level operations as MCP tools: posts CRUD, footers, mentions, watermarks, accounts, profile, news drafts, and episode drafts. Authentication uses the same bearer token (`sm_...`) as the REST API — one token works for both interfaces.

MCP clients (Claude Code, OpenCode, etc.) configure the server URL and an `Authorization: Bearer <token>` header — or sign in with OAuth instead (below).

The MCP endpoint has its own middleware, `middleware.MCPAuthRequired`, not `AuthRequired`. It accepts JWTs, API tokens and OAuth access tokens, and **always clears `isAdmin`/`isTeamAdmin`**: MCP exposes user features only, even for an admin (`get_profile` reports both as false). Never add an admin or team-admin tool to it. An unauthenticated request gets a 401 with `WWW-Authenticate: Bearer resource_metadata="…"`, which is how OAuth clients discover the authorization server.

#### OAuth / OpenID Connect (`handlers/oauth.go`)
Remote MCP clients that can't take a pasted token — Claude.ai custom connectors (and the Claude desktop/mobile apps, which share them), ChatGPT connectors, Claude Code's `/mcp` sign-in — use the built-in OAuth 2.1 authorization server, following the MCP authorization spec:

- Discovery: `/.well-known/oauth-protected-resource` (RFC 9728), `/.well-known/oauth-authorization-server` (RFC 8414) and `/.well-known/openid-configuration`, each also under the `/api/mcp` path suffix. The issuer is `APP_URL`, which must therefore be the public HTTPS URL for Claude.ai or ChatGPT to connect — their servers make the calls.
- Dynamic client registration at `POST /api/oauth/register` (RFC 7591). Redirect URIs must be https, http on loopback (any port matches, RFC 8252), or a native app's private-use scheme.
- Authorization code + mandatory PKCE S256. The authorization endpoint is the SPA page `/oauth/authorize` (`OAuthConsentPage`): it sends a signed-out user through `/login?next=…`, then calls `GET /api/oauth/authorize` to validate the request and `POST /api/oauth/authorize` to approve or deny, and follows the returned `redirectUrl`. An unknown client or unregistered redirect URI is shown as an error and never redirected to.
- `POST /api/oauth/token` (authorization_code, refresh_token with rotation), `POST /api/oauth/revoke`, `GET /api/oauth/userinfo`, `GET /api/oauth/jwks`. With the `openid` scope the token response carries an RS256 ID token; the signing key is generated on first use and stored in `oauth_keys`.
- Tokens are opaque (`spa_` access, 1 h; `spr_` refresh, 90 days), stored only as SHA-256 hashes in `oauth_tokens`, with codes in `oauth_codes`; TTL indexes expire both. **OAuth access tokens are accepted by `/api/mcp` and userinfo only** — `AuthRequired` does not know them, so the REST API rejects them.
- The Profile page lists connected apps (`GET /api/oauth/grants`) and disconnects one (`DELETE /api/oauth/grants/:clientId`, deletes all its tokens). Regenerating the API token does not affect OAuth connections. The MCP handler implements the JSON-RPC 2.0 protocol with `initialize`, `ping`, `tools/list`, and `tools/call` methods. Each tool performs the same database queries as the corresponding REST handler, respecting team/user scoping.

MCP growth tools (`mcp_growth.go`): `get_post_insights` (one post, or every Instagram post of the last N days, with sends/saves per reach and the ad-candidate flag; `refresh` reads Instagram first) and `get_account_insights` (followers, goal tracker, weekly report and account totals for a range). `create_post`/`update_post` take `collaborators` and `userTags`; the news draft tools take `radar` and `germanRelease`.

MCP draft tools: `create_news_draft`, `list_news_drafts`, `get_news_draft`, `update_news_draft`, `delete_news_draft`, `post_news_draft`, `create_episode_draft`, `list_episode_drafts`, `get_episode_draft`, `update_episode_draft`, `delete_episode_draft`, `post_episode_draft`. Post drafts (post/story/reel) are regular posts created with `status: "draft"` via `create_post`.

MCP image uploads: the `upload_image` tool accepts base64-encoded image data and a filename, stores the file (disk + MongoDB), and returns its URL. All create/update tools (`create_post`, `update_post`, `create_news_draft`, `update_news_draft`, `create_episode_draft`, `update_episode_draft`) also accept an `images` parameter — an array of `{data, filename}` objects with base64-encoded data — for inline image upload in a single tool call. Uploaded image URLs are merged with any `imageUrls` passed in the same call. Supported types: jpg, jpeg, png, gif, webp, mp4, mov.

### Frontend (`frontend/`)
React 19 + TypeScript + Vite. No state management library — auth state lives in `AuthContext`, everything else is local component state fetched via the `ApiClient`.

- `src/services/api.ts` — single `ApiClient` class that wraps all `fetch` calls; token stored in `localStorage`; redirects to `/login` on 401
- `src/contexts/AuthContext.tsx` — global auth state (`user`, `loading`, `login`, `logout`, `refreshUser`)
- `src/types/index.ts` — all shared TypeScript types (`Post`, `User`, `Team`, `SocialAccount`, etc.)
- `src/App.tsx` — route definitions; `ProtectedRoute` enforces `adminOnly`/`teamAdminOnly` flags
- `src/components/Common/Modal.tsx` — the shared overlay + panel every dialog renders through; write a new dialog with this, not a hand-rolled `.modal-overlay`. It owns backdrop dismissal: a click on the backdrop asks before discarding whenever the user has entered something, so an accidental click outside cannot wipe a half-filled form. Dirtiness comes from native input/change events inside the panel, which fire only for real typing and picking — React's own state updates never trip it, so a dialog whose fields are filled in asynchronously does not look edited. Pass `dirty` to override that for a dialog holding state the DOM does not show, or `closeOnBackdrop={false}` to require its own buttons. A form whose fields are buttons rather than inputs must pass `dirty`, or a backdrop click discards it without asking. The convention queue form (`QueueFormModal`, used for both New and Edit) goes further: it passes `closeOnBackdrop={false}` so a click outside never dismisses it, and confirms in its own `handleClose` behind Cancel and the X — most of that form is toggles and chips the Modal's watch cannot see. The PostEditor is the one dialog that still builds its own overlay: that overlay also hosts the image editor and cropper as siblings of the panel, which `Modal` has no place for, and it tracks `isDirty` and confirms in `handleClose` itself.
- `src/components/Common/ErrorBoundary.tsx` — wraps the routed content in `App.tsx`. Without it React unmounts the whole tree on any render error and leaves a blank page with nothing to act on.

Components are organized by feature under `src/components/` (Calendar, PostEditor, Admin, Inbox, Footers, News, Episode, etc.).

### Draft system
Three draft types exist across the application:

- **Post/Story/Reel drafts** — regular `Post` documents with `status: "draft"`. Created through the PostEditor (Calendar page). The Calendar page has a "Drafts" tab that lists all draft posts across types, with edit/schedule/delete actions. MCP: use `create_post` with `status: "draft"`.
- **News drafts** — stored in the `news_drafts` collection (`models/news_draft.go`). Managed through the News page's Create/Drafts tabs. REST: `GET/POST /api/news/drafts`, `GET/PUT/DELETE /api/news/drafts/:id`, `POST /api/news/drafts/:id/post`, `DELETE /api/news/drafts?posted=true` (bulk). MCP: `create_news_draft`, `list_news_drafts`, `get_news_draft`, `update_news_draft`, `delete_news_draft`, `post_news_draft`.
- **Episode drafts** — stored in the `episode_drafts` collection (`models/episode_draft.go`). Managed through the Episodes page's Create/Drafts tabs. REST: `GET/POST /api/episode/drafts`, `GET/PUT/DELETE /api/episode/drafts/:id`, `POST /api/episode/drafts/:id/post`, `DELETE /api/episode/drafts?posted=true` (bulk). MCP: `create_episode_draft`, `list_episode_drafts`, `get_episode_draft`, `update_episode_draft`, `delete_episode_draft`, `post_episode_draft`.

News and episode drafts follow the same lifecycle: create → list/browse → edit → publish (sends to webhook + optionally creates a social post + marks the draft `posted`). Drafts are submitted as `multipart/form-data` with `data` (JSON) and optional `images` fields.

Publishing never deletes a draft. It sets `posted: true` and `postedAt`, which moves the entry from the News/Episodes page's Drafts tab to its Posted tab; `GET /api/news/drafts?posted=true` (same for `/episode/`) and the MCP `list_*_drafts` tools with `posted: true` return that list, and every list defaults to open drafts only. Drafts saved before this existed have no `posted` field and count as open. The News and Episodes pages' Posted tabs each have a "Remove All" button that clears that list in one call (`DELETE /api/news/drafts?posted=true`, same for `/episode/`); the same endpoint without `posted=true` clears the open drafts instead. Submitting from the form (`POST /api/news/submit`, `POST /api/episode/submit`) may carry a `draftId`: the submitted content is then written back onto that draft before it is marked posted, so the Posted entry records what actually went out rather than the last saved state. A submit without `draftId` — one that was never a draft — creates no entry, unless it is marked "Radar Post?": then it is recorded as a posted entry so it can reach the Radar queue.

The News and Episode forms hold a single image, and the backend keeps the `imageUrls` it is given alongside any uploaded file. A local image — a fresh pick, or a crop of the draft's own stored image, which has no local file behind it — is therefore sent with an empty `imageUrls` so it replaces what was stored rather than being appended to it. After a draft save the form adopts the URLs the server returns, so the next save neither re-uploads the same crop nor works from an image cleanup has already freed.

### Post create/update API contract
Posts are submitted as `multipart/form-data` with two fields: `data` (JSON string of the post object) and `images` (zero or more binary files). This applies to both the REST API and the frontend `ApiClient.createPost`/`updatePost` methods.

The PostEditor takes images three ways — the file picker, a drop on the image area, and a clipboard paste — all funnelled through `addFiles`, which drops whatever the post type can't use (a post takes images, a story either, a reel video). Like the News form, the empty image area is itself the drop target and says so. A pasted or dropped image opens the cropper right away; a picked one only when it isn't square already. The cropper carries the same Overlay picker as the Episode form — the team's watermarks plus None — but unlike News and Episode, which preselect the team's configured creator watermark, it opens on None every time, so an overlay on a post image is always a deliberate pick. Picking one draws it over the crop area and bakes it into the output; picking None takes it off again.

### Upload lifecycle
Every uploaded image is stored twice — on disk in `UPLOAD_DIR` and as bytes in the `uploads` collection — and referenced by URL (`/api/uploads/<file>`) from whichever document owns it.

`handlers/uploads_cleanup.go` frees those files again. Deleting a post, a news or episode draft (single or bulk), a watermark, or a convention queue item or queue removes the images it held; updating one of those documents frees the images the update dropped, as does submitting a draft whose image changed. Nothing is deleted blindly: `cleanupUploads` first checks every collection/field that can hold an upload URL (`uploadReferences`) and keeps any file still pointed at, because the same URL is routinely shared — submitting a draft gives its image to the post it creates, and a consumed convention queue item hands its image to the post it spawns. Cleanup is best-effort and never fails the request it follows; a reference check that errors keeps the files.

Automatic cleanup only covers uploads a document owned. Two kinds are left over: the backlog from before cleanup existed, and imports abandoned before anything referenced them (`POST /api/upload`, `/api/upload-from-url` and captures store a file immediately, minutes before the post or draft that will reference it exists). `handlers/uploads_sweep.go` clears both — an admin-only scan/sweep over the whole store, reachable from the Storage card on the admin Settings page:

- `GET /api/admin/uploads/orphans` — scan, delete nothing
- `POST /api/admin/uploads/sweep` — scan, delete nothing
- `POST /api/admin/uploads/sweep?delete=true` — delete what it finds

Both take `minAgeHours` (default 24), the age guard that spares uploads an open editor has stored but not yet referenced. The sweep reads both halves of the store — the `uploads` collection and `UPLOAD_DIR` — so a record whose file is missing, or a file with no record, is found too, and reports totals, orphan counts and bytes, and a sample. A failed reference scan aborts it rather than treating everything as unreferenced.

The Radar cover and closing slides a team uploads are referenced from `teams.growth.radar.coverImageUrl` / `closingImageUrl`; generated Radar slides are owned by the post that carries them.

**When a new model starts storing image URLs, add it to `uploadReferences`** — otherwise a delete elsewhere, or a sweep, will free files that model still needs.

### Authorization model
Three roles with distinct Gin context keys:
- **Global admin** (`isAdmin=true`) — full access to `/api/admin/*`
- **Team admin** (`isTeamAdmin=true` + `teamId` set) — access to `/api/team/*`; can manage their own team's accounts and members
- **Regular user** — access to `/api/posts`, `/api/footers`, `/api/watermarks`, `/api/inbox`

Posts and footers are scoped: if the user has a `teamId`, queries filter by team; otherwise by `userId`.

### Unified token authentication
A single bearer token (`sm_...` prefix for users, `st_...` for teams) authenticates all external access: REST API calls (`/api/*`) and MCP requests (`/api/mcp`). OAuth-connected MCP clients use their own tokens instead (see MCP server above), which work for `/api/mcp` only. Tokens are generated on the Profile page and passed as `Authorization: Bearer <token>`. Regenerating a token invalidates the previous one everywhere.

### Chrome extension (`chrome-extension/`)
A Manifest V3 Chrome extension that captures the current browser tab (screenshot + extracted page images) and sends the data to a SocialPod server's `/api/capture` endpoint to create a draft. Supports news and post draft types — episode drafts are created in the web UI only. Users configure their SocialPod server URL and API token in the extension popup. `make chrome-extension` produces a `socialpod-capture.zip` ready for Chrome Web Store upload.

Game titles in captured text always appear in plain double quotes (`"Wingspan"`) — house style across all published material. `gameNameQuotingRule` is appended to every capture system prompt, the team's `agentSystemPrompt` override included (`buildSystemPrompt`). For a BGG capture the exact title is known, so `enforceQuotedGameName` additionally quotes it in the generated text: it normalises typographic quotes, skips mentions inside a longer word or a URL, and is idempotent. Two fields are deliberately exempt: `gameNamePublisher` (a structured field the Episode form fills as `Name (Publisher)`) and a `newsTagline` that would exceed its 30-character chapter-title limit with the quotes added.

Captured images are normally narrowed down by a vision model (`selectBestImage`), but BoardGameGeek links bypass that entirely: a BGG game URL (`bggGameIDFromURL`) takes the cover from the BGG XML API — or, if that API is unavailable, from the game page's `og:image` — and runs it through `BGGHandler.downloadAndProcess`, the same letterbox + overlay pipeline as the UI's "import from BGG" button. The overlay matches what the corresponding form would use (`bggOverlayType`): the team's BGG watermark for news and post captures, the news episode overlay for episode captures. A BGG capture never falls back to the vision model for its image; if no cover can be fetched, the draft is created without one.

### n8n integration (`n8n-nodes-socialpod/`)
A pre-built n8n community node with a `dist/` directory already compiled. Install with `npm install --omit=dev` on the target machine — full `npm install` fails on Node < 22 due to the `isolated-vm` transitive dev dependency.

Supported resources: Post (create/get/list/update/delete/reschedule/retry), Footer (CRUD), Account (list), Mention (CRUD + export/import), Watermark (list/delete), AI Text (generate). All seven platforms are supported: Bluesky, Instagram, Twitter/X, Mastodon, Threads, LinkedIn, YouTube.

## Environment
Copy `.env.example` to `.env` and set at minimum `JWT_SECRET` (strong random string) and `MONGO_PASSWORD` before exposing the service. `APP_URL` must be publicly reachable for Instagram OAuth and webhooks to work.
