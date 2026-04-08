# -----------------------------------------------------------------
# Dockerfile — Multi-stage build for the Ark8de monolith server
# -----------------------------------------------------------------
#
# WHY MULTI-STAGE?
#   A multi-stage build uses multiple FROM instructions in a single
#   Dockerfile. Each stage is a separate build environment. We copy
#   only the final binary from the build stage into the runtime stage.
#
#   Result: the final image contains ONLY the binary (and minimal OS),
#   not the Go toolchain (~600MB) or source code. Final image is ~10MB
#   vs ~620MB for a naive single-stage build.
#
# STAGES IN THIS FILE:
#   1. builder  — compiles the Go binary
#   2. (final)  — minimal Alpine image with just the binary
# -----------------------------------------------------------------


# =================================================================
# Stage 1: builder — compile the Go binary
# =================================================================
# golang:1.24-alpine is the official Go image based on Alpine Linux.
# Alpine is a minimal Linux distribution (~5MB) that keeps the build
# image small. We use this stage ONLY for compilation.
FROM golang:1.24-alpine AS builder

# Set the working directory inside the container.
# All subsequent commands (COPY, RUN) operate relative to this path.
WORKDIR /build

# WHY COPY go.mod and go.sum BEFORE the source code?
#   Docker builds in layers — each instruction is a cached layer.
#   If go.mod and go.sum haven't changed, Docker reuses the cached
#   `go mod download` layer, skipping the multi-second download step
#   even when source files change. This is a standard Go Docker trick.
COPY go.mod go.sum ./

# Download all dependencies declared in go.mod.
# -v prints progress for visibility.
RUN go mod download

# Now copy the actual source code.
# We copy the entire monolith directory (.) to the workdir (/build).
COPY . .

# Compile the server binary.
#
# CGO_ENABLED=0 disables CGo (C bindings in Go).
# WHY: When CGo is enabled, the binary is dynamically linked to the
# system's C library (libc). The builder uses a different libc than
# the minimal Alpine runtime image, causing "not found" errors.
# With CGO_ENABLED=0, Go produces a fully static binary — no external
# libraries required. It runs on any Linux regardless of libc.
#
# GOOS=linux forces a Linux binary even if the developer is on macOS.
# GOARCH=amd64 targets the x86-64 architecture (standard server CPU).
#
# -o bin/server sets the output path.
# ./cmd/server is the package to build.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-w -s" \
    -o bin/server \
    ./cmd/server

# -ldflags="-w -s" strips debug information and the symbol table,
# reducing binary size by ~30% with no runtime impact.


# =================================================================
# Stage 2: runtime — minimal image with just the binary
# =================================================================
# alpine:3.20 is a minimal Linux image (~5MB).
# We copy only the compiled binary from the builder stage above —
# the Go toolchain and source code are NOT included in this image.
FROM alpine:3.20

# WHY ca-certificates?
#   The server may make HTTPS requests to external services (e.g. S3 for
#   file uploads in Phase 1). Without CA certificates, TLS verification
#   fails with "certificate signed by unknown authority". This package
#   provides the standard root certificate bundle.
RUN apk add --no-cache ca-certificates

# WHY a non-root user?
#   By default, Docker containers run as root (UID 0). If the container is
#   ever compromised, the attacker has root inside the container — and
#   potentially on the host via container escapes. Running as a non-root
#   user limits the blast radius: the attacker can only do what `appuser` can.
#   This is a standard security practice for production container images.
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Copy the compiled binary from the builder stage.
# --from=builder refers to the stage named "builder" above.
COPY --from=builder /build/bin/server /app/server

# Switch to the non-root user for all subsequent RUN/CMD/ENTRYPOINT.
USER appuser

# Expose port 8080 — this is documentation only (does not publish the port).
# The actual port mapping is done in docker-compose.yml or `docker run -p`.
EXPOSE 8080

# HEALTHCHECK tells Docker how to determine if the container is healthy.
# Docker will mark the container as "healthy" or "unhealthy" based on this.
# This is what allows docker-compose `depends_on: condition: service_healthy` to work.
#
# --interval=30s   check every 30 seconds
# --timeout=5s     fail if the check takes more than 5 seconds
# --start-period=5s don't count failures during the first 5 seconds of startup
# --retries=3      mark unhealthy only after 3 consecutive failures
#
# wget is used instead of curl because Alpine includes wget by default.
# -q: quiet (no progress output)
# --spider: fetch the URL but don't save the body (like curl -I)
# --tries=1: don't retry on failure (Docker manages retries)
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q --spider --tries=1 http://localhost:8080/healthz || exit 1

# CMD is the default command run when the container starts.
# Using array form (exec form) avoids wrapping in a shell — the process
# receives signals (SIGTERM) directly, enabling our graceful shutdown code.
CMD ["/app/server"]
