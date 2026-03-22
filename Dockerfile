# ── Stage 1: Builder ─────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build deps
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files first (cache layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /app/bot ./cmd/main.go

# ── Stage 2: Runtime ─────────────────────────────────────────────────────────
FROM alpine:3.19

# Install CA certs and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Copy binary
COPY --from=builder /app/bot /bot

# Storage volume mountpoint (for generated .md strategy files)
RUN mkdir -p /storage && adduser -D -H botuser && chown botuser /storage
USER botuser

VOLUME ["/storage"]

EXPOSE 8080

ENV TZ=Asia/Jakarta

ENTRYPOINT ["/bot"]
