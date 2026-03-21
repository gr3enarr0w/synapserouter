---
name: python-venv
description: "Virtual environment management — venv, uv, pip, dependency isolation."
triggers:
  - "venv"
  - "virtualenv"
  - "uv"
  - "pip install"
  - "requirements.txt"
  - "pyproject.toml"
role: coder
phase: implement
---
# Skill: Python Virtual Environments

Virtual environment management — venv, uv, pip, dependency isolation.

Source: [Python Venv Manager](https://mcpmarket.com/ko/tools/skills/python-venv-manager), [uv Python Manager](https://mcpmarket.com/tools/skills/uv-python-manager-4).

---

## When to Use

- Creating or activating Python virtual environments
- Managing dependencies and package versions
- Setting up a new Python project
- Resolving dependency conflicts

---

## Core Rules

1. **Always use a venv** — never install to system Python
2. **Detect before creating** — check for `.venv/`, `venv/`, `.python-version` first
3. **Prefer uv** when available — 10-100x faster than pip
4. **Pin versions** — `requirements.txt` or `pyproject.toml` with exact versions
5. **One venv per project** — don't share across projects

---

## Quick Reference

### Detect existing venv
```bash
# Check common locations
ls -d .venv venv .env 2>/dev/null
# Check if already activated
echo $VIRTUAL_ENV
```

### Create with venv (stdlib)
```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

### Create with uv (fast)
```bash
uv venv .venv
source .venv/bin/activate
uv pip install -r requirements.txt
# Or use uv's project management:
uv init && uv add requests python-dotenv
```

### Pin Python version
```bash
echo "3.12" > .python-version
# uv will respect this automatically
```

---

## Anti-Patterns

- Installing packages globally with `pip install` outside a venv
- Reactivating venv before every shell command (maintain session)
- Creating new venv when one already exists
- Using `pip freeze > requirements.txt` without cleaning (captures transitive deps)
