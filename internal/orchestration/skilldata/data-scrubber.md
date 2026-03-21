---
name: data-scrubber
description: "PII detection, removal, and identity anonymization — 15 text categories plus consistent pseudonyms."
triggers:
  - "pii"
  - "anonymize"
  - "scrub"
  - "gdpr"
  - "redact"
  - "personally identifiable"
role: coder
phase: implement
---
# Data Scrubber

Scrubs personally identifiable information from database text fields and anonymizes identity fields with consistent pseudonyms for safe modeling.

## Two-Layer Approach

### Layer 1: Text PII Removal
Replaces PII in ticket descriptions, summaries, and comments with redaction markers like `[EMAIL]`, `[IP]`, `[SSN]`, etc. 15 categories covering emails, IPs, SSNs, credit cards, phone numbers, API keys, tokens, Kerberos principals, LDAP DNs, and credentials in URLs.

### Layer 2: Identity Anonymization
Replaces real usernames/names with consistent numbered pseudonyms. Same person always gets the same number, preserving relationships for modeling.

- `reporter_id` → `reporter_0001` (demand patterns, repeat requesters)
- `author_id` + `author_name` → `author_0001` (agent throughput, workload)
- `assignee` → `assignee_0001` (resolution patterns, capacity)
- `reporter_email` → cleared (no modeling value)

## Usage

```bash
python jsm_modeling.py scrub              # Full scrub + anonymize
python jsm_modeling.py scrub --dry-run    # Audit only
```

## Implementation

- `ingest/scrubber.py` — `scrub_pii()`, `scrub_database()`, `_build_identity_map()`
- Idempotent, GDPR-compliant, `--dry-run` for audit reporting
