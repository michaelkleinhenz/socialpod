# Getting Started

## Prerequisites

- **Docker** and **Docker Compose** (v2)
- A publicly reachable domain for Instagram OAuth (`APP_URL` must be accessible from Meta's servers)
- Platform-specific credentials — see [[Connecting Platforms]]

## Quick Start

```bash
# 1. Clone and configure
git clone https://github.com/michaelkleinhenz/socialpod.git
cd socialpod
cp .env.example .env
# Edit .env — at minimum change JWT_SECRET

# 2. Build and start
make up

# 3. Open http://localhost:8080
#    The first registered user becomes the admin.
```

## Configuration

All configuration is done via environment variables in `.env`:

| Variable | Default | Description |
|---|---|---|
| `MONGO_USER` | `socialmedia` | MongoDB root username |
| `MONGO_PASSWORD` | `socialmedia_secret` | MongoDB root password — **change this** |
| `MONGO_DATABASE` | `socialmedia` | MongoDB database name |
| `JWT_SECRET` | `change-me-in-production` | Secret for signing JWT tokens — **change this** |
| `APP_URL` | `http://localhost:8080` | Public URL of the app (used for Instagram OAuth and media hosting) |
| `APP_PORT` | `8080` | Port the app listens on |
| `UPLOAD_DIR` | `./uploads` | Local cache of uploaded image/video files (the bucket, or MongoDB without one, holds them durably) |
| `AWS_S3_BUCKET_NAME` | — | S3-compatible bucket for uploaded files and full-system backups. Without it uploads are stored in MongoDB and backups are off |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | — | Credentials for that bucket |
| `AWS_ENDPOINT_URL` | AWS for the region | Endpoint of a non-AWS bucket (MinIO, Cloudflare R2, Railway/Tigris, ...) |
| `AWS_DEFAULT_REGION` | `us-east-1` | Bucket region |

## Backups

With an S3 bucket configured, an admin can set up backups on **Admin → Settings → Backups**: a schedule (hourly to weekly, or manual only), how many backups to keep, and the encryption passphrase. Each backup holds the whole system (users, teams, connected accounts, settings and API keys, OAuth connections, posts, drafts including image drafts, queues, agents and their run logs, growth history, the media library with its templates and colours, and every uploaded file, videos and sounds included) as an AES-256 encrypted ZIP under `backups/` in the bucket (uploaded files themselves live under `uploads/`); it opens with 7-Zip, WinZip or Keka using the passphrase. The card lists the stored backups to download, restore or delete, and restores an uploaded backup file. A restore replaces all data and first stores a safety backup of the current state. Keep the passphrase safe: a backup cannot be restored without it. Server environment settings (`JWT_SECRET`, MongoDB and S3 credentials) are not part of a backup.

## Production Deployment

For a publicly accessible deployment:

1. Set `APP_URL` to your public domain (e.g. `https://socialpod.example.com`).
2. Generate a strong `JWT_SECRET`:
   ```bash
   openssl rand -hex 32
   ```
3. Change `MONGO_PASSWORD` to a strong password.
4. Place a reverse proxy (Caddy, Traefik, nginx, etc.) in front for TLS termination.

The app ships as a single binary that serves both the API and the frontend UI on the same port — only one port needs to be exposed.

### Docker Compose Commands

| Command | Description |
|---|---|
| `make up` | Build and start all services |
| `make down` | Stop all services |
| `make restart` | Restart all services |
| `make logs` | Follow container logs |
| `make clean` | Stop services and remove volumes (deletes all data) |
| `make status` | Show running containers |

### Reverse Proxy Example (Caddy)

```
socialpod.example.com {
    reverse_proxy localhost:8080
}
```

Caddy handles HTTPS automatically. For Traefik, add the appropriate labels to the `socialpod` service in `docker-compose.yml`.
