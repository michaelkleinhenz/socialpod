.PHONY: up down restart build logs clean frontend backend dev mongo status

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

# Local builds
frontend:
	cd frontend && npm ci && npm run build

backend: frontend
	cp -r frontend/dist backend/cmd/server/dist
	cd backend && go build -o bin/server ./cmd/server

# Local dev (requires MongoDB running — use `make mongo` first)
dev: frontend
	cp -r frontend/dist backend/cmd/server/dist
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev
