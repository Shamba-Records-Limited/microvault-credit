#!/bin/sh
set -e

# Determine Microvault path for shared pkg (workspace context)
MICROVAULT_DIR="${MICROVAULT_DIR:-../microvault}"

# Generate Swagger docs - search in cmd/credit for main, internal/credit for controllers, and microvault/pkg for shared
swag init --parseDependency --parseInternal \
    --generalInfo ./cmd/credit/main.go \
    --dir ./,./internal/credit,"${MICROVAULT_DIR}/pkg/controllers","${MICROVAULT_DIR}/pkg/payment/yellowcard","${MICROVAULT_DIR}/pkg/auth","${MICROVAULT_DIR}/pkg/validation" \
    --output ./cmd/credit/docs

# Build the themed Redoc static HTML from swagger.json. Not
# `@redocly/cli build-docs --theme` — see scripts/render-redoc.js for why.
if [ -f ./cmd/credit/docs/swagger.json ]; then
    node ./scripts/render-redoc.js ./cmd/credit/docs/swagger.json \
        ./cmd/credit/docs/redoc-static.html "microvault Credit API" || echo "Redoc render skipped"
fi
