---
name: security-review
description: "OWASP vulnerability detection, audit workflows, SAST patterns."
triggers:
  - "auth"
  - "credential"
  - "token"
  - "oauth"
  - "secret"
  - "password"
  - "api key"
  - "apikey"
  - "secure"
  - "security"
  - "vulnerability"
  - "permission"
  - "encrypt"
  - "decrypt"
  - "owasp"
role: reviewer
phase: analyze
mcp_tools:
  - "research-mcp.research_search"
verify:
  - name: "hardcoded secrets"
    command: "grep -rn 'password\\|secret\\|token' --include='*.go' | grep -v '_test.go\\|Getenv\\|flag\\|//\\|interface\\|struct\\|type\\|func\\|Token()\\|TokenSource' | grep '=.*\"[^\"]*\"' || echo 'OK'"
    expect: "OK"
  - name: "credentials in source"
    command: "grep -rn 'sk-\\|ghp_\\|gho_\\|xoxb-\\|AKIA' --include='*.go' || echo 'OK'"
    expect: "OK"
  - name: "secrets in logs"
    command: "grep -rn 'log.*URL\\|log.*url\\|log.*Key\\|log.*key\\|log.*Token\\|log.*token\\|log.*Password\\|log.*password' --include='*.go' | grep -v '_test.go' || echo 'OK'"
    manual: "Check that URLs containing secrets, API keys, tokens, and passwords are NOT logged in plaintext. Redact sensitive portions before logging."
  - name: "input validation"
    command: "grep -rn 'io.ReadAll\\|ioutil.ReadAll' --include='*.go' | grep -v 'LimitReader\\|_test.go' || echo 'OK'"
    manual: "All external input (HTTP responses, file reads) should be size-limited. Unbounded ReadAll on network responses is a DoS vector."
---
# Skill: Security Review

OWASP vulnerability detection, audit workflows, SAST patterns.

Source: [Trail of Bits skills](https://github.com/trailofbits/skills) (3.4K stars), [affaan-m/security-review](https://github.com/affaan-m/everything-claude-code/tree/main/skills/security-review) (70K stars).

---

## When to Use

- Reviewing code for security vulnerabilities
- Auditing authentication/authorization
- Checking for injection vulnerabilities
- Pre-deployment security assessment

## When NOT to Use

- For code quality/style review → use `code-review`
- For offensive security testing → out of scope

---

## Core Rules

1. **Validate all external input** — user input, API responses, file uploads
2. **Parameterized queries** — never string-concatenate SQL
3. **Least privilege** — minimal permissions, scoped tokens
4. **Defense in depth** — multiple layers of security
5. **Secrets management** — env vars or vault, never hardcoded
6. **Dependency scanning** — `pip audit`, `npm audit`, `cargo audit`

---

## OWASP Top 10 Checklist

| # | Category | What to Check |
|---|----------|---------------|
| A01 | Broken Access Control | Auth checks on every endpoint, RBAC enforcement |
| A02 | Cryptographic Failures | TLS everywhere, no hardcoded secrets, strong hashing |
| A03 | Injection | SQL injection, command injection, LDAP injection |
| A04 | Insecure Design | Threat modeling, rate limiting, input validation |
| A05 | Security Misconfiguration | Default creds, debug mode, verbose errors |
| A06 | Vulnerable Components | Dependency audit, CVE scanning |
| A07 | Auth Failures | Brute force protection, session management |
| A08 | Data Integrity | Signed updates, CI/CD pipeline security |
| A09 | Logging Failures | Audit logs, no PII in logs, tamper detection |
| A10 | SSRF | URL validation, allowlists for external requests |

---

## Variant Analysis (from Trail of Bits)

When you find a vulnerability, search for variants:
1. Understand the root cause
2. Create an exact match pattern (grep/semgrep)
3. Identify what can be abstracted
4. Iteratively generalize (stop at 50% false positive rate)
5. Triage all results

```bash
# Example: Find SQL injection patterns
rg "f['\"].*SELECT.*{" --type py
rg "execute\(f['\"]" --type py
rg "\.format\(.*\).*execute" --type py
```

---

## Quick Audit Commands

```bash
# Python dependency audit
pip audit
# Check for hardcoded secrets
rg "(password|secret|token|api_key)\s*=\s*['\"][^'\"]+['\"]" --type py
# Check for eval/exec
rg "(eval|exec)\(" --type py
# Check subprocess calls
rg "subprocess\.(call|run|Popen)" --type py
```
