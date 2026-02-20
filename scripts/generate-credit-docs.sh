#!/bin/sh
set -e

# Determine Microvault path for shared pkg (workspace context)
MICROVAULT_DIR="${MICROVAULT_DIR:-../microvault}"

# Generate Swagger docs - search in cmd/credit for main, internal/credit for controllers, and microvault/pkg for shared
swag init --parseDependency --parseInternal \
    --generalInfo ./cmd/credit/main.go \
    --dir ./,./internal/credit,"${MICROVAULT_DIR}/pkg/controllers","${MICROVAULT_DIR}/pkg/payment/yellowcard","${MICROVAULT_DIR}/pkg/auth","${MICROVAULT_DIR}/pkg/validation" \
    --output ./cmd/credit/docs

# Build Redoc static HTML from swagger.json
if [ -f ./cmd/credit/docs/swagger.json ]; then
    npx @redocly/cli build-docs ./cmd/credit/docs/swagger.json \
        --output ./cmd/credit/docs/redoc-static.html 2>/dev/null || echo "Redocly build skipped"
fi
