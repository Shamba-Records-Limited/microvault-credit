#!/bin/sh
set -e

# Determine Microvault path for shared pkg (workspace context)
MICROVAULT_DIR="${MICROVAULT_DIR:-../Microvault}"

# Generate Swagger docs - search in cmd/credit for main, internal/credit for controllers, and Microvault/pkg for shared
swag init --parseDependency --parseInternal \
    --generalInfo ./cmd/credit/main.go \
    --dir ./,./internal/credit,"${MICROVAULT_DIR}/pkg" \
    --output ./cmd/credit/docs

# Build Redoc static HTML from swagger.json
if [ -f ./cmd/credit/docs/swagger.json ]; then
    npx @redocly/cli build-docs ./cmd/credit/docs/swagger.json \
        --output ./cmd/credit/docs/redoc-static.html 2>/dev/null || echo "Redocly build skipped"
fi
