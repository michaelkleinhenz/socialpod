.PHONY: up down restart build logs clean backend frontend dev-backend dev-frontend mongo status

# Docker Compose targets
up:
	docker compose up --build -d

down:
	docker compose down

restart:
	docker compose restart

build:
	docker compose build

logs:
	docker compose logs -f

clean:
	docker compose down -v

status:
	docker compose ps

# Start only MongoDB (useful for local development)
mongo:
	docker compose up -d mongodb

# Local builds (no Docker)
backend:
	cd backend && go build -o bin/server ./cmd/server

frontend:
	cd frontend && npm run build

# Local dev servers (requires MongoDB running — use `make mongo` first)
dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev
