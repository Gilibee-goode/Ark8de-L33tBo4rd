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
FROM golang:1.25-alpine AS builder

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
# WHAT IS scratch?
#   `scratch` is Docker's empty image — literally zero bytes, no OS, no shell,
#   no package manager. This is possible because our binary is STATICALLY LINKED
#   (CGO_ENABLED=0) — it has no runtime dependencies on libc or other system libs.
#
# WHY scratch INSTEAD OF alpine?
#   Alpine is ~8MB. For a statically linked Go binary that only needs:
#     - The binary itself (~13MB)
#     - CA certificates for HTTPS (~200KB)
#     - Template and static files (~50KB)
#   ...Alpine's package manager, shell, and busybox tools are dead weight.
#   Using scratch cuts ~8MB from the final image and eliminates the OS attack surface.
#
# TRADE-OFFS:
#   - No shell → cannot `docker exec -it <container> sh` for debugging
#   - No wget → HEALTHCHECK must use the binary itself or a compiled health checker
#   - No package manager → can't install tools at runtime
#   These are acceptable for production — debugging is done via logs and metrics.
FROM scratch

# WHY copy CA certificates?
#   The server may make HTTPS requests to external services. Without CA
#   certificates, TLS verification fails with "certificate signed by unknown
#   authority". Since scratch has no package manager, we copy the certificates
#   from the builder stage (Alpine's ca-certificates package is pre-installed
#   in the golang Alpine image).
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Copy the compiled binary from the builder stage.
# --from=builder refers to the stage named "builder" above.
COPY --from=builder /build/bin/server /app/server

# Copy the HTML templates and static assets.
# These are NOT compiled into the binary — they are read from disk at request time.
# Without this, the frontend pages would fail with "template parse failed".
COPY --from=builder /build/templates /app/templates
COPY --from=builder /build/static    /app/static

# Set the working directory for the runtime container.
# All paths that the server resolves at runtime (e.g. "templates/", "static/")
# are relative to this directory.
WORKDIR /app

# Expose port 8080 — this is documentation only (does not publish the port).
# The actual port mapping is done in docker-compose.yml or `docker run -p`.
EXPOSE 8080

# HEALTHCHECK tells Docker how to determine if the container is healthy.
# Docker will mark the container as "healthy" or "unhealthy" based on this.
# This is what allows docker-compose `depends_on: condition: service_healthy` to work.
#
# In a scratch image we have no shell and no wget/curl. Instead we use the
# binary itself as the health check command — our server returns 200 from
# /healthz when the database is reachable, and 503 if not.
#
# We call the binary with "NONE" as the healthcheck arg to trigger a simple HTTP GET.
# HOWEVER: since our binary is an HTTP server (not a health-check CLI), we can't
# easily do this from scratch. Instead, we'll rely on the orchestrator (docker-compose
# or Kubernetes) to probe the /healthz endpoint externally.
#
# For docker-compose, add this to the service definition:
#   healthcheck:
#     test: ["CMD-SHELL", "wget -q --spider --tries=1 http://localhost:8080/healthz || exit 1"]
#
# For Kubernetes, use a liveness/readiness probe (configured in the Deployment manifest).

# CMD is the default command run when the container starts.
# Using array form (exec form) is REQUIRED in scratch images — there is no
# /bin/sh to interpret shell form. The binary receives OS signals (SIGTERM)
# directly, enabling our graceful shutdown code in main.go.
CMD ["/app/server"]
