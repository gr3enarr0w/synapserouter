---
name: feature-engineer
description: "Transform raw data into ML-ready features — encoding, scaling, temporal features, text features."
triggers:
  - "feature engineering"
  - "encoding"
  - "scaling"
  - "one-hot"
  - "feature importance"
role: coder
phase: implement
language: python
pipeline: data-science
---
# Skill: Feature Engineering

Transform raw data into ML-ready features — encoding, scaling, temporal features, text features.

Source: [Feature Engineering](https://mcpmarket.com/tools/skills/machine-learning-feature-engineering-5).

---

## When to Use

- Preparing data for machine learning models
- Creating temporal/time-based features
- Text feature extraction
- Feature selection and importance analysis

---

## Core Rules

1. **Understand the data first** — EDA before feature engineering
2. **No data leakage** — features must use only past data for predictions
3. **Handle missing values explicitly** — impute, flag, or drop with reasoning
4. **Scale appropriately** — StandardScaler for linear models, none for tree-based
5. **Create interaction features** — domain knowledge drives combinations
6. **Test feature importance** — drop unimportant features to reduce noise

---

## Patterns

### Temporal features (from timestamps)
```python
df['day_of_week'] = df['created_at'].dt.dayofweek
df['hour'] = df['created_at'].dt.hour
df['is_weekend'] = df['day_of_week'].isin([5, 6]).astype(int)
df['days_since_created'] = (pd.Timestamp.now() - df['created_at']).dt.days
df['resolution_days'] = (df['resolved_at'] - df['created_at']).dt.days
```

### Text features
```python
from sklearn.feature_extraction.text import TfidfVectorizer

tfidf = TfidfVectorizer(max_features=100, stop_words='english')
text_features = tfidf.fit_transform(df['summary'])

# Simple text stats
df['summary_word_count'] = df['summary'].str.split().str.len()
df['summary_char_count'] = df['summary'].str.len()
df['has_question_mark'] = df['summary'].str.contains(r'\?').astype(int)
```

### Categorical encoding
```python
# One-hot for low cardinality
df = pd.get_dummies(df, columns=['status'], prefix='status')

# Target encoding for high cardinality
from sklearn.preprocessing import TargetEncoder
te = TargetEncoder()
df['category_encoded'] = te.fit_transform(df[['category']], df['resolution_days'])
```

### Aggregation features
```python
# Ticket-level features from comments
comment_stats = comments.groupby('ticket_key').agg(
    comment_count=('comment_id', 'count'),
    avg_comment_length=('body', lambda x: x.str.len().mean()),
    time_to_first_response=('created_at', 'min')
)
df = df.merge(comment_stats, on='ticket_key', how='left')
```

---

## Feature Selection

```python
from sklearn.ensemble import RandomForestClassifier

model = RandomForestClassifier(n_estimators=100)
model.fit(X, y)
importances = pd.Series(model.feature_importances_, index=X.columns).sort_values(ascending=False)
print(importances.head(20))
```
