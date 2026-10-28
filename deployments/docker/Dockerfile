# Build stage
FROM golang:1.27.3-alpine3.24@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS builder

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
FROM alpine:3.24@sha256:71e9b7bae160ba167fa17414401b94cbcdaf3c32feeab9298bd794a2a0614c35

WORKDIR /app

RUN apk upgrade --no-cache && apk add --no-cache wget \
  && adduser -D -H -u 10001 app \
  && mkdir -p /data && chown app:app /data

COPY --from=builder /app/main .
COPY --from=builder /app/web ./web

# For production deployments, use DB_TYPE=postgres instead of SQLite.
# An existing SQLite volume created by an older (root) image must be
# chowned to UID 10001 once.
VOLUME /data

# Run unprivileged.
USER 10001

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
