# Stage 1: Build Svelte frontend natively on host architecture
FROM --platform=$BUILDPLATFORM node:24-alpine AS ui-builder

WORKDIR /app
COPY ui/package.json ui/package-lock.json ./
RUN npm ci

COPY ui/ ./
RUN npm run build

# Stage 2: Cross-compile Go binary natively on host architecture
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=ui-builder /app/dist ./ui/dist

ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.GitCommit=${COMMIT} -X main.BuildDate=${DATE}" \
    -o /composebox ./cmd/composebox

# Stage 3: Minimal production runtime for target architecture
FROM alpine:3.23

LABEL org.opencontainers.image.title="composebox" \
      org.opencontainers.image.description="A lightweight, single-binary manager for Docker Compose stacks" \
      org.opencontainers.image.url="https://github.com/composebox/composebox" \
      org.opencontainers.image.source="https://github.com/composebox/composebox" \
      org.opencontainers.image.licenses="MIT"

# Install Docker CLI and Docker Compose plugin
RUN apk add --no-cache \
    ca-certificates \
    curl \
    docker-cli \
    docker-cli-compose \
    tzdata

# Configure non-root user with host-compatible GID (988) for docker socket access
RUN addgroup -g 1001 composebox && \
    adduser -u 1001 -G composebox -s /bin/sh -D composebox && \
    (getent group 988 >/dev/null || addgroup -g 988 docker-host) && \
    addgroup composebox $(getent group 988 | cut -d: -f1) && \
    mkdir -p /stacks && \
    chown -R 1001:1001 /stacks

WORKDIR /app
COPY --from=backend-builder /composebox /usr/local/bin/composebox

USER 1001:1001

ENV PORT=8080
ENV HOST=0.0.0.0
ENV STACKS_DIR=/stacks

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -fsS http://127.0.0.1:${PORT}/api/health || exit 1

ENTRYPOINT ["composebox"]
