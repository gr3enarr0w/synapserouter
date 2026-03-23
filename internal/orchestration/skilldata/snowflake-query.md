---
name: snowflake-query
description: "Snowflake-specific SQL, schema exploration, warehouse management, stages, streams, tasks."
triggers:
  - "snowflake"
  - "warehouse"
  - "time travel"
  - "zero-copy clone"
role: coder
phase: analyze
language: sql
---
# Skill: Snowflake Query

Snowflake-specific SQL, schema exploration, warehouse management, stages, streams, tasks.

Source: [Snowflake Query](https://mcpmarket.com/es/tools/skills/snowflake-query-execution), [Snowflake MCP](https://github.com/Snowflake-Labs/mcp) (251 stars).

---

## When to Use

- Querying Snowflake data warehouse
- Schema exploration and metadata queries
- Warehouse sizing and performance tuning
- Snowflake-specific features (time travel, streams, tasks, stages)

---

## Core Rules

1. **Use warehouses efficiently** — auto-suspend, right-size, scale up not out for single queries
2. **Clustering keys** — for large tables frequently filtered on specific columns
3. **Time travel** — query historical data with `AT(TIMESTAMP => ...)`
4. **Zero-copy cloning** — `CREATE TABLE clone AS CLONE source`
5. **Flatten for JSON** — `LATERAL FLATTEN()` for semi-structured data

---

## Patterns

### Schema exploration
```sql
SHOW SCHEMAS IN DATABASE my_db;
SHOW TABLES IN SCHEMA my_db.public;
DESCRIBE TABLE my_db.public.tickets;
```

### Time travel
```sql
-- Query data as it was 1 hour ago
SELECT * FROM tickets AT(OFFSET => -3600);

-- Query data at a specific timestamp
SELECT * FROM tickets AT(TIMESTAMP => '2026-03-01 12:00:00'::TIMESTAMP);
```

### Semi-structured data
```sql
SELECT
    raw:ticket_key::STRING AS key,
    raw:classification.category::STRING AS category,
    raw:classification.confidence::FLOAT AS confidence
FROM raw_tickets,
LATERAL FLATTEN(input => raw:tags) t;
```

### Streams and tasks (CDC)
```sql
-- Create a stream to track changes
CREATE STREAM ticket_changes ON TABLE tickets;

-- Create a task to process changes
CREATE TASK process_changes
  WAREHOUSE = compute_wh
  SCHEDULE = '5 MINUTE'
AS
  INSERT INTO ticket_audit
  SELECT *, CURRENT_TIMESTAMP() FROM ticket_changes;
```

### Warehouse management
```sql
ALTER WAREHOUSE compute_wh SET
  WAREHOUSE_SIZE = 'MEDIUM'
  AUTO_SUSPEND = 60
  AUTO_RESUME = TRUE
  MIN_CLUSTER_COUNT = 1
  MAX_CLUSTER_COUNT = 3;
```

---

## Performance Tips

- `EXPLAIN` before deploying expensive queries
- Avoid `SELECT *` — specify columns
- Use `LIMIT` during development
- Monitor with `QUERY_HISTORY()` function
- Partition pruning via clustering keys
