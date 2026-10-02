.PHONY: help migrate-up migrate-down migrate-version migrate-force build build-credit build-migrate run up up-build down test test-integration test-integration-down docs clean lint lint-new lint-fix fmt yc-tx yc-seq yc-list yc-all yc-account yc-get mg-tx mg-list mg-all heal-account

COMPOSE := docker compose
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
	@echo "  make build               - Build all applications (credit + migrate + admin)"
	@echo "  make build-credit        - Build credit application"
	@echo "  make build-migrate       - Build migration CLI"
	@echo "  make build-admin         - Build admin dashboard (regenerates templ + CSS)"
	@echo ""
	@echo "Run Commands:"
	@echo "  make run                 - Run credit application"
	@echo "  make run-admin           - Run admin dashboard"
	@echo ""
	@echo "Admin Asset Commands:"
	@echo "  make admin-generate      - Regenerate templ Go from .templ files"
	@echo "  make admin-assets        - Rebuild the admin stylesheet"
	@echo "  make admin-watch         - Rebuild the stylesheet on change"
	@echo ""
	@echo "Docker Commands:"
	@echo "  make up                  - Start the dev stack in the background"
	@echo "  make up-build            - Rebuild images, then start the dev stack"
	@echo "  make down                - Stop the dev stack"
	@echo ""
	@echo "Test Commands:"
	@echo "  make test                - Run all tests"
	@echo "  make test-integration    - Run DB-backed tests in an ephemeral Postgres"
	@echo "  make test-integration-down - Tear down the test stack"
	@echo ""
	@echo "Lint Commands:"
	@echo "  make lint                - Report every lint issue"
	@echo "  make lint-new            - Report only issues in code changed against main"
	@echo "  make lint-fix            - Apply the auto-fixable subset"
	@echo "  make fmt                 - Format with gofumpt + goimports"
	@echo ""
	@echo "Provider Console Commands (require the provider env vars exported):"
	@echo "  make yc-tx ID=<payment_id> - Look up one YellowCard payment"
	@echo "  make yc-seq SEQ=<sequence_id> - Look up a YellowCard payment by our sequenceId"
	@echo "  make yc-list [N=50]      - List recent YellowCard payments"
	@echo "  make yc-all [N=100]      - Page through every YellowCard payment as JSON"
	@echo "  make yc-account          - Show YellowCard balances"
	@echo "  make yc-get P=<path> [Q=<query>] - Signed GET against any YellowCard path"
	@echo "  make mg-tx TX=<hash>     - Look up one MoneyGram SEP-24 transaction"
	@echo "  make mg-list [N=50]      - List recent MoneyGram SEP-24 transactions"
	@echo "  make mg-all              - Page through every MoneyGram SEP-24 transaction"
	@echo ""
	@echo "Operator Commands:"
	@echo "  make heal-account ACCOUNT=<id|address|phone> [APPLY=1] - Report on (or heal) a child Stellar account"
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
build: build-credit build-migrate build-admin
	@echo "All applications built successfully"

build-credit:
	@echo "Building credit application..."
	@go build -o bin/credit cmd/credit/main.go
	@echo "Build complete: bin/credit"

build-migrate:
	@echo "Building migration CLI..."
	@go build -o bin/migrate cmd/migrate/main.go
	@echo "Build complete: bin/migrate"

build-admin: admin-generate admin-assets
	@echo "Building admin dashboard..."
	@go build -o bin/admin ./cmd/admin
	@echo "Build complete: bin/admin"

# Admin asset pipeline. templ compiles .templ to _templ.go; Tailwind scans the
# .templ sources directly, so generate is not a prerequisite of the CSS build.
admin-generate:
	@go tool templ generate

admin-assets:
	@cd $(ADMIN_ASSETS) && npm install --silent && npm run build

admin-watch:
	@cd $(ADMIN_ASSETS) && npm run watch

# Run commands
run:
	@go run cmd/credit/main.go

# Docker commands. The build context is the workspace root, so the sibling
# microvault checkout must be present.
up:
	@$(COMPOSE) up

up-build:
	@$(COMPOSE) up --build

down:
	@$(COMPOSE) down

# Test commands
# Linting. golangci-lint must be built with the same Go version the modules
# target, or it refuses to load the config; `go install` picks that up from the
# local toolchain.
GOLANGCI := golangci-lint

lint:
	@echo "Linting..."
	@$(GOLANGCI) run ./...

# What a pull request should be held to while the pre-existing backlog is worked
# down: only code that differs from main is reported.
lint-new:
	@echo "Linting changes against main..."
	@$(GOLANGCI) run --new-from-rev=main ./...

lint-fix:
	@echo "Applying auto-fixes..."
	@$(GOLANGCI) run --fix ./...

fmt:
	@echo "Formatting..."
	@$(GOLANGCI) fmt ./...

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

# Provider consoles. Both scripts read their credentials from the environment,
# so export the provider vars in the calling shell first. Make treats bare words
# as goals, hence the named variables rather than positional arguments.
yc-tx:
ifndef ID
	@echo "Error: payment ID required. Usage: make yc-tx ID=abc123"
	@exit 1
endif
	@scripts/yc-payments.sh tx $(ID)

yc-seq:
ifndef SEQ
	@echo "Error: sequence ID required. Usage: make yc-seq SEQ=<sequence_id>"
	@exit 1
endif
	@scripts/yc-payments.sh seq $(SEQ)

yc-list:
	@scripts/yc-payments.sh list $(or $(N),50)

yc-all:
	@scripts/yc-payments.sh all $(or $(N),100)

yc-account:
	@scripts/yc-payments.sh account

yc-get:
ifndef P
	@echo "Error: path required. Usage: make yc-get P=/send/abc123 [Q=country=KE]"
	@exit 1
endif
	@scripts/yc-payments.sh get $(P) $(Q)

mg-tx:
ifndef TX
	@echo "Error: stellar tx hash required. Usage: make mg-tx TX=<hash>"
	@exit 1
endif
	@scripts/mg-sep24.sh $(TX)

mg-list:
	@scripts/mg-sep24.sh list $(or $(N),50)

mg-all:
	@scripts/mg-sep24.sh all

# Operator commands. Locally these run against the dev .env; on testnet the
# same binaries ship in the credit image, run with docker exec.
heal-account:
ifndef ACCOUNT
	@echo "Error: account required. Usage: make heal-account ACCOUNT=<id|address|phone> [APPLY=1]"
	@exit 1
endif
	@go run ./cmd/account-heal --account $(ACCOUNT) $(if $(APPLY),--apply,)

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
