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
FROM scratch

# Copy CA certs (needed for HTTPS to Yahoo Finance & Telegram)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy binary
COPY --from=builder /app/bot /bot

# Storage volume mountpoint (for generated .md strategy files)
VOLUME ["/storage"]

ENV TZ=Asia/Jakarta

ENTRYPOINT ["/bot"]
