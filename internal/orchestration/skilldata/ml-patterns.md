---
name: ml-patterns
description: "Machine learning development — train/test splits, feature engineering, model evaluation, sklearn/pandas pipelines."
triggers:
  - "machine learning"
  - "ml"
  - "sklearn"
  - "scikit"
  - "tensorflow"
  - "pytorch"
  - "pandas"
  - "numpy"
  - "train"
  - "predict"
  - "classification"
  - "regression"
  - "neural network"
  - "model"
  - "dataset"
role: coder
phase: analyze
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "no data leakage"
    command: "grep -rn 'fit_transform\\|fit(' --include='*.py' | grep -v 'X_train\\|train' | grep -v '_test\\|test_' | head -5 || echo 'OK'"
    expect: "OK"
    manual: "fit() and fit_transform() should ONLY be called on training data, never on test/validation data. Test data uses transform() only."
  - name: "train test split exists"
    command: "grep -rn 'train_test_split\\|TimeSeriesSplit\\|KFold\\|StratifiedKFold' --include='*.py' || echo 'MISSING'"
    expect_not: "MISSING"
  - name: "random seed set"
    command: "grep -rn 'random_state\\|random.seed\\|np.random.seed\\|set_seed\\|manual_seed' --include='*.py' || echo 'MISSING'"
    expect_not: "MISSING"
    manual: "All random operations should have a fixed seed for reproducibility"
  - name: "metrics imported"
    command: "grep -rn 'accuracy_score\\|f1_score\\|mean_squared_error\\|r2_score\\|classification_report\\|confusion_matrix' --include='*.py' || echo 'MISSING'"
    expect_not: "MISSING"
  - name: "no hardcoded paths"
    command: "grep -rn \"'/home/\\|'C:\\\\\\|'/Users/\" --include='*.py' | head -5 || echo 'OK'"
    expect: "OK"
  - name: "dependency file exists"
    command: "ls requirements.txt pyproject.toml setup.py 2>/dev/null | head -1 || echo 'MISSING'"
    expect_not: "MISSING"
    manual: "ML projects must have requirements.txt with pinned versions (pandas, scikit-learn, numpy, etc.)"
---
# Skill: ML Patterns

Machine learning development — proper train/test methodology, feature engineering, model evaluation, and sklearn/pandas pipeline patterns.

---

## When to Use

- Building ML models (classification, regression, clustering)
- Feature engineering and data preprocessing
- Model evaluation and comparison
- sklearn, pandas, numpy, tensorflow, or pytorch code

---

## Core Rules

1. **Split before transform** — train_test_split FIRST, then fit on train only
2. **No data leakage** — never fit scalers/encoders on test data
3. **Fixed random seeds** — `random_state=42` everywhere for reproducibility
4. **Evaluate properly** — use appropriate metrics (not just accuracy)
5. **Cross-validation** — don't trust a single train/test split
6. **Feature engineering before modeling** — garbage in, garbage out
7. **Baseline first** — always compare against a simple baseline model

---

## Patterns

### Project Structure
```
project/
  data/
    raw/           # Original data, never modified
    processed/     # Cleaned/transformed data
  notebooks/       # EDA and experimentation
  src/
    data.py        # Data loading and cleaning
    features.py    # Feature engineering
    model.py       # Model training and prediction
    evaluate.py    # Metrics and evaluation
  requirements.txt
  README.md
```

### Proper Train/Test Split
```python
import pandas as pd
from sklearn.model_selection import train_test_split
from sklearn.preprocessing import StandardScaler

# Load data
df = pd.read_csv("data/raw/dataset.csv")

# Split FIRST — before any preprocessing
X = df.drop("target", axis=1)
y = df["target"]
X_train, X_test, y_train, y_test = train_test_split(
    X, y, test_size=0.2, random_state=42, stratify=y  # stratify for classification
)

# Fit scaler on train ONLY, transform both
scaler = StandardScaler()
X_train_scaled = scaler.fit_transform(X_train)  # fit + transform on train
X_test_scaled = scaler.transform(X_test)         # transform only on test
```

### sklearn Pipeline (prevents leakage)
```python
from sklearn.pipeline import Pipeline
from sklearn.compose import ColumnTransformer
from sklearn.preprocessing import StandardScaler, OneHotEncoder
from sklearn.ensemble import RandomForestClassifier

numeric_features = ["age", "income", "score"]
categorical_features = ["city", "gender"]

preprocessor = ColumnTransformer(transformers=[
    ("num", StandardScaler(), numeric_features),
    ("cat", OneHotEncoder(handle_unknown="ignore"), categorical_features),
])

pipeline = Pipeline(steps=[
    ("preprocessor", preprocessor),
    ("classifier", RandomForestClassifier(random_state=42)),
])

# Pipeline handles fit/transform correctly
pipeline.fit(X_train, y_train)
y_pred = pipeline.predict(X_test)
```

### Model Evaluation
```python
from sklearn.metrics import (
    classification_report, confusion_matrix,
    mean_squared_error, r2_score, mean_absolute_error,
)

# Classification
print(classification_report(y_test, y_pred))
print(confusion_matrix(y_test, y_pred))

# Regression
mse = mean_squared_error(y_test, y_pred)
rmse = mean_squared_error(y_test, y_pred, squared=False)
mae = mean_absolute_error(y_test, y_pred)
r2 = r2_score(y_test, y_pred)
print(f"RMSE: {rmse:.4f}, MAE: {mae:.4f}, R2: {r2:.4f}")
```

### Cross-Validation
```python
from sklearn.model_selection import cross_val_score

scores = cross_val_score(pipeline, X_train, y_train, cv=5, scoring="f1_macro")
print(f"CV F1: {scores.mean():.4f} (+/- {scores.std():.4f})")
```

### Feature Engineering
```python
# Datetime features
df["day_of_week"] = df["date"].dt.dayofweek
df["month"] = df["date"].dt.month
df["is_weekend"] = df["day_of_week"].isin([5, 6]).astype(int)

# Interaction features
df["price_per_sqft"] = df["price"] / df["sqft"]

# Binning
df["age_group"] = pd.cut(df["age"], bins=[0, 18, 35, 55, 100],
                          labels=["young", "adult", "middle", "senior"])

# Handle missing values
df["value"].fillna(df["value"].median(), inplace=True)
```

### Handling Imbalanced Data
```python
from sklearn.utils.class_weight import compute_class_weight
from imblearn.over_sampling import SMOTE

# Option 1: Class weights
weights = compute_class_weight("balanced", classes=np.unique(y_train), y=y_train)
model = RandomForestClassifier(class_weight="balanced", random_state=42)

# Option 2: SMOTE (only on training data!)
smote = SMOTE(random_state=42)
X_train_resampled, y_train_resampled = smote.fit_resample(X_train, y_train)
```

---

## Anti-Patterns

- Fitting scaler/encoder on full dataset before splitting — **data leakage**
- Using accuracy on imbalanced datasets — use F1, precision, recall, AUC
- No cross-validation — single split results are unreliable
- Tuning hyperparameters on test set — use validation set or CV
- Ignoring feature distributions — check for skew, outliers first
- Not setting random seeds — results won't be reproducible
- Training on all data without holdout — can't validate generalization
