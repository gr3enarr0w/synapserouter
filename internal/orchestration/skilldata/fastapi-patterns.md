---
name: fastapi-patterns
description: "FastAPI development — async patterns, dependency injection, Pydantic validation, SQLAlchemy 2.0."
triggers:
  - "fastapi"
  - "pydantic"
  - "sqlalchemy"
  - "uvicorn"
role: coder
phase: analyze
language: python
mcp_tools:
  - "context7.query-docs"
---

> **Spec Override:** These patterns are DEFAULTS for when no spec is provided.
> If a project spec defines different architecture, package structure, or scope,
> FOLLOW THE SPEC. Do not apply these defaults over explicit spec requirements.
# Skill: FastAPI Patterns

FastAPI development — async patterns, dependency injection, Pydantic validation, SQLAlchemy 2.0.

Source: [FastAPI Development](https://mcpmarket.com/tools/skills/fastapi-python-api-development), [Python Backend Expert](https://mcpmarket.com/es/tools/skills/python-backend-expert).

---

## When to Use

- Building REST APIs with FastAPI
- Implementing authentication (JWT, OAuth)
- Database integration with SQLAlchemy 2.0
- Async endpoint design

---

## Core Rules

1. **Pydantic models for all I/O** — request bodies, response models, validation
2. **Dependency injection** — use `Depends()` for shared logic (auth, DB sessions)
3. **Async by default** — `async def` for endpoints, `httpx` for external calls
4. **Separate concerns** — Unless the project spec requires different module organization, use routes, services, models, schemas in separate modules
5. **Use status codes** — `status.HTTP_201_CREATED`, not magic numbers

---

## Patterns

### Project structure
```
app/
├── main.py              # FastAPI app + middleware
├── routers/             # Route definitions
│   ├── auth.py
│   └── tickets.py
├── models/              # SQLAlchemy models
├── schemas/             # Pydantic schemas
├── services/            # Business logic
├── dependencies.py      # Shared Depends()
└── config.py            # Settings via pydantic-settings
```

### Route with dependency injection
```python
from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.ext.asyncio import AsyncSession

router = APIRouter(prefix="/tickets", tags=["tickets"])

@router.get("/{key}", response_model=ItemResponse)
async def get_ticket(key: str, db: AsyncSession = Depends(get_db)):
    ticket = await db.get(Item, key)
    if not ticket:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND)
    return ticket
```

### Pydantic settings
```python
from pydantic_settings import BaseSettings

class Settings(BaseSettings):
    database_url: str
    jwt_secret: str
    debug: bool = False

    class Config:
        env_file = ".env"
```

### JWT authentication
```python
from fastapi.security import HTTPBearer

security = HTTPBearer()

async def get_current_user(token: str = Depends(security)):
    payload = jwt.decode(token.credentials, settings.jwt_secret)
    return await get_user(payload["sub"])
```

---

## Anti-Patterns

- Blocking I/O in async endpoints (use `run_in_executor`)
- N+1 queries (use `selectinload` / `joinedload`)
- Business logic in route handlers (extract to services)
- Missing response_model (leaks internal data)
