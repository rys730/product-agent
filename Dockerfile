# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Cache dependency downloads separately from source changes.
COPY go.mod go.sum* ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o product-agent .

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.21

# ca-certificates needed for outbound HTTPS (GitHub API, OpenAI/OpenRouter).
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/product-agent .

# Nobody user — no root in production.
RUN adduser -D -u 1001 agent
USER agent

EXPOSE 8080

ENTRYPOINT ["./product-agent"]
