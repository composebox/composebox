# composebox

A lightweight web UI and manager for Docker Compose stacks, distributed as a single static binary.

[![Release](https://img.shields.io/github/v/release/composebox/composebox?style=flat-square)](https://github.com/composebox/composebox/releases)
[![Go Version](https://img.shields.io/badge/go-1.26+-00ADD8?style=flat-square&logo=go)](https://golang.org)
[![Container Image](https://img.shields.io/badge/docker-ghcr.io%2Fcomposebox%2Fcomposebox-blue?style=flat-square&logo=docker)](https://github.com/composebox/composebox/pkgs/container/composebox)
[![Platforms](https://img.shields.io/badge/platforms-linux%2Famd64%20|%20linux%2Farm64-lightgrey?style=flat-square)](https://github.com/composebox/composebox/pkgs/container/composebox)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)

`composebox` points to a directory of Docker Compose projects on your filesystem, discovers each stack automatically, and provides a responsive dark-mode web interface to edit compose definitions and environment variables, control container lifecycles, and stream real-time logs.

---

## Highlights

- **Filesystem-as-Database:** 1 directory = 1 stack. Uses standard `compose.yaml` and `.env` files without proprietary database layers or synchronization drift.
- **Embedded Single Binary:** Built in Go with Svelte 5 and Tailwind CSS v4 assets bundled via Go's native `embed` package (`//go:embed`). No Node.js runtime required in production.
- **Minimal Footprint:** Idles around ~15MB of RAM with sub-second container startup times.
- **Integrated Security:** First-run browser password setup, bcrypt password hashing, and constant-time HMAC-SHA256 session signatures. Password manager and Bitwarden autofill compatible.
- **Resilient Log Streaming:** Server-Sent Events (SSE) stream `docker compose logs -f` without WebSocket proxy drops.
- **Safe File Writes:** Modifications to `compose.yaml` and `.env` are staged into a temporary file and swapped atomically (`atomicWriteFile`) to prevent partial writes.
- **Keyboard Shortcuts:** Built-in shortcut handlers for common workflow operations (`Ctrl+S`, `/`, `Esc`).

---

## Quick Start

### Docker Compose (Recommended)

```yaml
services:
  composebox:
    image: ghcr.io/composebox/composebox:latest
    container_name: composebox
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /path/to/your/stacks:/stacks
    environment:
      - STACKS_DIR=/stacks
      - PORT=8080
```

### Standalone Binary

Download the release binary for your architecture (`linux/amd64` or `linux/arm64`) from [Releases](https://github.com/composebox/composebox/releases) and run:

```bash
./composebox -stacks /path/to/your/stacks -port 8080
```

---

## Configuration & Customization

Settings can be specified using command-line arguments or environment variables. Command-line flags take precedence over environment variables.

| Flag | Environment Variable | Default | Description |
|---|---|---|---|
| `-port` | `PORT` | `8080` | Port to listen on |
| `-host` | `HOST` | `127.0.0.1` | IP interface to bind to (`0.0.0.0` in container image) |
| `-stacks` | `STACKS_DIR` | `./stacks` | Directory where Docker Compose stacks are stored |
| `-password` | `COMPOSEBOX_PASSWORD` | *(empty)* | Predefined admin password (skips web setup) |
| `-no-auth` | `NO_AUTH` | `false` | Disable authentication entirely |
| `-version` | — | — | Print version information and exit |

---

## Reverse Proxy & Ingress Setup

`composebox` binds to `0.0.0.0` or `127.0.0.1` and works seamlessly behind reverse proxies and tunneling agents. Because real-time logs are streamed via Server-Sent Events (SSE), ensure proxy buffering is disabled.

### Cloudflare Tunnel (`cloudflared`)

Add an ingress rule to your `/etc/cloudflared/config.yml`:

```yaml
ingress:
  - hostname: compose.example.com
    service: http://127.0.0.1:8080
  - service: http_status:404
```

### Nginx

```nginx
server {
    server_name compose.example.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Disable proxy buffering for real-time SSE log streaming
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 24h;
    }
}
```

### Caddy

```caddy
compose.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

---

## Keyboard Shortcuts

The web interface includes built-in keyboard navigation:

| Keybinding | Action |
|---|---|
| `Ctrl + S` / `Cmd + S` | Save active `compose.yaml` and `.env` files |
| `/` | Focus stack search filter in the sidebar |
| `Esc` | Close open modals or clear active filters |

---

## Authentication & Session Security

1. **Setup Mode:** If launched without `COMPOSEBOX_PASSWORD`, visiting the web interface initiates a first-run setup wizard asking for an admin password. The password is encrypted with bcrypt and stored with `0600` file permissions at `.composebox.auth` inside your stacks directory.
2. **Persistent HMAC Sessions:** Session tokens are cryptographically signed using HMAC-SHA256 with a persistent secret key stored at `.composebox.secret`. Active sessions survive server restarts.
3. **Headless & GitOps:** Pass `COMPOSEBOX_PASSWORD=your-secret-password` to bypass the interactive first-run setup in automated or containerized deployments.
4. **Local / Air-gapped:** Pass `-no-auth` or set `NO_AUTH=true` when running in protected environments where authentication is handled upstream.

---

## REST API Reference

All protected endpoints require an authenticated session cookie (`composebox_session`) or Bearer token header.

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/health` | Health check endpoint returning status and version |
| `GET` | `/api/auth/status` | Returns whether authentication is enabled and current login state |
| `POST` | `/api/auth/setup` | Initializes admin password on fresh installations |
| `POST` | `/api/auth/login` | Authenticates admin credentials and returns session token |
| `POST` | `/api/auth/logout` | Invalidates active session cookie |
| `GET` | `/api/stacks` | Lists all discovered stacks, container statuses, and ports |
| `POST` | `/api/stacks` | Scaffolds a new stack folder with initial compose file |
| `GET` | `/api/stacks/{name}` | Returns stack details, service containers, and file contents |
| `POST` | `/api/stacks/{name}/save` | Atomically saves updated `compose.yaml` and `.env` contents |
| `DELETE` | `/api/stacks/{name}` | Safely removes stopped stack directory from disk |
| `POST` | `/api/stacks/{name}/up` | Executes `docker compose up -d --remove-orphans` |
| `POST` | `/api/stacks/{name}/down` | Executes `docker compose down` |
| `POST` | `/api/stacks/{name}/restart` | Executes `docker compose restart` |
| `POST` | `/api/stacks/{name}/pull` | Executes `docker compose pull` |
| `GET` | `/api/stacks/{name}/logs` | Streams live Docker logs as Server-Sent Events (`text/event-stream`) |

---

## Building from Source

### Prerequisites

- **Go:** 1.26 or later
- **Node.js:** 22 or later (for compiling the Svelte UI)
- **Make:** Standard build utility (optional)

### Build Steps

```bash
# 1. Clone repository
git clone https://github.com/composebox/composebox.git
cd composebox

# 2. Build the Svelte UI and compile static binary
make build

# 3. Run unit and integration tests
make test

# 4. (Optional) Build multi-arch container image
make docker
```

The compiled standalone binary is placed at `bin/composebox`.

---

## Project Structure

```
composebox/
├── cmd/composebox/       # Application entrypoint & CLI flag handling
├── internal/
│   ├── auth/             # Bcrypt hashing, persistent secrets, constant-time HMAC sessions
│   ├── config/           # Environment and CLI configuration parser
│   ├── server/           # REST router, middleware, and SSE stream handlers
│   └── stack/            # Directory scanner, atomic file I/O, & Docker Compose runner
├── ui/                   # Svelte 5 + Tailwind CSS v4 frontend
│   ├── src/              # Svelte components, reactivity runes, and API client
│   ├── embed.go          # Go embed fs wrapper for compiled assets
│   ├── package.json      # Frontend build dependencies
│   └── vite.config.js    # Vite compilation config
├── Dockerfile            # Multi-stage reproducible container build
├── Makefile              # Development and release automation
└── README.md
```

---

## License

This project is licensed under the [MIT License](LICENSE).
