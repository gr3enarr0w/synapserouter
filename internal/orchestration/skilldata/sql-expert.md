---
name: sql-expert
description: "Cross-dialect SQL — PostgreSQL, MySQL, SQLite, SQL Server, CTEs, optimization, schema design."
triggers:
  - "sql"
  - "query"
  - "database"
  - "schema"
  - "migration"
  - "postgresql"
  - "mysql"
  - "sqlite"
role: coder
phase: analyze
language: sql
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "Constraint coverage"
    command: "grep -iE '(CHECK|UNIQUE|FOREIGN KEY|NOT NULL|PRIMARY KEY)' *.sql | wc -l"
    expect: "constraints found"
  - name: "SQL statement structure"
    command: "grep -cE '(SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER)' *.sql"
    expect: "SQL statements present"
---
# Skill: SQL Expert

Cross-dialect SQL — PostgreSQL, MySQL, SQLite, SQL Server, CTEs, optimization, schema design.

Source: [SQL Expert](https://mcpmarket.com/es/tools/skills/sql-expert), [SQL Query Expert](https://mcpmarket.com/tools/skills/sql-query-expert).

---

## When to Use

- Writing complex SQL queries (CTEs, window functions, recursive)
- Database schema design
- Query optimization and EXPLAIN analysis
- Cross-dialect SQL translation

## When NOT to Use

- For Snowflake-specific features → use `snowflake-query`
- For dbt modeling → use `dbt-modeler`

---

## Core Rules

1. **CTEs over subqueries** — readable, maintainable, debuggable
2. **Window functions** — `ROW_NUMBER()`, `LAG()`, `SUM() OVER ()` for analytics
3. **EXPLAIN before deploying** — always check query plans
4. **Indexes on WHERE/JOIN columns** — covering indexes for frequent queries
5. **Parameterized queries** — never string-format user input
6. **NULL handling** — `COALESCE()`, `NULLIF()`, understand three-value logic

---

## Patterns

### CTE for readability
```sql
WITH weekly_counts AS (
    SELECT strftime('%Y-W%W', created_at) AS week,
           category,
           COUNT(*) AS cnt
    FROM tickets t
    JOIN classifications c ON t.key = c.ticket_key
    GROUP BY week, category
),
ranked AS (
    SELECT *, ROW_NUMBER() OVER (PARTITION BY week ORDER BY cnt DESC) AS rn
    FROM weekly_counts
)
SELECT week, category, cnt
FROM ranked
WHERE rn <= 3;
```

### Window functions for trends
```sql
SELECT date,
       value,
       AVG(value) OVER (ORDER BY date ROWS BETWEEN 6 PRECEDING AND CURRENT ROW) AS moving_avg,
       value - LAG(value) OVER (ORDER BY date) AS day_over_day_change
FROM metrics;
```

### Recursive CTE (for hierarchies)
```sql
WITH RECURSIVE tree AS (
    SELECT id, parent_id, name, 0 AS depth
    FROM categories WHERE parent_id IS NULL
    UNION ALL
    SELECT c.id, c.parent_id, c.name, t.depth + 1
    FROM categories c JOIN tree t ON c.parent_id = t.id
)
SELECT * FROM tree ORDER BY depth, name;
```

---

## Dialect Differences

| Feature | PostgreSQL | MySQL | SQLite | SQL Server |
|---------|-----------|-------|--------|------------|
| String concat | `\|\|` | `CONCAT()` | `\|\|` | `+` |
| Date diff | `AGE()` | `DATEDIFF()` | `julianday()` | `DATEDIFF()` |
| Upsert | `ON CONFLICT DO UPDATE` | `ON DUPLICATE KEY UPDATE` | `ON CONFLICT DO UPDATE` | `MERGE` |
| JSON | `jsonb` | `JSON_*` | `json_*` | `JSON_*` |
| Limit | `LIMIT N` | `LIMIT N` | `LIMIT N` | `TOP N` |
