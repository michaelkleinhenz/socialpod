# SocialPod — Social Media Scheduler

A self-hosted social media scheduling platform for **Bluesky** and **Instagram**. Features a drag-and-drop calendar UI, multitenancy, image uploads, a built-in image editor with watermarks, per-platform text customization, an Instagram inbox (comments & DMs), suffix management, a background post publisher, and a REST API for external integrations including a native **n8n node**.

## Quick Start

```bash
# 1. Clone and configure
cp .env.example .env
# Edit .env — at minimum change JWT_SECRET

# 2. Build and start
make up

# 3. Open http://localhost:8080
#    The first registered user becomes the admin.
```

## Prerequisites

- **Docker** and **Docker Compose** (v2)
- For Instagram: a Meta developer account with an Instagram app (see below)
- For Bluesky: an app password generated from your Bluesky account settings

## Configuration

All configuration is done via environment variables in `.env`:

| Variable | Default | Description |
|---|---|---|
| `MONGO_USER` | `socialmedia` | MongoDB root username |
| `MONGO_PASSWORD` | `socialmedia_secret` | MongoDB root password |
| `MONGO_DATABASE` | `socialmedia` | MongoDB database name |
| `JWT_SECRET` | `change-me-in-production` | Secret for signing JWT tokens — **change this** |
| `APP_URL` | `http://localhost:8080` | Public URL of the app (used for Instagram OAuth) |
| `APP_PORT` | `8080` | Port the app listens on |
| `UPLOAD_DIR` | `./uploads` | Directory for uploaded image/video files |

### Production Deployment

For a publicly accessible deployment:

1. Set `APP_URL` to your public domain (e.g. `https://socialpod.example.com`).
2. Generate a strong `JWT_SECRET` (e.g. `openssl rand -hex 32`).
3. Change `MONGO_PASSWORD` to a strong password.
4. Place a reverse proxy (Caddy, Traefik, etc.) in front for TLS.

The app ships as a single binary that serves both the API and the frontend UI on the same port.

---

## Setting Up Bluesky Authentication

Bluesky uses **app passwords** for authentication — no OAuth flow required.

### Step 1: Generate an App Password

1. Log in to [bsky.app](https://bsky.app).
2. Go to **Settings → Privacy and Security → App passwords**.
3. Click **Add App Password**, give it a name (e.g. "SocialPod"), and click **Create**.
4. Copy the generated password (format: `xxxx-xxxx-xxxx-xxxx`). You will not be able to see it again.

### Step 2: Add the Account in SocialPod

1. Log in to SocialPod as an admin.
2. Navigate to **Accounts** in the sidebar.
3. Click **Add Bluesky**.
4. Enter:
   - **Handle**: Your Bluesky handle (e.g. `yourname.bsky.social`)
   - **App Password**: The password you generated above
   - **PDS Host** (optional): Leave blank for the default `https://bsky.social`. Only change this if you use a custom PDS.
5. Click **Add Account**.

The account is now active and ready for scheduling posts.

### Bluesky Post Features

- Text posts up to 300 characters
- Up to 4 images per post
- Automatic hashtag detection and richtext facets
- Custom PDS support for self-hosted instances

---

## Setting Up Instagram Authentication

SocialPod uses the **Instagram Standalone** (Basic Display / Business) OAuth flow. This requires a Meta developer app.

### Step 1: Create an Instagram App on Meta

1. Go to [developers.facebook.com](https://developers.facebook.com/) and log in.
2. Click **My Apps → Create App**.
3. Select **Other** as the use case, then **Consumer** as the app type.
4. Name your app and click **Create**.
5. In the app dashboard, find **Instagram** (under "Add products") and click **Set Up**.

### Step 2: Configure Instagram Basic Display

1. In your Meta app dashboard, go to **Instagram → Basic Display**.
2. Click **Create New App** if prompted.
3. Under **Valid OAuth Redirect URIs**, add:
   ```
   https://your-domain.com/api/auth/instagram/callback
   ```
   Replace `your-domain.com` with your `APP_URL`. For local development use:
   ```
   https://localhost:3000/api/auth/instagram/callback
   ```
   > **Note**: Instagram requires HTTPS for redirect URIs, even in development. You may need a tunneling tool (e.g. ngrok) for local testing.
4. Under **Deauthorize Callback URL** and **Data Deletion Request URL**, you can use placeholder URLs for now.
5. Save your changes.

### Step 3: Note Your App Credentials

From your Meta app dashboard → **Instagram → Basic Display**:
- **Instagram App ID** (also shown under Settings → Basic)
- **Instagram App Secret** (click "Show" to reveal)

### Step 4: Configure SocialPod

1. Log in to SocialPod as an admin.
2. Go to **Settings** in the sidebar.
3. Enter:
   - **Application URL**: Your public URL (must match what you configured in Meta — e.g. `https://your-domain.com`)
   - **Instagram App ID**: From step 3
   - **Instagram App Secret**: From step 3
4. Click **Save Settings**.

### Step 5: Connect an Instagram Account

1. Go to **Accounts** in the sidebar.
2. Click **Connect Instagram**.
3. You will be redirected to Instagram's authorization page.
4. Log in with the Instagram account you want to post from and click **Authorize**.
5. You will be redirected back to SocialPod. The account now appears in the accounts list.

### Instagram Post Requirements

SocialPod supports three Instagram content types, selected via the `postType` field:

| Post Type | Media Required | Description |
|---|---|---|
| `post` (default) | 1–10 images | Regular feed post or carousel |
| `story` | 1 image or video | Instagram Story (disappears after 24h) |
| `reel` | 1 video (MP4) | Instagram Reel — appears in the Reels tab |

Additional requirements:
- Instagram **requires at least one media file** per post — text-only posts are not supported.
- Captions up to 2,200 characters (not applicable to Stories).
- Media files must be publicly accessible (the `APP_URL` must be reachable from Instagram's servers).
- The long-lived access token expires after ~60 days. You will need to reconnect the account when it expires.

### Instagram Scopes Requested

SocialPod requests the following permissions during OAuth:
- `instagram_business_basic` — Read profile info
- `instagram_business_content_publish` — Create and publish posts
- `instagram_business_manage_messages` — Manage messages

---

## Suffix Management

Suffixes are snippets of text that are **automatically appended** to a post when it is published, without counting against the character limit visible in the editor.

### Creating Suffixes

1. Click **Suffixes** in the sidebar (available to all users).
2. Click **New Suffix**.
3. Give it a name (e.g. "Website footer") and enter the content (e.g. `🌐 mysite.com`).
4. Click **Create**.

### Using Suffixes in Posts

When composing a post, suffix dropdowns appear for each selected platform:
- **Bluesky suffix** — appended only when publishing to Bluesky
- **Instagram suffix** — appended only when publishing to Instagram

The character counter in the editor automatically deducts the length of the selected suffix so you always see accurate remaining characters.

Suffixes are stored by reference on the post. If you update a suffix, all future posts using it will pick up the new text.

---

## Built-in Image Editor

Every post image can be edited directly in the browser using the built-in image editor (powered by [Filerobot Image Editor](https://github.com/scaleflex/filerobot-image-editor)). Click the pencil icon on any attached image to open it.

### Available Tools

| Tab | Description |
|---|---|
| **Adjust** | Brightness, contrast, saturation, exposure, and more |
| **Annotate** | Text overlays, shapes, arrows, and freehand drawing |
| **Watermark** | Place admin-uploaded watermark images on the photo |
| **Filters** | Apply preset color filters |
| **Finetune** | Fine-grained colour and tone adjustments |
| **Resize** | Change image dimensions |

### Crop Presets

The editor includes presets for common social media aspect ratios:

| Preset | Ratio |
|---|---|
| Square | 1:1 |
| Story | 9:16 |
| Landscape | 16:9 |
| Portrait | 4:5 |

### Custom Fonts

Text annotations support a selection of web-safe fonts plus **Rockwell** (with serif fallbacks) for a classic look.

---

## Watermarks

Admins can manage a library of watermark images that users can apply inside the image editor.

### Managing Watermarks (Admin)

1. Log in as an admin and navigate to **Watermarks** in the sidebar.
2. Optionally enter a name, then click **Upload** and select an image (PNG, JPEG, GIF, or WebP).
3. The watermark appears in the grid and is immediately available to all users in the image editor.
4. Click the trash icon on any watermark to remove it.

### Applying Watermarks (Users)

1. Open a post with an attached image and click the pencil (edit) icon.
2. Select the **Watermark** tab in the editor toolbar.
3. Click a watermark from the gallery to place it on the image.
4. Drag, resize, or reposition it, then click **Save**.

---

## Per-Platform Text Customization

When posting to both Bluesky and Instagram at the same time, you can write different text for each platform rather than using one shared caption.

### Using Per-Platform Text

1. In the post editor, select both **Bluesky** and **Instagram** as platforms.
2. Toggle **Customize per platform** (above the text area).
3. A separate text field appears for each selected platform.
4. Leave a platform field empty to fall back to the shared text below.

The character counter enforces each platform's limit independently (300 for Bluesky, 2,200 for Instagram).

---

## AI Text Generation

When an OpenRouter API key is configured in **Settings**, a magic-wand button appears in the post editor. Click it to rewrite or improve the current post text using an AI model.

### Enabling AI Text Generation

1. Log in as an admin and go to **Settings → AI / OpenRouter**.
2. Enter your OpenRouter API key and select a model.
3. Save settings. The wand button becomes available to all users immediately.

---

## Instagram Inbox

SocialPod can receive Instagram comments and direct messages via Meta webhooks and display them in a unified inbox.

### Prerequisites

- Your SocialPod `APP_URL` must be publicly accessible (Meta requires HTTPS for webhook delivery).
- An Instagram account connected in **Accounts**.
- Meta webhook subscription configured (see below).

### Configuring Meta Webhooks

1. In your Meta app dashboard, go to **Webhooks**.
2. Add a new subscription for the **Instagram** object.
3. Set the **Callback URL** to:
   ```
   https://your-domain.com/api/webhooks/instagram
   ```
4. Set the **Verify Token** to any string — no configuration needed on the SocialPod side.
5. Subscribe to the `comments` and `messages` fields.

### Using the Inbox

| Page | Description |
|---|---|
| **Comments** | Instagram comments on your posts, with unread counts and reply support |
| **Direct Messages** | Instagram DMs, with unread tracking and reply support |
| **Feed** | Your Instagram account's own post feed |

- Click **Reply** on any comment or DM to send a response directly from SocialPod.
- Messages are marked as read when opened.
- Click the refresh button to manually fetch the latest messages.

---

## Multitenancy

- The **first user** to register becomes the **admin**.
- Subsequent users are regular users who can create and manage their own posts.
- Admins have access to:
  - Dashboard with post statistics
  - Social account management (Bluesky & Instagram)
  - User management
  - Application settings
- Each user's posts and suffixes are isolated — users can only see and edit their own.
- **Teams**: admins can create teams and assign users. Team members share posts and suffixes scoped to the team.

---

## REST API

SocialPod exposes a REST API for external scheduling. Authenticate with either a JWT token (from login) or an API token.

### Generate an API Token

1. Go to **Profile** (click your name in the sidebar).
2. Click **Generate API Token**.
3. Copy the token (format: `sm_...`).

Team tokens (format: `st_...`) can be generated from the admin **Teams** page.

### Authentication

Include the token in the `Authorization` header:

```
Authorization: Bearer sm_your_api_token_here
```

### Endpoints

#### Posts

```bash
# Create a scheduled post (multipart/form-data — post data in `data` JSON field)
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"Hello from the API!","platforms":["bluesky"],"scheduledAt":"2025-06-01T09:00:00Z","status":"scheduled"}'

# Create a post with images (attach files as `images` fields)
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"Photo post","platforms":["instagram"],"scheduledAt":"2025-06-01T09:00:00Z","status":"scheduled"}' \
  -F "images=@photo.jpg"

# Create an Instagram Story (postType="story", requires one image or video)
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"","platforms":["instagram"],"postType":"story","scheduledAt":"2025-06-01T09:00:00Z","status":"scheduled"}' \
  -F "images=@story.jpg"

# Create an Instagram Reel (postType="reel", requires one MP4 video)
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"Check out this reel!","platforms":["instagram"],"postType":"reel","scheduledAt":"2025-06-01T09:00:00Z","status":"scheduled"}' \
  -F "images=@reel.mp4"

# Create a post with a suffix
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"Check this out","platforms":["bluesky","instagram"],"scheduledAt":"2025-06-01T09:00:00Z","status":"scheduled","suffixIds":{"bluesky":"<suffix-id>","instagram":"<suffix-id>"}}'

# List posts (optional query params: start, end, status, platform)
curl "http://localhost:8080/api/posts?start=2025-01-01T00:00:00Z&end=2025-12-31T23:59:59Z&status=scheduled" \
  -H "Authorization: Bearer sm_..."

# Get a single post
curl http://localhost:8080/api/posts/{id} \
  -H "Authorization: Bearer sm_..."

# Update a post (multipart/form-data, same as create; only provided fields are changed)
curl -X PUT http://localhost:8080/api/posts/{id} \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"Updated content","postType":"reel"}'

# Reschedule a post
curl -X PATCH http://localhost:8080/api/posts/{id}/reschedule \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"scheduledAt":"2025-06-02T10:00:00Z"}'

# Delete a post
curl -X DELETE http://localhost:8080/api/posts/{id} \
  -H "Authorization: Bearer sm_..."
```

#### Suffixes

```bash
# List suffixes
curl http://localhost:8080/api/suffixes \
  -H "Authorization: Bearer sm_..."

# Create a suffix
curl -X POST http://localhost:8080/api/suffixes \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"name":"Website footer","content":"🌐 mysite.com"}'

# Update a suffix
curl -X PUT http://localhost:8080/api/suffixes/{id} \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"content":"🌐 mysite.com | follow for more"}'

# Delete a suffix
curl -X DELETE http://localhost:8080/api/suffixes/{id} \
  -H "Authorization: Bearer sm_..."
```

#### Image Upload

```bash
curl -X POST http://localhost:8080/api/upload \
  -H "Authorization: Bearer sm_..." \
  -F "image=@photo.jpg"
# Returns: {"url": "/api/uploads/1234567890.jpg", "filename": "1234567890.jpg"}
```

Include returned URLs in the `imageUrls` array of the post `data` field, or attach binary files directly as `images` fields.

#### Post Data Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `content` | string | Yes* | Post text (*not required for Stories) |
| `platforms` | array | Yes | `["bluesky"]`, `["instagram"]`, or both |
| `postType` | string | No | `post` (default), `story`, or `reel` |
| `scheduledAt` | string | Yes | ISO 8601 datetime (e.g. `2025-06-01T09:00:00Z`) |
| `status` | string | No | `scheduled` (default) or `draft` |
| `imageUrls` | array | No | Pre-uploaded image/video URLs |
| `firstComment` | string | No | Posted as the first comment after publishing |
| `suffixIds` | object | No | `{"bluesky":"<id>","instagram":"<id>"}` |
| `accountIds` | object | No | `{"bluesky":"<id>","instagram":"<id>"}` — specific account to use |
| `tags` | array | No | Tags for internal organisation |

> **Note on `postType`**: `story` and `reel` are Instagram-only. Reels require an MP4 video file. Stories accept an image or video. The `content` field is used as the caption for Reels and is ignored for Stories.

#### Health Check

```bash
curl http://localhost:8080/api/health
# Returns: {"status": "ok"}
```

---

## n8n Integration

SocialPod ships with a native **n8n community node** located in the `n8n-nodes-socialpod/` directory. It supports all post and suffix operations and handles authentication automatically.

### Supported Operations

| Resource | Operations |
|---|---|
| **Post** | Create, Get, List, Update, Delete, Reschedule |
| **Suffix** | Create, List, Update, Delete |

The **Post → Create** and **Post → Update** operations include a **Post Type** field with three options: `Post` (default), `Reel`, and `Story`. Set this to `Reel` or `Story` when scheduling Instagram Reels or Stories. Attach the corresponding video or image file via the **Binary Image Property** or **Image URLs** fields.

### Installation

The node ships with a pre-built `dist/` directory, so you do **not** need to compile it on the target machine. Only runtime dependencies need to be installed.

> **Important**: Always use `npm install --omit=dev` when installing on the n8n server. A full `npm install` pulls in build-time dependencies (`n8n-workflow` and its transitive native module `isolated-vm`) that require Node >= 22 to compile. Since n8n already provides `n8n-workflow` at runtime, only `--omit=dev` is needed.

#### Option A — Install from the local directory (self-hosted n8n)

1. **Copy to n8n's custom nodes directory:**

   | n8n setup | Custom nodes path |
   |---|---|
   | npm global | `~/.n8n/custom` |
   | Docker | Mount a volume at `/home/node/.n8n/custom` |
   | n8n Desktop | `~/.n8n/custom` |

   ```bash
   mkdir -p ~/.n8n/custom
   cp -r n8n-nodes-socialpod ~/.n8n/custom/
   cd ~/.n8n/custom/n8n-nodes-socialpod
   npm install --omit=dev
   ```

2. **Restart n8n.** The SocialPod node will appear in the node palette.

#### Option B — Install via npm link (development)

For local development you need the full install (requires Node >= 22):

```bash
cd n8n-nodes-socialpod
npm install
npm run build
npm link

# In your n8n installation directory:
npm link n8n-nodes-socialpod
```

Restart n8n after linking.

#### Option C — Docker Compose (recommended for production)

Add the node to your n8n Docker setup by mounting the directory and installing runtime deps:

```yaml
services:
  n8n:
    image: n8nio/n8n
    volumes:
      - ./n8n-nodes-socialpod:/home/node/.n8n/custom/n8n-nodes-socialpod
    environment:
      - N8N_CUSTOM_EXTENSIONS=/home/node/.n8n/custom
```

On the host, install runtime dependencies only:

```bash
cd n8n-nodes-socialpod
npm install --omit=dev
```

Then start n8n. The pre-built `dist/` is already included — no build step required.

### Configuring the Credential

1. In n8n, go to **Credentials → New Credential → SocialPod API**.
2. Enter:
   - **API URL**: Base URL of your SocialPod instance (e.g. `https://socialpod.example.com`)
   - **API Token**: Your token from the SocialPod Profile page (`sm_...`) or a team token (`st_...`)
3. Click **Save**. n8n will test the credential against `/api/auth/me` to verify it.

### Example Workflow

A simple workflow that creates a scheduled post every weekday at 9 AM:

```
[Cron: Mon–Fri 09:00] → [SocialPod: Post → Create]
```

In the **SocialPod** node, configure:
- **Resource**: Post
- **Operation**: Create
- **Content**: `{{ $json.content }}` (or a static string)
- **Platforms**: Bluesky, Instagram
- **Scheduled At**: `{{ new Date().toISOString() }}`
- **Status**: Scheduled

---

## Makefile Targets

| Command | Description |
|---|---|
| `make up` | Build and start all services via Docker Compose |
| `make down` | Stop all services |
| `make restart` | Restart all services |
| `make build` | Build Docker images without starting |
| `make logs` | Follow container logs |
| `make clean` | Stop services and remove volumes (deletes data) |
| `make backend` | Build frontend + Go binary locally |
| `make frontend` | Build only the React frontend locally |
| `make dev` | Build frontend, embed it, and run the Go server locally |
| `make dev-frontend` | Run the Vite dev server (standalone, for frontend development) |
| `make mongo` | Start only the MongoDB container |
| `make status` | Show running containers |

---

## Architecture

```
                    ┌──────────────────────┐     ┌──────────┐
                    │   SocialPod Binary   │     │ MongoDB  │
                    │   (Go / Gin)         │────▶│          │
                    │                      │     │ Port     │
                    │  ┌── API (/api/*)    │     │ 27017    │
                    │  ├── Frontend (SPA)  │     └──────────┘
                    │  └── Scheduler       │
                    │      Port 8080       │
                    └──────────┬───────────┘
                               │
                   ┌───────────┼───────────┐
                   ▼                       ▼
            ┌─────────────┐       ┌──────────────┐
            │   Bluesky    │       │  Instagram   │
            │   AT Proto   │       │  Graph API   │
            └─────────────┘       └──────────────┘
```

The React frontend is built at compile time and embedded into the Go binary via `//go:embed`. A single binary serves the API, the SPA, uploaded files, and runs the background scheduler — all on one port.

- **API** routes live under `/api/*`.
- **Frontend** is served for all other routes, with SPA fallback to `index.html`.
- **Scheduler** checks every 30 seconds for posts past their `scheduledAt` time and publishes them. Suffixes are fetched and appended to post content at publish time.

---

## CI/CD

The project includes GitHub Actions workflows for continuous integration and deployment.

### CI (`.github/workflows/ci.yml`)

Runs on every push and PR to `main`:

1. **Backend** — Sets up Go, runs `go build`, `go vet`, and `go test -race`
2. **Frontend** — Sets up Node 20, runs `npm ci`, TypeScript type check, and `npm run build`
3. **Docker Build** — Builds both Docker images (runs after backend and frontend pass)

### CD (`.github/workflows/cd.yml`)

Runs when a version tag is pushed (e.g. `v1.0.0`):

1. **Build & Push** — Builds Docker images and pushes them to GitHub Container Registry (`ghcr.io`) with semver tags, SHA tags, and `latest`.
2. **Deploy** — SSHs into your production server and runs `docker compose pull && docker compose up -d`.

### Releasing a New Version

```bash
git tag v1.0.0
git push origin v1.0.0
```

This triggers the CD pipeline to build, push, and deploy.

### Required Setup

Configure these in your GitHub repository settings (**Settings → Secrets and variables → Actions**):

**Secrets:**
| Secret | Description |
|---|---|
| `DEPLOY_SSH_KEY` | Private SSH key for deployment server access |

**Variables** (set under the `production` environment):
| Variable | Description |
|---|---|
| `DEPLOY_HOST` | Deployment server hostname or IP |
| `DEPLOY_USER` | SSH username on the deployment server |
| `DEPLOY_PATH` | Path to the project on the server (default: `~/socialpod`) |
| `VITE_API_URL` | Public API URL baked into the frontend build (optional) |

> The deploy step is skipped if `DEPLOY_HOST` is not set, so CI/CD works out of the box for image publishing even without a deployment target.

---

## Adobe Express Integration

SocialPod can integrate the [Adobe Express Embed SDK](https://developer.adobe.com/express/embed-sdk/) so users can create images directly inside the post editor. To enable it, enter your Adobe Express Client ID in **Settings → Adobe Express**.

The SDK uses popups and cross-origin iframes that are blocked by default in some browsers. Adjust the following settings for the domain where SocialPod is hosted:

### Chrome

1. Navigate to `chrome://settings/content/popups`.
2. Under **Allowed to send pop-ups and use redirects**, click **Add** and enter your SocialPod domain (e.g. `https://socialpod.example.com`).
3. Navigate to `chrome://settings/content/cookies`.
4. Ensure **Third-party cookies** are not fully blocked, or add `[*.]adobe.com` and `[*.]adobelogin.com` to the allowed list.

### Firefox

1. Open SocialPod in Firefox, click the shield icon in the address bar.
2. Toggle off **Enhanced Tracking Protection** for this site, or set it to **Standard** globally under `about:preferences#privacy`.
3. If popups are blocked, click the notification bar that appears and select **Allow pop-ups for this site**.

### Brave

1. Click the Brave Shields icon (lion) in the address bar while on your SocialPod site.
2. Set **Shields** to **Down** for this site, or individually:
   - Set **Cross-site cookies blocked** to **All cookies allowed**.
   - Ensure **Block pop-ups** is disabled.
3. Alternatively, add `https://[*.]adobe.com` and `https://[*.]adobelogin.com` to **brave://settings/content/cookies** under allowed sites.

> **Note**: Adobe Express requires the user to sign in with an Adobe account. The SDK will prompt for login when the user first exports a design.

---

## Troubleshooting

**Instagram OAuth fails with "redirect URI mismatch"**
- Ensure the `APP_URL` in SocialPod settings exactly matches the redirect URI configured in your Meta app (including protocol and trailing slashes).

**Bluesky posts fail with "auth failed"**
- Verify the handle and app password are correct.
- If using a custom PDS, ensure the PDS host URL is correct and reachable.

**Images not showing in Instagram posts**
- Instagram fetches images from your server. Your `APP_URL` must be publicly accessible (not `localhost`).

**First user is not admin**
- The admin flag is set during registration when the users collection is empty. If you need to reset, clear the `users` collection in MongoDB.

**Container won't start — MongoDB connection refused**
- MongoDB needs to pass its health check first. Wait a few seconds and check `make logs`.

**n8n node not appearing after installation**
- Confirm the node directory contains a `dist/` folder with compiled `.js` files (it should be included out of the box).
- Make sure you ran `npm install --omit=dev` (not `npm install`) to avoid native module build failures on Node 20.
- Restart n8n completely (not just reload).
- Check n8n logs for any import errors related to `n8n-nodes-socialpod`.

**n8n node install fails with `isolated-vm` / `node-gyp` errors**
- You ran `npm install` without `--omit=dev`. The `n8n-workflow` dev dependency pulls in `isolated-vm`, which requires Node >= 22 to compile. Use `npm install --omit=dev` instead — n8n provides `n8n-workflow` at runtime.

**Watermark images show as broken after a container restart**
- Uploaded files are stored in `UPLOAD_DIR` (default `/app/uploads` when using `docker-compose.yml`). The provided `docker-compose.yml` mounts a named volume (`uploads_data`) at that path, so files survive restarts. If you deployed before this volume was added, run `make down` and `make up` — Docker will create the volume and future uploads will persist.

**Watermark images broken immediately after upload**
- Ensure `UPLOAD_DIR` is the same path in both the environment variable and the volume mount. The default `docker-compose.yml` sets `UPLOAD_DIR=/app/uploads` and mounts `uploads_data:/app/uploads`, so no manual configuration is needed.

**Instagram inbox is empty / webhooks not arriving**
- Confirm your `APP_URL` is publicly reachable over HTTPS. Meta will not deliver webhooks to `localhost` or HTTP endpoints.
- Verify the webhook subscription in the Meta app dashboard shows a green checkmark (successful verification).
- Check that the `comments` and `messages` fields are subscribed.

**AI text generation button not appearing**
- The wand button only appears when an OpenRouter API key is saved in **Settings**. Ensure the key is valid and the model is set.
