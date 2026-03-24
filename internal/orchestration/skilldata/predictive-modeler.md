---
name: predictive-modeler
description: "ML forecasting, time-series analysis, risk assessment, trend extrapolation."
triggers:
  - "forecast"
  - "predict"
  - "time series"
  - "regression"
  - "classification"
  - "model training"
role: coder
phase: implement
language: python
pipeline: data-science
---
# Skill: Predictive Modeler (Global)

ML forecasting, time-series analysis, risk assessment, trend extrapolation.

---

## When to Use

- Forecasting future values from time-series data
- Risk assessment and prediction ranking
- Trend analysis and growth rate calculation
- Anomaly detection in metrics

---

## Approaches

### 1. Statistical baseline (always start here)

```python
import pandas as pd

# Moving average
df['ma_7d'] = df['value'].rolling(7).mean()

# Growth rate
df['growth'] = df['value'].pct_change(periods=7) * 100

# Linear trend
from numpy.polynomial import polynomial as P
coeffs = P.polyfit(range(len(df)), df['value'], 1)
trend_slope = coeffs[1]
```

### 2. Decomposition

```python
from statsmodels.tsa.seasonal import seasonal_decompose

result = seasonal_decompose(df['value'], period=7)
# result.trend, result.seasonal, result.resid
```

### 3. AI-powered (for qualitative analysis)

Feed statistical trends to an LLM with:
- Growth rates by category
- Outlier incidents
- Known upcoming changes
- Historical patterns

Ask for risk-ranked predictions with evidence and mitigations.

---

## Risk Ranking

| Level | Criteria |
|-------|----------|
| CRITICAL | High probability + high impact, needs immediate action |
| HIGH | Likely to occur, significant impact |
| MEDIUM | Possible based on trends, moderate impact |
| LOW | Less likely, manageable impact |

Each prediction should include:
- **What**: What will happen
- **Evidence**: Data supporting the prediction
- **Mitigation**: Actionable steps to address it

---

## SQL-Based Trends

```sql
-- Weekly trend with moving average
WITH weekly AS (
    SELECT strftime('%Y-W%W', date) as week, COUNT(*) as cnt
    FROM events GROUP BY week
)
SELECT week, cnt,
       AVG(cnt) OVER (ORDER BY week ROWS BETWEEN 3 PRECEDING AND CURRENT ROW) as ma_4wk
FROM weekly ORDER BY week;
```
