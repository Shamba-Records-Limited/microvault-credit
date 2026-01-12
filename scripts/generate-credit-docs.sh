#!/bin/sh
set -e

# Generate Swagger docs - search in cmd/credit for main and pkg/internal for controllers
swag init --parseDependency --parseInternal --generalInfo ./cmd/credit/main.go --dir ./,./pkg,./internal/credit --output ./cmd/credit/docs

# Build Redoc static HTML from swagger.json
if [ -f ./cmd/credit/docs/swagger.json ]; then
    npx @redocly/cli build-docs ./cmd/credit/docs/swagger.json --output ./cmd/credit/docs/redoc-static.html 2>/dev/null || echo "Redocly build skipped"
fi
