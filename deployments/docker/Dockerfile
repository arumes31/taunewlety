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
FROM alpine:3.18

WORKDIR /app

COPY --from=builder /app/main .
COPY --from=builder /app/web ./web

RUN apk add --no-cache wget

HEALTHCHECK --interval=30s --timeout=3s \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/login || exit 1

EXPOSE 8080

CMD ["./main"]
