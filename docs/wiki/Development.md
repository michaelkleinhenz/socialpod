# Development

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
      ┌────────┬───────┬───────┴──────┬──────────┬──────────┬──────────┐
      ▼        ▼       ▼              ▼          ▼          ▼          ▼
 ┌────────┐ ┌──────┐ ┌──────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐
 │Bluesky │ │Insta │ │X/Twitter │ │Mastodon│ │Threads │ │LinkedIn│ │YouTube │
 │AT Proto│ │Graph │ │  API v2  │ │REST API│ │  API   │ │OAuth2  │ │Data v3 │
 └────────┘ └──────┘ └──────────┘ └────────┘ └────────┘ └────────┘ └────────┘
```

### Single-binary deployment

The React frontend is built and copied into `backend/cmd/server/dist/`, then embedded at compile time via `//go:embed dist/*`. The Go binary serves both the REST API (`/api/*`) and the SPA (all other routes fall back to `index.html`) on a single port.

Build sequence: `npm run build` → `cp -r frontend/dist backend/cmd/server/dist` → `go build`.

### Backend (`backend/`)

Go module `socialmedia`, using Gin as the HTTP framework.

| Path | Description |
|---|---|
| `cmd/server/main.go` | Wires dependencies; defines route groups (public, authenticated, admin, team admin) |
| `internal/config/` | Loads all config from environment variables |
| `internal/database/mongo.go` | MongoDB wrapper with typed collection accessors and index creation |
| `internal/models/` | BSON-tagged Go structs for each collection |
| `internal/handlers/` | One file per handler group (`auth.go`, `posts.go`, `admin.go`, etc.) |
| `internal/middleware/auth.go` | `AuthRequired` — tries JWT → user API token → team API token |
| `internal/services/` | `bluesky.go`, `instagram.go`, `linkedin.go`, `mastodon.go`, `threads.go`, `twitter.go`, `youtube.go`, `scheduler.go`, `imageutil.go`, `email.go` |

The scheduler runs every 30 seconds, queries for `status == "scheduled"` and `scheduledAt <= now`, and publishes posts. Suffixes are fetched from the DB and appended at publish time.

### Frontend (`frontend/`)

React 19 + TypeScript + Vite. No state management library — auth state lives in `AuthContext`, everything else is local component state fetched via `ApiClient`.

| Path | Description |
|---|---|
| `src/services/api.ts` | `ApiClient` class wrapping all `fetch` calls; redirects to `/login` on 401 |
| `src/contexts/AuthContext.tsx` | Global auth state (`user`, `loading`, `login`, `logout`, `refreshUser`) |
| `src/types/index.ts` | All shared TypeScript types |
| `src/App.tsx` | Route definitions and `ProtectedRoute` |

### Authorization Model

| Role | Context Keys | Access |
|---|---|---|
| Global admin | `isAdmin=true` | Full access to `/api/admin/*` |
| Team admin | `isTeamAdmin=true` + `teamId` | `/api/team/*` — own team's accounts and members |
| Regular user | — | `/api/posts`, `/api/suffixes`, `/api/watermarks`, `/api/inbox` |

Posts and suffixes are scoped: if the user has a `teamId`, queries filter by team; otherwise by `userId`.

---

## Local Development

### Prerequisites

- Go 1.24+
- Node.js 20+
- MongoDB (either local or via `make mongo`)

### Commands

```bash
# Backend
cd backend/
go vet ./...
go test ./... -race -count=1
go test ./path/to/package -run TestName -race

# The backend requires the frontend dist to exist
mkdir -p backend/cmd/server/dist && echo '<!doctype html>' > backend/cmd/server/dist/index.html

# Frontend
cd frontend/
npm ci
npm run build      # tsc + vite build
npm run lint       # eslint
npx tsc --noEmit   # type check only
```

### Makefile Targets

| Command | Description |
|---|---|
| `make up` | Build and start all services via Docker Compose |
| `make down` | Stop all services |
| `make restart` | Restart all services |
| `make build` | Build Docker images without starting |
| `make logs` | Follow container logs |
| `make clean` | Stop services and remove volumes (deletes data) |
| `make backend` | Build frontend + Go binary locally |
| `make frontend` | Build only the React frontend |
| `make dev` | Build frontend, embed it, and run the Go server locally |
| `make dev-frontend` | Run the Vite dev server (hot reload, proxies to backend) |
| `make mongo` | Start only the MongoDB container |
| `make status` | Show running containers |

---

## CI/CD

### CI (`.github/workflows/ci.yml`)

Runs on every push and PR to `main`:

1. **Backend** — `go build`, `go vet`, `go test -race`
2. **Frontend** — `npm ci`, TypeScript type check, `npm run build`
3. **Docker Build** — builds both Docker images (runs after backend and frontend pass)

### CD (`.github/workflows/cd.yml`)

Runs when a version tag is pushed (e.g. `v1.0.0`):

1. **Build & Push** — builds Docker images and pushes to GitHub Container Registry (`ghcr.io`) with semver tags, SHA tags, and `latest`
2. **Deploy** — SSHs into the production server and runs `docker compose pull && docker compose up -d`

### Releasing a New Version

```bash
git tag v1.0.0
git push origin v1.0.0
```

### Required Secrets and Variables

Configure in **GitHub Settings → Secrets and variables → Actions**:

**Secrets:**

| Secret | Description |
|---|---|
| `DEPLOY_SSH_KEY` | Private SSH key for deployment server access |

**Variables** (under the `production` environment):

| Variable | Description |
|---|---|
| `DEPLOY_HOST` | Deployment server hostname or IP |
| `DEPLOY_USER` | SSH username |
| `DEPLOY_PATH` | Path on the server (default: `~/socialpod`) |
| `VITE_API_URL` | Public API URL baked into the frontend build (optional) |

> The deploy step is skipped if `DEPLOY_HOST` is not set, so CI/CD works out of the box for image publishing even without a deployment target.
