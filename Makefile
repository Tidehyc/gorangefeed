.PHONY: all up down down-v migrate migrate-down run worker test test-race vet build clean help

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

# Run API service locally
run:
	go run ./cmd/api

# Run Worker service locally
worker:
	go run ./cmd/worker

# Run all tests with code coverage
test:
	go test -v -cover ./...

# Run all tests with race detector
test-race:
	go test -v -race -cover ./...

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

help:
	@echo "Available Makefile targets:"
	@echo "  make up           Start Docker Compose services (MySQL, Redis, RabbitMQ)"
	@echo "  make down         Stop Docker Compose services"
	@echo "  make down-v       Stop Docker Compose services and delete volumes"
	@echo "  make migrate      Execute database migration up"
	@echo "  make migrate-down Rollback one database migration step"
	@echo "  make run          Run API service locally"
	@echo "  make worker       Run Worker service locally"
	@echo "  make test         Execute tests with coverage"
	@echo "  make test-race    Execute tests with race detector"
	@echo "  make vet          Run go vet code analysis"
	@echo "  make build        Build API, Worker and Migrate binaries"
	@echo "  make clean        Remove build artifacts"
