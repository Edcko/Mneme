# ── Builder ───────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src

# Layer 1: module cache (rebuilds only when go.mod/go.sum change)
COPY go.mod go.sum ./
RUN go mod download

# Layer 2: source + build (rebuilds on any code change)
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/mneme ./cmd/engram/

# ── Runtime ───────────────────────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -g '' engram

WORKDIR /app

COPY --from=builder /out/mneme .

# Backward compatibility symlink: engram → mneme
RUN ln -s mneme engram

USER engram

EXPOSE 7438

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://localhost:7438/health || exit 1

# Environment variables are expanded by the shell.
# MNEME_DSN and MNEME_SECRET are required; MNEME_PORT defaults to 7438.
CMD exec /app/mneme cloud serve \
    --port "${MNEME_PORT:-7438}" \
    --dsn "${MNEME_DSN}" \
    --secret "${MNEME_SECRET}"
