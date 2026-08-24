#!/bin/sh
set -e

# Determine microvault path - workspace context
MICROVAULT_DIR="${MICROVAULT_DIR:-../microvault}"

# Generate Swagger docs - search in microvault repo
swag init --parseDependency --parseInternal \
    --generalInfo "${MICROVAULT_DIR}/cmd/microvault/main.go" \
    --dir "${MICROVAULT_DIR}/,${MICROVAULT_DIR}/pkg" \
    --output "${MICROVAULT_DIR}/cmd/microvault/docs"

# Build Redoc static HTML from swagger.json
if [ -f "${MICROVAULT_DIR}/cmd/microvault/docs/swagger.json" ]; then
    npx @redocly/cli build-docs "${MICROVAULT_DIR}/cmd/microvault/docs/swagger.json" \
        --output "${MICROVAULT_DIR}/cmd/microvault/docs/redoc-static.html" 2>/dev/null || echo "Redocly build skipped"
fi
