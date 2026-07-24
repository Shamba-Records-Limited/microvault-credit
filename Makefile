.PHONY: help migrate-up migrate-down migrate-version migrate-force build build-credit build-migrate run test test-integration test-integration-down docs clean

COMPOSE_TEST := docker compose -f docker-compose.test.yml

help:
	@echo "Microvault Credit Makefile Commands"
	@echo ""
	@echo "Migration Commands:"
	@echo "  make migrate-up          - Run all pending database migrations (core + credit)"
	@echo "  make migrate-down        - Rollback the last credit migration"
	@echo "  make migrate-version     - Show current credit migration version"
	@echo "  make migrate-force V=N   - Force credit migration to version N"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build               - Build all applications (credit + migrate)"
	@echo "  make build-credit        - Build credit application"
	@echo "  make build-migrate       - Build migration CLI"
	@echo ""
	@echo "Run Commands:"
	@echo "  make run                 - Run credit application"
	@echo ""
	@echo "Test Commands:"
	@echo "  make test                - Run all tests"
	@echo "  make test-integration    - Run DB-backed tests in an ephemeral Postgres"
	@echo "  make test-integration-down - Tear down the test stack"
	@echo ""
	@echo "Documentation Commands:"
	@echo "  make docs                - Generate API documentation"
	@echo ""
	@echo "Other Commands:"
	@echo "  make clean               - Remove built binaries"
	@echo ""

# Migration commands
migrate-up:
	@go run cmd/migrate/main.go up

migrate-down:
	@go run cmd/migrate/main.go down

migrate-version:
	@go run cmd/migrate/main.go version

migrate-force:
ifndef V
	@echo "Error: Version number required. Usage: make migrate-force V=3"
	@exit 1
endif
	@go run cmd/migrate/main.go force $(V)

# Build commands
build: build-credit build-migrate
	@echo "All applications built successfully"

build-credit:
	@echo "Building credit application..."
	@go build -o bin/credit cmd/credit/main.go
	@echo "Build complete: bin/credit"

build-migrate:
	@echo "Building migration CLI..."
	@go build -o bin/migrate cmd/migrate/main.go
	@echo "Build complete: bin/migrate"

# Run commands
run:
	@go run cmd/credit/main.go

# Test commands
test:
	@echo "Running all tests..."
	@go test -v ./...

# Spin up an ephemeral Postgres, migrate it, run the DB-backed suite, then tear
# down. `down` (not `down -v`) keeps the Go mod/build caches for faster reruns;
# the database is tmpfs-backed and discarded regardless. The runner's exit code
# is preserved through the teardown so CI sees a real pass/fail.
test-integration:
	@echo "Running DB-backed tests against an ephemeral Postgres..."
	@$(COMPOSE_TEST) up --build --abort-on-container-exit --exit-code-from test; \
		status=$$?; \
		$(COMPOSE_TEST) down; \
		exit $$status

test-integration-down:
	@$(COMPOSE_TEST) down

# Documentation commands
docs:
	@echo "Generating credit API documentation..."
	@sh scripts/generate-credit-docs.sh
	@echo "Credit docs generated: cmd/credit/docs/"

# Clean command
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -rf cmd/credit/docs/
	@echo "Clean complete"
