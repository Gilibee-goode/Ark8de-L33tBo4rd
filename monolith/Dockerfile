# -----------------------------------------------------------------
# Dockerfile — Multi-stage build for ANY Ark8de service binary
# -----------------------------------------------------------------
# One Dockerfile builds every service. Select the binary with:
#   docker build -f monolith/Dockerfile --build-arg SERVICE=team-service .
#
# Valid SERVICE values: server (monolith), auth-service, player-service,
# team-service, leaderboard-service, frontend-service, gateway, migrate, seed
#
# NOTE: the build context is the REPO ROOT (not monolith/) so that
# db/migrations can be baked into the migrate image.
# -----------------------------------------------------------------

# =================================================================
# Stage 1: builder — compile the requested Go binary
# =================================================================
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy go.mod/go.sum first so `go mod download` stays cached across code changes.
COPY monolith/go.mod monolith/go.sum ./
RUN go mod download

COPY monolith/ .

# SERVICE selects which cmd/ package to compile.
ARG SERVICE=server

# CGO_ENABLED=0 → fully static binary (required for scratch).
# GOARCH is intentionally unset so the image matches the build host
# (arm64 on Apple Silicon, amd64 on x86 CI).
RUN CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-w -s" -o bin/service ./cmd/${SERVICE}


# =================================================================
# Stage 2: runtime — scratch image with just the binary + assets
# =================================================================
FROM scratch

# CA certs for any outbound HTTPS.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY --from=builder /build/bin/service /app/service

# Templates + static assets — only frontend-service/server read these, but
# they are tiny and including them keeps the Dockerfile uniform.
COPY --from=builder /build/templates /app/templates
COPY --from=builder /build/static    /app/static

# SQL migrations — read by the migrate binary via MIGRATIONS_PATH=/app/migrations.
COPY db/migrations /app/migrations

WORKDIR /app

EXPOSE 8080

CMD ["/app/service"]
