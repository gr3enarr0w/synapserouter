---
name: dbt-modeler
description: "dbt development — medallion architecture, staging/marts, incremental models, testing."
triggers:
  - "dbt"
  - "medallion"
  - "staging model"
  - "mart model"
  - "incremental model"
role: coder
phase: analyze
language: sql
mcp_tools:
  - "context7.query-docs"
---
# Skill: dbt Modeler

dbt development — medallion architecture, staging/marts, incremental models, testing.

Source: [dbt Core Development](https://mcpmarket.com/tools/skills/dbt-core-development), [dbt MCP](https://github.com/dbt-labs/dbt-mcp) (505 stars).

---

## When to Use

- Building dbt models (staging, intermediate, marts)
- Implementing incremental materialization
- Writing dbt tests and documentation
- Designing medallion architecture (bronze/silver/gold)

---

## Core Rules

1. **Staging models** — 1:1 with sources, light transformations (rename, cast, clean)
2. **Intermediate models** — business logic, joins, aggregations
3. **Mart models** — final consumption layer, wide denormalized tables
4. **Incremental by default** — for large tables, use `is_incremental()` macro
5. **Test everything** — unique, not_null, relationships, accepted_values
6. **Source freshness** — `loaded_at_field` with warning/error thresholds

---

## Project Structure

```
models/
├── staging/
│   ├── stg_tickets.sql
│   ├── stg_comments.sql
│   └── _staging.yml           # Source + model configs
├── intermediate/
│   ├── int_ticket_enriched.sql
│   └── int_classification_stats.sql
├── marts/
│   ├── dim_tickets.sql
│   ├── fct_ticket_volume.sql
│   └── _marts.yml
└── sources.yml
```

---

## Patterns

### Staging model
```sql
WITH source AS (
    SELECT * FROM {{ source('jira', 'tickets') }}
),
renamed AS (
    SELECT
        ticket_key AS ticket_id,
        summary,
        status,
        CAST(created_at AS TIMESTAMP) AS created_at,
        CAST(resolved_at AS TIMESTAMP) AS resolved_at
    FROM source
)
SELECT * FROM renamed
```

### Incremental model
```sql
{{
    config(
        materialized='incremental',
        unique_key='ticket_id',
        incremental_strategy='merge'
    )
}}

SELECT * FROM {{ ref('stg_tickets') }}
{% if is_incremental() %}
WHERE updated_at > (SELECT MAX(updated_at) FROM {{ this }})
{% endif %}
```

### Schema YAML
```yaml
models:
  - name: dim_tickets
    description: "Item dimension table"
    columns:
      - name: ticket_id
        tests: [unique, not_null]
      - name: category
        tests:
          - accepted_values:
              values: ['Access', 'Configuration', 'Migration']
```

---

## Commands

```bash
dbt run                    # Run all models
dbt run --select staging   # Run staging only
dbt test                   # Run all tests
dbt build                  # Run + test in DAG order
dbt docs generate && dbt docs serve  # Documentation
```
