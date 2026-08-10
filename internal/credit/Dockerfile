# --- Builder stage ---
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache npm git ca-certificates tzdata && \
    npm i -g @redocly/cli@latest && \
    go install github.com/swaggo/swag/cmd/swag@latest

# Set the working directory
WORKDIR /build

# Copy workspace file
COPY go.work go.work.sum* ./

# Copy module files for both repos
COPY microvault/go.mod microvault/go.sum ./microvault/
COPY microvault-credit/go.mod microvault-credit/go.sum ./microvault-credit/

RUN go mod download

# Copy source code for both repos
COPY microvault/ ./microvault/
COPY microvault-credit/ ./microvault-credit/

# Generate documentation
WORKDIR /build/microvault-credit/internal/credit
RUN swag init && \
    redocly build-docs ./docs/swagger.yaml --output ./docs/redoc-static.html 2>/dev/null || true

# Build the application
WORKDIR /build/microvault-credit
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -ldflags="-s -w" -o credit ./cmd/credit/main.go

# --- Release stage ---
FROM alpine:latest AS release

# Install runtime dependencies and create user
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S appgroup && \
    adduser -S appuser -G appgroup

WORKDIR /app

# Copy binary and necessary files from builder
COPY --from=builder /build/microvault-credit/credit ./credit
COPY --from=builder /build/microvault-credit/internal/credit/docs/redoc-static.html ./cmd/credit/docs/redoc-static.html
COPY --from=builder /build/microvault/static ./static
COPY --from=builder /build/microvault-credit/internal/credit/entrypoint.sh ./entrypoint.sh

# Set permissions before switching user
RUN chmod +x ./entrypoint.sh ./credit && \
    chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose application port
EXPOSE 8081

# Command to run when starting the container
ENTRYPOINT ["./entrypoint.sh"]
