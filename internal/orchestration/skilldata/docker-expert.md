---
name: docker-expert
description: "Docker and container best practices — multi-stage builds, security, compose, optimization."
triggers:
  - "docker"
  - "container"
  - "dockerfile"
  - "compose"
  - "podman"
role: coder
phase: implement
mcp_tools:
  - "context7.query-docs"
---
# Skill: Docker Expert

Docker & container best practices — multi-stage builds, security, compose, optimization.

Source: [affaan-m/docker-patterns](https://github.com/affaan-m/everything-claude-code/tree/main/skills/docker-patterns) (70K stars), [Docker 2025 Master](https://mcpmarket.com/tools/skills/docker-2025-master).

---

## When to Use

- Writing or optimizing Dockerfiles
- Docker Compose setup
- Container security hardening
- Image size optimization

---

## Core Rules

1. **Multi-stage builds** — separate build and runtime stages
2. **Non-root user** — `USER appuser` in production images
3. **Minimal base images** — `python:3.12-slim`, `golang:alpine`, `distroless`
4. **`.dockerignore`** — exclude `.git`, `node_modules`, `__pycache__`, `.env`
5. **Layer caching** — copy dependency files before source code
6. **Health checks** — `HEALTHCHECK CMD curl -f http://localhost:8080/health`
7. **No secrets in images** — use build args or runtime env vars

---

## Patterns

### Python multi-stage
```dockerfile
FROM python:3.12-slim AS builder
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir --target=/deps -r requirements.txt

FROM python:3.12-slim
COPY --from=builder /deps /usr/local
COPY . /app
WORKDIR /app
RUN useradd -r appuser && chown -R appuser /app
USER appuser
CMD ["python", "main.py"]
```

### Go multi-stage
```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app/server ./cmd/server

FROM gcr.io/distroless/static
COPY --from=builder /app/server /server
CMD ["/server"]
```

### Docker Compose
```yaml
services:
  app:
    build: .
    ports: ["8080:8080"]
    env_file: .env
    depends_on:
      db:
        condition: service_healthy
  db:
    image: postgres:16-alpine
    volumes: [db-data:/var/lib/postgresql/data]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready"]
volumes:
  db-data:
```

---

## Security Checklist

- [ ] Non-root user
- [ ] No secrets baked in
- [ ] Minimal base image
- [ ] `COPY` specific files, not `COPY .`
- [ ] Pin image versions (not `:latest`)
- [ ] Scan with `docker scout cves`
