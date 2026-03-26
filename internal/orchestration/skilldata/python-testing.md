---
name: python-testing
description: "Python testing — pytest, fixtures, mocking, parametrize, TDD workflow."
triggers:
  - "test"
  - "verify"
  - "validate"
  - "coverage"
role: tester
phase: verify
language: python
mcp_tools:
  - "context7.query-docs"
depends_on:
  - "code-implement"
---
# Skill: Python Testing

pytest best practices — fixtures, mocking, parametrize, TDD workflow, coverage.

Source: [affaan-m/python-testing](https://github.com/affaan-m/everything-claude-code/tree/main/skills/python-testing) (70K stars).

---

## When to Use

- Writing or reviewing Python tests
- Setting up test infrastructure
- Debugging flaky tests
- Improving test coverage

---

## Core Rules

1. **pytest over unittest** — cleaner syntax, better fixtures, plugins
2. **Arrange-Act-Assert** — clear structure in every test
3. **One assertion per test** (ideally) — test one behavior
4. **Descriptive names** — `test_login_fails_with_expired_token` not `test_login_2`
5. **Fixtures over setup/teardown** — composable, explicit dependencies
6. **parametrize for variants** — don't copy-paste tests with different inputs
7. **Mock at boundaries** — external APIs, databases, file I/O

---

## Patterns

### Fixtures
```python
@pytest.fixture
def db_connection():
    conn = sqlite3.connect(":memory:")
    conn.execute("CREATE TABLE tickets (key TEXT, summary TEXT)")
    yield conn
    conn.close()

@pytest.fixture
def sample_ticket():
    return {"key": "TEST-1", "summary": "Test ticket"}
```

### Parametrize
```python
@pytest.mark.parametrize("input,expected", [
    ("user@example.com", "[REDACTED_EMAIL]"),
    ("[~username]", "[REDACTED_MENTION]"),
    ("no-pii-here", "no-pii-here"),
])
def test_scrub_pii(input, expected):
    assert scrub_pii(input) == expected
```

### Mocking external APIs
```python
def test_fetch_tickets(mocker):
    mock_response = mocker.Mock()
    mock_response.json.return_value = {"issues": []}
    mock_response.status_code = 200
    mocker.patch("requests.get", return_value=mock_response)

    result = fetch_tickets("PROJ")
    assert result == []
```

### Testing exceptions
```python
def test_invalid_config_raises():
    with pytest.raises(ValueError, match="missing required"):
        load_config({})
```

### conftest.py for shared fixtures
```python
# tests/conftest.py
@pytest.fixture(scope="session")
def test_db():
    """Session-scoped test database."""
    db = setup_test_db()
    yield db
    db.close()
```

---

## Running Tests

```bash
pytest                          # Run all tests
pytest -v                       # Verbose output
pytest -x                       # Stop on first failure
pytest -k "test_scrub"          # Run matching tests
pytest --cov=src --cov-report=html  # Coverage report
pytest -n auto                  # Parallel execution (pytest-xdist)
```

---

## Anti-Patterns

- Testing implementation details instead of behavior
- Tests that depend on execution order
- Fixtures that do too much (setup + test + assertion)
- Mocking everything (test the real code when possible)
- No assertions (test runs but doesn't verify anything)
