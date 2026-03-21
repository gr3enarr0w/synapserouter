---
name: eda-explorer
description: "Exploratory data analysis — distributions, outliers, correlations, trends, statistical summaries."
triggers:
  - "eda"
  - "exploratory"
  - "distribution"
  - "outlier"
  - "correlation"
  - "statistical"
role: analyst
phase: analyze
---
# Skill: EDA Explorer (Global)

Exploratory data analysis — distributions, outliers, correlations, trends, statistical summaries.

Incorporates trend analysis patterns (formerly separate skill).

---

## When to Use

- Analyzing any tabular dataset (CSV, SQLite, DataFrame)
- Understanding data distributions and patterns
- Finding outliers and anomalies
- Generating summary statistics
- Analyzing time-series trends, growth rates, and seasonality

---

## Process

### 1. Dataset overview
```python
import pandas as pd

df = pd.read_csv("data.csv")  # or sqlite3, etc.
print(f"Shape: {df.shape}")
print(f"Columns: {list(df.columns)}")
print(f"Dtypes:\n{df.dtypes}")
print(f"Missing:\n{df.isnull().sum()}")
print(f"Summary:\n{df.describe()}")
```

### 2. Distribution analysis
```python
# Categorical distributions
for col in df.select_dtypes(include='object').columns:
    print(f"\n{col}:")
    print(df[col].value_counts().head(10))

# Numeric distributions
for col in df.select_dtypes(include='number').columns:
    print(f"\n{col}: mean={df[col].mean():.2f}, std={df[col].std():.2f}, "
          f"median={df[col].median():.2f}")
```

### 3. Outlier detection (IQR method)
```python
def find_outliers(series):
    Q1, Q3 = series.quantile([0.25, 0.75])
    IQR = Q3 - Q1
    return series[(series < Q1 - 1.5*IQR) | (series > Q3 + 1.5*IQR)]
```

### 4. Correlation analysis
```python
corr = df.select_dtypes(include='number').corr()
# Find strong correlations (> 0.7)
strong = corr[(corr.abs() > 0.7) & (corr != 1.0)].stack().drop_duplicates()
```

### 5. Time-series patterns
```python
df['date'] = pd.to_datetime(df['date_col'])
daily = df.groupby(df['date'].dt.date).size()
print(f"Trend: {daily.rolling(7).mean().iloc[-1]:.1f} per day (7-day avg)")
```

---

## SQL-Based EDA & Trends (SQLite)

### Quick stats
```bash
sqlite3 -header -column data.db "
  SELECT COUNT(*) as rows,
         COUNT(DISTINCT category) as categories,
         MIN(created_at) as earliest,
         MAX(created_at) as latest
  FROM records;
"
```

### Weekly trends with moving average
```sql
WITH weekly AS (
    SELECT strftime('%Y-W%W', date_col) AS week, COUNT(*) AS cnt
    FROM events GROUP BY week
)
SELECT week, cnt,
       ROUND(AVG(cnt) OVER (ORDER BY week ROWS BETWEEN 3 PRECEDING AND CURRENT ROW), 1) AS ma_4wk,
       ROUND((cnt - LAG(cnt) OVER (ORDER BY week)) * 100.0 /
             NULLIF(LAG(cnt) OVER (ORDER BY week), 0), 1) AS wow_growth_pct
FROM weekly ORDER BY week;
```

### Month-over-month comparison
```sql
SELECT strftime('%Y-%m', date_col) AS month,
       COUNT(*) AS current_count,
       LAG(COUNT(*)) OVER (ORDER BY strftime('%Y-%m', date_col)) AS prev_month,
       ROUND((COUNT(*) - LAG(COUNT(*)) OVER (ORDER BY strftime('%Y-%m', date_col))) * 100.0 /
             NULLIF(LAG(COUNT(*)) OVER (ORDER BY strftime('%Y-%m', date_col)), 0), 1) AS mom_growth
FROM events
GROUP BY month ORDER BY month;
```

### Day-of-week seasonality
```sql
SELECT
    CASE CAST(strftime('%w', date_col) AS INTEGER)
        WHEN 0 THEN 'Sun' WHEN 1 THEN 'Mon' WHEN 2 THEN 'Tue'
        WHEN 3 THEN 'Wed' WHEN 4 THEN 'Thu' WHEN 5 THEN 'Fri' WHEN 6 THEN 'Sat'
    END AS day_name,
    COUNT(*) AS total,
    ROUND(AVG(cnt), 1) AS avg_per_day
FROM (
    SELECT date(date_col) AS day, strftime('%w', date_col) AS dow, COUNT(*) AS cnt
    FROM events GROUP BY day
)
GROUP BY dow ORDER BY CAST(dow AS INTEGER);
```

### Cumulative sum
```sql
SELECT date(date_col) AS day,
       COUNT(*) AS daily,
       SUM(COUNT(*)) OVER (ORDER BY date(date_col)) AS cumulative
FROM events
GROUP BY day ORDER BY day;
```
