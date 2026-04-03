-- Add search_circuit_breakers table for persistent circuit breaker state across CLI invocations.
-- Tracks rate-limit failures for search backends (sourcegraph, kagi, jina, etc.)
CREATE TABLE IF NOT EXISTS search_circuit_breakers (
    backend_name TEXT PRIMARY KEY,
    state TEXT NOT NULL DEFAULT 'closed',
    failure_count INTEGER NOT NULL DEFAULT 0,
    last_failure DATETIME,
    cooldown_until DATETIME
);
