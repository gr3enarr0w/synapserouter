---
name: api-design
description: "REST/OpenAPI endpoint design — pagination, auth schemes, error handling, versioning."
triggers:
  - "endpoint"
  - "handler"
  - "rest"
  - "route"
  - "api"
  - "openapi"
  - "swagger"
role: architect
phase: analyze
mcp_tools:
  - "context7.query-docs"
---

> **Spec Override:** These patterns are defaults. If a project spec defines different
> architecture, package structure, or scope, follow the spec instead.
# Skill: API Design

REST/OpenAPI patterns — pagination, auth schemes, error handling, versioning.

---

## When to Use

- Designing REST APIs
- Writing OpenAPI/Swagger specs
- Implementing pagination and filtering
- API versioning strategy

---

## Core Rules

1. **Nouns for resources** — `/items`, `/users`, not `/getItems`
2. **HTTP methods for actions** — GET (read), POST (create), PUT (replace), PATCH (update), DELETE
3. **Consistent error format** — `{"error": {"code": "NOT_FOUND", "message": "..."}}`
4. **Pagination by default** — cursor-based for large datasets, offset for small
5. **Versioning** — URL path (`/v2/`) or header (`Accept: application/vnd.api.v2+json`)
6. **HATEOAS links** — include navigation links in responses

---

## Patterns

### RESTful resource design
```
GET    /api/v1/items           # List (paginated)
GET    /api/v1/items/KEY-123   # Get one
POST   /api/v1/items           # Create
PATCH  /api/v1/items/KEY-123   # Partial update
DELETE /api/v1/items/KEY-123   # Delete
GET    /api/v1/items/KEY-123/comments  # Sub-resource
```

### Pagination (cursor-based)
```json
{
  "data": [...],
  "pagination": {
    "next_cursor": "eyJpZCI6MTAwfQ==",
    "has_more": true,
    "total": 574
  }
}
```

### Filtering and sorting
```
GET /api/v1/items?status=open&category=access&sort=-created_at&limit=20
```

### Error responses
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid request",
    "details": [
      {"field": "status", "message": "must be one of: open, closed, resolved"}
    ]
  }
}
```

### Auth patterns
| Method | Use Case | Header |
|--------|----------|--------|
| Bearer token | API keys, JWTs | `Authorization: Bearer <token>` |
| Basic auth | Simple username/password | `Authorization: Basic <base64>` |
| OAuth 2.0 | Third-party access | `Authorization: Bearer <access_token>` |
| API key | Machine-to-machine | `X-API-Key: <key>` |

---

## HTTP Status Codes

| Code | Meaning | When to Use |
|------|---------|-------------|
| 200 | OK | Successful GET, PUT, PATCH |
| 201 | Created | Successful POST |
| 204 | No Content | Successful DELETE |
| 400 | Bad Request | Invalid input |
| 401 | Unauthorized | Missing/invalid auth |
| 403 | Forbidden | Valid auth, insufficient permissions |
| 404 | Not Found | Resource doesn't exist |
| 409 | Conflict | Duplicate resource |
| 429 | Too Many Requests | Rate limited |
| 500 | Internal Server Error | Unhandled error |
