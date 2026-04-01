# Test Evaluation Results

**Run ID:** eval-1774878558193017000
**Status:** COMPLETED
**Duration:** 9 seconds
**Date:** 2026-03-30

## Results Summary

- **Total Exercises:** 3
- **Language:** Go
- **Mode:** routing (automatic provider selection)
- **Two-Pass:** Enabled

## Pass Rates

| Pass | Rate | Count |
|------|------|-------|
| Pass 1 | 66.7% | 2/3 |
| Pass 2 | 33.3% | 1/3 |

## Provider Performance

| Provider | Total | Pass 1 | Pass 2 | Rate P1 | Rate P2 |
|----------|-------|--------|--------|---------|---------|
| ollama-chain-1 | 3 | 2 | 1 | 66.7% | 33.3% |

## Metrics

- **Avg Latency:** 2,388ms
- **Total Tokens:** 3,125
- **Fallback Rate:** 33.3% (1 exercise escalated)

## Command Used

```bash
./synroute eval run --language go --count 3 --two-pass
```

## View Full Results

```bash
./synroute eval results --run-id eval-1774878558193017000 --json
```

## Next Steps

Run larger evaluation:
```bash
./synroute eval run --language go --count 50 --two-pass --concurrency 6
```

Compare with previous run:
```bash
./synroute eval compare --run-a eval-1774878558193017000 --run-b <previous-run-id>
```