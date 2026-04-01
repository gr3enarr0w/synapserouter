# Caddy (simplified) — Reconstruction Spec

## Overview

A simplified HTTP server inspired by Caddy. Handles HTTP requests through a middleware chain with configurable routes, reverse proxy, and static file serving. Uses JSON configuration (no Caddyfile parser). Supports graceful reload via admin API.

## Scope

**IN SCOPE:**
- HTTP server with configurable listen address
- JSON-based configuration loading (file or stdin)
- Route matching (host, path, method)
- Handler chain (middleware pattern)
- Static file server handler
- Reverse proxy handler (single upstream, no load balancing)
- Response headers handler (add/set/remove headers)
- Admin API (GET /config, POST /config for reload)
- Graceful shutdown and config reload
- Request logging middleware
- Access log to stdout

**OUT OF SCOPE:**
- TLS/HTTPS and automatic certificate management
- Caddyfile syntax parsing
- Module loading system (plugins)
- Load balancing across multiple upstreams
- WebSocket proxying
- Compression middleware
- Authentication/authorization
- Metrics/tracing
- Rate limiting

**TARGET:** ~1000-1500 LOC Go, ~12 source files

## Architecture

- **Language/Runtime:** Go 1.22+
- **No external dependencies** — stdlib only (net/http, encoding/json, io, os, etc.)
- **Key packages:**
  - `net/http` (server, reverse proxy via httputil)
  - `net/http/httputil` (ReverseProxy for proxy handler)
  - `encoding/json` (config parsing)
  - `path/filepath` (file server path resolution)
  - `log/slog` (structured logging)

### Directory Structure
```
cmd/
  caddy/
    main.go               # Entry point, flag parsing, start server
internal/
  config/
    config.go             # Config types, Load(), Validate()
  server/
    server.go             # HTTP server lifecycle (start, stop, reload)
    routes.go             # Route matching logic
  handler/
    fileserver.go         # Static file serving handler
    reverseproxy.go       # Reverse proxy handler
    headers.go            # Response headers handler
  middleware/
    logging.go            # Request logging middleware
    chain.go              # Middleware chain builder
  admin/
    admin.go              # Admin API (GET/POST /config)
go.mod
```

### Design Patterns
- **Handler chain:** Each route has an ordered list of middleware + a final handler
- **Config-driven:** All behavior defined in JSON config, no hardcoded routes
- **Graceful reload:** Admin API accepts new config, server swaps without dropping connections
- **Interface-based handlers:** All handlers implement `http.Handler`

## Configuration Format

### JSON Config Structure
```json
{
  "admin": {
    "listen": "localhost:2019"
  },
  "apps": {
    "http": {
      "servers": {
        "main": {
          "listen": [":8080"],
          "routes": [
            {
              "match": [
                {
                  "host": ["example.com"],
                  "path": ["/api/*"]
                }
              ],
              "handle": [
                {
                  "handler": "reverse_proxy",
                  "upstream": "localhost:3000"
                }
              ]
            },
            {
              "match": [
                {
                  "path": ["/*"]
                }
              ],
              "handle": [
                {
                  "handler": "file_server",
                  "root": "/var/www/html"
                }
              ]
            }
          ]
        }
      }
    }
  }
}
```

## Core Components

### Config Types (internal/config/config.go)

```go
type Config struct {
    Admin AdminConfig          `json:"admin"`
    Apps  map[string]AppConfig `json:"apps"`
}

type AdminConfig struct {
    Listen string `json:"listen"` // default "localhost:2019"
}

type AppConfig struct {
    Servers map[string]ServerConfig `json:"servers"`
}

type ServerConfig struct {
    Listen []string      `json:"listen"` // [":8080", ":443"]
    Routes []RouteConfig `json:"routes"`
}

type RouteConfig struct {
    Match  []MatchConfig   `json:"match"`
    Handle []HandlerConfig `json:"handle"`
}

type MatchConfig struct {
    Host   []string `json:"host,omitempty"`   // hostname match
    Path   []string `json:"path,omitempty"`   // path prefix/glob match
    Method []string `json:"method,omitempty"` // HTTP method match
}

type HandlerConfig struct {
    Handler  string `json:"handler"`            // "file_server", "reverse_proxy", "headers"
    Root     string `json:"root,omitempty"`      // file_server root directory
    Upstream string `json:"upstream,omitempty"`  // reverse_proxy target
    Headers  map[string][]string `json:"headers,omitempty"` // headers to set
}
```

### Server Lifecycle (internal/server/server.go)

- `NewServer(cfg ServerConfig) *Server`
- `Start() error` — bind to listen addresses, start serving
- `Stop(ctx context.Context) error` — graceful shutdown with timeout
- `Reload(cfg ServerConfig) error` — swap config, rebuild route table

### Route Matching (internal/server/routes.go)

- Routes evaluated in order (first match wins)
- Each route has one or more matchers (all must match = AND logic)
- Path matching: exact (`/about`), prefix (`/api/*`), or catch-all (`/*`)
- Host matching: exact hostname comparison
- Method matching: exact HTTP method

### Static File Server (internal/handler/fileserver.go)

- Serves files from a root directory
- Directory listing disabled by default
- Proper MIME type detection via `http.DetectContentType` or extension mapping
- 404 for missing files, 403 for directory access
- Path traversal prevention (reject `..` paths)

### Reverse Proxy (internal/handler/reverseproxy.go)

- Proxies requests to a single upstream server
- Uses `httputil.NewSingleHostReverseProxy`
- Sets `X-Forwarded-For`, `X-Forwarded-Host`, `X-Forwarded-Proto` headers
- Passes through response status and headers
- Error handling: 502 Bad Gateway on upstream failure

### Response Headers (internal/handler/headers.go)

- Add, set, or remove response headers
- Applied as middleware (wraps the response writer)

### Request Logging (internal/middleware/logging.go)

- Logs: method, path, status code, duration, client IP
- Uses `log/slog` for structured JSON output
- Wraps `http.ResponseWriter` to capture status code

### Admin API (internal/admin/admin.go)

- `GET /config` — returns current config as JSON
- `POST /config` — accepts new config JSON, validates, triggers reload
- Listens on separate address (default `localhost:2019`)

## Test Cases

### Functional Tests
1. **Static file serving:** GET /index.html → 200 with file content
2. **Missing file:** GET /nonexistent → 404
3. **Path traversal:** GET /../etc/passwd → 403 or 400
4. **Reverse proxy:** GET /api/data → proxied to upstream, response returned
5. **Proxy upstream down:** GET /api/data with dead upstream → 502
6. **Route matching:** request matches first matching route only
7. **Host matching:** request to wrong host → no match → 404
8. **Method matching:** POST to GET-only route → no match
9. **Headers handler:** response includes configured headers
10. **Admin GET config:** GET localhost:2019/config → current JSON config
11. **Admin reload:** POST new config → server reconfigures without restart
12. **Logging:** each request produces structured log line

### Edge Cases
1. Multiple listen addresses on same server
2. Empty route list → all requests get 404
3. Invalid config JSON → reject with error, keep running
4. Graceful shutdown while requests are in-flight

## Build & Run

### Build
```bash
go build -o caddy ./cmd/caddy
```

### Run
```bash
./caddy --config config.json
# Or
cat config.json | ./caddy --config -
```

### Test
```bash
go test ./...
```

## Acceptance Criteria

1. Project builds with `go build ./...` without errors
2. `go vet ./...` passes clean
3. Server starts and listens on configured port
4. Static file server returns files from root directory
5. Reverse proxy forwards requests to upstream and returns response
6. Route matching works: host, path, method matchers
7. First matching route wins (order matters)
8. Path traversal attempts are rejected
9. Admin API returns current config on GET
10. Admin API accepts and applies new config on POST
11. Request logging produces structured output
12. Response headers handler adds/sets headers correctly
13. Graceful shutdown completes in-flight requests
14. go.mod has no external dependencies (stdlib only)
