# Build stage
FROM golang:1.27-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the app with version injection
ARG VERSION=v0.1-dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X 'taunewlety/internal/app/taunewlety.Version=${VERSION}'" \
    -o main ./cmd/taunewlety \
    && mkdir -p /runtime/data


# Final stage
FROM scratch

WORKDIR /app

COPY --from=builder --chown=10001:10001 /app/main .
COPY --from=builder --chown=10001:10001 /app/web ./web
COPY --from=builder --chown=10001:10001 /runtime/data /data
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# For production deployments, use DB_TYPE=postgres instead of SQLite.
VOLUME /data

HEALTHCHECK --interval=30s --timeout=3s CMD ["./main", "healthcheck"]

EXPOSE 8080

USER 10001:10001

CMD ["./main"]
