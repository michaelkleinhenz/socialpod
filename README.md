# SocialPod — Social Media Scheduler

A self-hosted social media scheduling platform for **Bluesky** and **Instagram**. Features a drag-and-drop calendar UI, multitenancy, image uploads, a background post publisher, and a REST API for external integrations.

## Quick Start

```bash
# 1. Clone and configure
cp .env.example .env
# Edit .env — at minimum change JWT_SECRET

# 2. Build and start
make up

# 3. Open http://localhost:3000
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
| `APP_URL` | `http://localhost:3000` | Public URL of the app (used for Instagram OAuth) |
| `API_PORT` | `8080` | Backend API port |
| `FRONTEND_PORT` | `3000` | Frontend port |
| `VITE_API_URL` | `http://localhost:8080` | API URL the frontend calls |

### Production Deployment

For a publicly accessible deployment:

1. Set `APP_URL` to your public domain (e.g. `https://postflow.example.com`).
2. Set `VITE_API_URL` to your public API URL (e.g. `https://postflow.example.com` if using the nginx proxy, or `https://api.postflow.example.com`).
3. Generate a strong `JWT_SECRET` (e.g. `openssl rand -hex 32`).
4. Change `MONGO_PASSWORD` to a strong password.
5. Place a reverse proxy (Caddy, Traefik, etc.) in front for TLS.

The included nginx config proxies `/api/` requests to the backend, so in production you typically only need to expose port 3000 (or 80/443 via your reverse proxy).

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

- Instagram **requires at least one image** per post — text-only posts are not supported.
- Single image posts and carousels (up to 10 images) are supported.
- Captions up to 2,200 characters.
- Images must be publicly accessible (the app URL must be reachable from Instagram's servers).
- The long-lived access token expires after ~60 days. You will need to reconnect the account when it expires.

### Instagram Scopes Requested

SocialPod requests the following permissions during OAuth:
- `instagram_business_basic` — Read profile info
- `instagram_business_content_publish` — Create and publish posts
- `instagram_business_manage_messages` — Manage messages

---

## Multitenancy

- The **first user** to register becomes the **admin**.
- Subsequent users are regular users who can create and manage their own posts.
- Admins have access to:
  - Dashboard with post statistics
  - Social account management (Bluesky & Instagram)
  - User management
  - Application settings
- Each user's posts are isolated — users can only see and edit their own posts.

---

## REST API

SocialPod exposes a REST API for external scheduling. Authenticate with either a JWT token (from login) or an API token.

### Generate an API Token

1. Go to **Profile** (click your name in the sidebar).
2. Click **Generate API Token**.
3. Copy the token (format: `sm_...`).

### Authentication

Include the token in the `Authorization` header:

```
Authorization: Bearer sm_your_api_token_here
```

### Endpoints

#### Posts

```bash
# Create a scheduled post
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Hello from the API! #automation",
    "platforms": ["bluesky"],
    "scheduledAt": "2025-01-15T09:00:00Z",
    "status": "scheduled"
  }'

# List posts (with optional filters)
curl http://localhost:8080/api/posts?start=2025-01-01T00:00:00Z&end=2025-01-31T23:59:59Z \
  -H "Authorization: Bearer sm_..."

# Update a post
curl -X PUT http://localhost:8080/api/posts/{id} \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"content": "Updated content"}'

# Reschedule a post
curl -X PATCH http://localhost:8080/api/posts/{id}/reschedule \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"scheduledAt": "2025-01-16T10:00:00Z"}'

# Delete a post
curl -X DELETE http://localhost:8080/api/posts/{id} \
  -H "Authorization: Bearer sm_..."
```

#### Image Upload

```bash
curl -X POST http://localhost:8080/api/upload \
  -H "Authorization: Bearer sm_..." \
  -F "image=@photo.jpg"
# Returns: {"url": "/api/uploads/1234567890.jpg", "filename": "1234567890.jpg"}
```

Include returned URLs in the `imageUrls` array when creating a post.

#### Health Check

```bash
curl http://localhost:8080/api/health
# Returns: {"status": "ok"}
```

---

## Makefile Targets

| Command | Description |
|---|---|
| `make up` | Build and start all services |
| `make down` | Stop all services |
| `make restart` | Restart all services |
| `make build` | Build Docker images without starting |
| `make logs` | Follow container logs |
| `make clean` | Stop services and remove volumes (deletes data) |
| `make backend` | Build only the Go backend (local) |
| `make frontend` | Build only the React frontend (local) |
| `make dev-backend` | Run the Go backend locally (requires local MongoDB) |
| `make dev-frontend` | Run the React dev server locally |
| `make mongo` | Start only the MongoDB container |
| `make status` | Show running containers |

---

## Architecture

```
┌────────────┐     ┌──────────────┐     ┌──────────┐
│  Frontend   │────▶│   Backend    │────▶│ MongoDB  │
│  React/TS   │     │   Go / Gin   │     │          │
│  Port 3000  │     │   Port 8080  │     │ Port     │
│  (nginx)    │     │              │     │ 27017    │
└────────────┘     └──────┬───────┘     └──────────┘
                          │
                   ┌──────┴───────┐
                   │  Scheduler   │
                   │  (30s tick)  │
                   └──────┬───────┘
                          │
              ┌───────────┼───────────┐
              ▼                       ▼
       ┌─────────────┐       ┌──────────────┐
       │   Bluesky    │       │  Instagram   │
       │   AT Proto   │       │  Graph API   │
       └─────────────┘       └──────────────┘
```

- **Frontend** (nginx) serves the React SPA and proxies `/api/` to the backend.
- **Backend** handles auth, CRUD, file uploads, and runs the scheduler.
- **Scheduler** checks every 30 seconds for posts past their `scheduledAt` time with status `scheduled`, and publishes them to the configured platforms.

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
