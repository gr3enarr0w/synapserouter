---
name: python-patterns
description: "Idiomatic Python development — PEP 8, type hints, async patterns, data modeling."
triggers:
  - "python"
  - ".py"
  - "pip"
  - "pytest"
role: coder
phase: analyze
language: python
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "dependency file exists"
    command: "ls requirements.txt pyproject.toml setup.py Pipfile 2>/dev/null | head -1 || echo 'MISSING'"
    expect_not: "MISSING"
    manual: "Project must have requirements.txt, pyproject.toml, or equivalent dependency file"
  - name: "README exists"
    command: "ls README.md README.rst README 2>/dev/null | head -1 || echo 'MISSING'"
    expect_not: "MISSING"
---
# Skill: Python Patterns

Idiomatic Python development — PEP 8, type hints, async patterns, data modeling, stdlib best practices.

Source: [affaan-m/python-patterns](https://github.com/affaan-m/everything-claude-code/tree/main/skills/python-patterns) (70K stars), [Modern Python 3.13](https://mcpmarket.com/tools/skills/modern-python-3-13-specialist).

---

## When to Use

- Writing or reviewing Python code
- Designing data models (Pydantic, dataclasses)
- Implementing async patterns
- Structuring Python projects

## When NOT to Use

- For testing-specific guidance → use `python-testing`
- For FastAPI-specific patterns → use `fastapi-patterns`
- For virtual env management → use `python-venv`

---

## Core Rules

1. **Type hints everywhere** — all function signatures, class attributes, return types
2. **Pydantic for external data** — API responses, config, user input
3. **dataclasses for internal data** — simple value objects, DTOs
4. **pathlib over os.path** — `Path("file.txt")` not `os.path.join()`
5. **f-strings over format()** — `f"{name}"` not `"{}".format(name)`
6. **Context managers** — `with open()` not manual close
7. **Enumerate over range(len())** — `for i, item in enumerate(lst)`
8. **Dictionary unpacking** — `{**defaults, **overrides}`

---

## Project Structure

```
project/
├── pyproject.toml          # Project metadata + dependencies
├── src/
│   └── package_name/
│       ├── __init__.py
│       ├── main.py         # Entry point / CLI
│       ├── models.py       # Pydantic/dataclass models
│       ├── services.py     # Business logic
│       └── utils.py        # Shared utilities
├── tests/
│   ├── conftest.py
│   └── test_*.py
└── .python-version         # Pin Python version
```

---

## Patterns

### Pydantic models for structured data
```python
from pydantic import BaseModel, Field

class ItemClassification(BaseModel):
    category: str = Field(description="High-level category")
    issue_type: str = Field(description="Specific issue type")
    confidence: float = Field(ge=0.0, le=1.0)
    keywords: list[str] = Field(default_factory=list)
```

### Async with proper error handling
```python
import asyncio
import httpx

async def fetch_all(urls: list[str]) -> list[dict]:
    async with httpx.AsyncClient() as client:
        tasks = [client.get(url) for url in urls]
        responses = await asyncio.gather(*tasks, return_exceptions=True)
        return [r.json() for r in responses if not isinstance(r, Exception)]
```

### Generator for large datasets
```python
def read_large_file(path: Path) -> Iterator[dict]:
    with open(path) as f:
        for line in f:
            yield json.loads(line)
```

### functools for caching
```python
from functools import lru_cache

@lru_cache(maxsize=128)
def expensive_computation(key: str) -> Result:
    ...
```

---

## Anti-Patterns

- `import *` — always import explicitly
- Mutable default arguments — use `None` + assignment in body
- Bare `except:` — always catch specific exceptions
- Global variables for state — use classes or closures
- `type()` for type checking — use `isinstance()`
