# Getting Started

## Prerequisites

- **Docker** and **Docker Compose** (v2)
- A publicly reachable domain for Instagram OAuth and webhooks (`APP_URL` must be accessible from Meta's servers)
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
| `UPLOAD_DIR` | `./uploads` | Directory for uploaded image/video files |

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
