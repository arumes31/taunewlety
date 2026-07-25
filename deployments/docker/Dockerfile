# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the app with version injection
ARG VERSION=v0.1-dev
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-X 'taunewlety/internal/app/taunewlety.Version=${VERSION}'" -o main ./cmd/taunewlety/main.go


# Final stage
FROM alpine:3.21

WORKDIR /app

COPY --from=builder /app/main .
COPY --from=builder /app/web ./web

RUN apk add --no-cache wget

# For production deployments, use DB_TYPE=postgres instead of SQLite.
VOLUME /data

# Probe over HTTPS when the server is configured for direct TLS, otherwise
# plain HTTP. Certificate validation is skipped because the probe targets
# localhost, where the certificate's hostname will not match.
# This is a GET, not --spider: --spider issues a HEAD, and the router only
# answers GET on /health, so a spider probe always reports 404.
HEALTHCHECK --interval=30s --timeout=3s \
  CMD sh -c 'if [ -n "$TLS_CERT" ] && [ -n "$TLS_KEY" ]; then \
        wget --no-verbose --tries=1 --no-check-certificate -O /dev/null "https://localhost:${PORT:-8080}/health"; \
      else \
        wget --no-verbose --tries=1 -O /dev/null "http://localhost:${PORT:-8080}/health"; \
      fi' || exit 1

EXPOSE 8080

CMD ["./main"]
