.PHONY: all up down down-v migrate migrate-down promote-admin run worker test test-race e2e vet build clean seed seed-media web-build web-dev help

all: vet test build

# Start local infrastructure (MySQL 3307, Redis 6380, RabbitMQ 5673)
up:
	docker compose up -d

# Stop local infrastructure
down:
	docker compose down

# Stop infrastructure and remove data volumes
down-v:
	docker compose down -v

# Run database migrations up
migrate:
	go run ./cmd/migrate -action up

# Rollback one database migration step
migrate-down:
	go run ./cmd/migrate -action down

# Grant the administrator role to a user (usage: make promote-admin ID=1)
promote-admin:
	@test -n "$(ID)" || (echo "usage: make promote-admin ID=<user_id>" && exit 1)
	docker compose exec -T mysql mysql -uroot -proot_password gorangefeed \
		-e "UPDATE users SET role='admin' WHERE id=$(ID); SELECT id, username, status, role FROM users WHERE id=$(ID);"

# Run API service locally
run:
	go run ./cmd/api

# Run Worker service locally
worker:
	go run ./cmd/worker

# Run all tests with code coverage
test:
	go test -v -p 1 -cover ./internal/...

# Run all tests with race detector
test-race:
	go test -v -p 1 -race -cover ./internal/...

# Run the end-to-end suite (requires the compose stack from `make up`)
e2e:
	go test -count=1 ./test/e2e/

# Static code analysis
vet:
	go vet ./...

# Build all binaries into bin/
build:
	mkdir -p bin
	go build -o bin/api ./cmd/api
	go build -o bin/worker ./cmd/worker
	go build -o bin/migrate ./cmd/migrate

# Clean build artifacts
clean:
	rm -rf bin/ coverage.txt coverage.html coverage.out

# Seed demo data (users, videos, follows, likes, comments). Requires the compose stack.
# Run `make seed-media` first if the clip files are missing.
seed:
	go run ./cmd/seed

# Render the royalty-free demo clips and covers under uploads/ (ffmpeg required).
seed-media:
	bash scripts/seed_media.sh

# Build the React client into web/dist, served by the API on `make run`.
web-build:
	cd web && npm install && npm run build

# Run the Vite dev server (proxies /api and /media to 127.0.0.1:8080).
web-dev:
	cd web && npm run dev

help:
	@echo "Available Makefile targets:"
	@echo "  make up           Start Docker Compose services (MySQL, Redis, RabbitMQ)"
	@echo "  make down         Stop Docker Compose services"
	@echo "  make down-v       Stop Docker Compose services and delete volumes"
	@echo "  make migrate      Execute database migration up"
	@echo "  make migrate-down Rollback one database migration step"
	@echo "  make promote-admin ID=<id> Grant the admin role to an existing user"
	@echo "  make run          Run API service locally"
	@echo "  make worker       Run Worker service locally"
	@echo "  make test         Execute tests with coverage"
	@echo "  make test-race    Execute tests with race detector"
	@echo "  make e2e          Execute end-to-end suite (requires 'make up')"
	@echo "  make vet          Run go vet code analysis"
	@echo "  make build        Build API, Worker and Migrate binaries"
	@echo "  make clean        Remove build artifacts"
	@echo "  make seed         Seed demo users, videos, follows and interactions"
	@echo "  make seed-media   Render demo clips and covers with ffmpeg"
	@echo "  make web-build    Build the React client into web/dist"
	@echo "  make web-dev      Run the Vite dev server on :5173"
