#!/bin/sh
set -e

# Determine Microvault path (workspace context)
MICROVAULT_DIR="${MICROVAULT_DIR:-../Microvault}"

# Generate Swagger docs - search in Microvault repo
swag init --parseDependency --parseInternal \
    --generalInfo "${MICROVAULT_DIR}/cmd/Microvault/main.go" \
    --dir "${MICROVAULT_DIR}/,${MICROVAULT_DIR}/pkg" \
    --output "${MICROVAULT_DIR}/cmd/Microvault/docs"

# Build Redoc static HTML from swagger.json
if [ -f "${MICROVAULT_DIR}/cmd/Microvault/docs/swagger.json" ]; then
    npx @redocly/cli build-docs "${MICROVAULT_DIR}/cmd/Microvault/docs/swagger.json" \
        --output "${MICROVAULT_DIR}/cmd/Microvault/docs/redoc-static.html" 2>/dev/null || echo "Redocly build skipped"
fi
