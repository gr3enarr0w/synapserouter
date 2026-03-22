# goodbooks-10k — Reconstruction Spec

## Overview

A book recommendation system built on the goodbooks-10k dataset (10,000 books, ~6M ratings from 53K users). Implements collaborative filtering (user-based and item-based KNN, SVD matrix factorization) and content-based filtering (TF-IDF on book tags), with a hybrid blending layer. CLI interface for training, evaluating, and getting recommendations.

## Scope

**IN SCOPE:**
- Data loading and preprocessing (5 CSV files)
- User-based collaborative filtering (KNN with cosine similarity)
- Item-based collaborative filtering (KNN)
- SVD matrix factorization (via surprise or manual implementation)
- Content-based filtering (TF-IDF on book tags)
- Hybrid blending (weighted combination of models)
- Evaluation metrics (RMSE, MAE, Precision@K, Recall@K, NDCG@K)
- CLI interface for train, evaluate, recommend
- Train/test split with proper methodology

**OUT OF SCOPE:**
- Web UI or API server
- Deep learning models (neural collaborative filtering)
- Real-time serving infrastructure
- Data collection/scraping
- XML book data parsing

**TARGET:** ~600-800 LOC Python, ~8 source files

## Architecture

- **Language/Runtime:** Python 3.10+
- **Key dependencies:**
  - pandas (data loading and manipulation)
  - numpy (matrix operations)
  - scikit-learn (TF-IDF, cosine similarity, metrics)
  - scipy (sparse matrices)
  - surprise (SVD implementation) — optional, can implement manually

### Directory Structure
```
goodbooks-recommender/
  data/
    books.csv
    ratings.csv
    book_tags.csv
    tags.csv
    to_read.csv
  src/
    __init__.py
    data.py           # Data loading and preprocessing
    collaborative.py  # User-based and item-based KNN
    matrix_factor.py  # SVD matrix factorization
    content.py        # TF-IDF content-based filtering
    hybrid.py         # Hybrid blender
    evaluate.py       # Metrics and evaluation
    cli.py            # CLI entry point
  requirements.txt
  README.md
```

### Design Patterns
- **Strategy pattern:** Each recommender implements a common interface (fit, predict, recommend)
- **Pipeline:** Data loading -> train/test split -> model training -> evaluation -> recommendation

## Data Flow

```
CSV Files
  |
  v
data.py: load_data() -> DataBundle(books_df, ratings_df, tags_df, book_tags_df)
  |
  v
data.py: train_test_split(ratings, test_size=0.2, random_state=42)
  |
  v
Model Training (fit on train set only)
  ├── collaborative.py: UserKNN.fit(train_ratings)
  ├── collaborative.py: ItemKNN.fit(train_ratings)
  ├── matrix_factor.py: SVDModel.fit(train_ratings)
  └── content.py: ContentModel.fit(books, book_tags, tags)
  |
  v
evaluate.py: evaluate(model, test_ratings) -> Metrics
  |
  v
hybrid.py: HybridModel.recommend(user_id, n=10) -> [(book_id, score), ...]
```

## Core Components

### DataBundle (src/data.py)
- **Purpose:** Load and preprocess all CSV files
- **Public API:**
  ```python
  class DataBundle:
      books: pd.DataFrame       # 10,000 rows
      ratings: pd.DataFrame     # ~6M rows (user_id, book_id, rating 1-5)
      tags: pd.DataFrame        # 34,252 rows (tag_id, tag_name)
      book_tags: pd.DataFrame   # ~1M rows (goodreads_book_id, tag_id, count)

  def load_data(data_dir: str) -> DataBundle
  def prepare_ratings(ratings: pd.DataFrame, min_ratings: int = 5) -> pd.DataFrame
  def split_ratings(ratings: pd.DataFrame, test_size: float = 0.2, random_state: int = 42) -> tuple[pd.DataFrame, pd.DataFrame]
  ```
- **Preprocessing:**
  - Filter users with < min_ratings ratings (reduces noise)
  - Create user-item sparse matrix (scipy.sparse.csr_matrix)
  - Map user_ids and book_ids to contiguous indices
  - Handle negative tag counts (clamp to 0)

### Data Schema

**books.csv** (10,000 rows):
- `book_id` (1-10000), `goodreads_book_id`, `best_book_id`, `work_id`
- `authors`, `original_title`, `title`, `original_publication_year`
- `average_rating`, `ratings_count`, `ratings_1` through `ratings_5`
- `isbn`, `isbn13`, `language_code`
- `image_url`, `small_image_url`

**ratings.csv** (~6M rows):
- `user_id` (1-53424, contiguous), `book_id` (1-10000, contiguous), `rating` (1-5)

**tags.csv** (34,252 rows):
- `tag_id`, `tag_name`

**book_tags.csv** (~1M rows):
- `goodreads_book_id`, `tag_id`, `count` (some negative, clamp to 0)

**to_read.csv** (~912K rows):
- `user_id`, `book_id`

### UserKNN (src/collaborative.py)
- **Purpose:** User-based collaborative filtering with K-nearest neighbors
- **Public API:**
  ```python
  class UserKNN:
      def __init__(self, k: int = 20, metric: str = "cosine")
      def fit(self, ratings: pd.DataFrame) -> None
      def predict(self, user_id: int, book_id: int) -> float
      def recommend(self, user_id: int, n: int = 10) -> list[tuple[int, float]]
  ```
- **Algorithm:**
  1. Build user-item sparse matrix
  2. Compute cosine similarity between target user and all other users
  3. Select K most similar users who rated the target item
  4. Weighted average of their ratings (weighted by similarity)

### ItemKNN (src/collaborative.py)
- **Purpose:** Item-based collaborative filtering
- **Public API:**
  ```python
  class ItemKNN:
      def __init__(self, k: int = 20, metric: str = "cosine")
      def fit(self, ratings: pd.DataFrame) -> None
      def predict(self, user_id: int, book_id: int) -> float
      def recommend(self, user_id: int, n: int = 10) -> list[tuple[int, float]]
  ```
- **Algorithm:**
  1. Build item-user sparse matrix (transpose of user-item)
  2. Compute cosine similarity between items
  3. For a user, find items they rated highly
  4. Recommend items most similar to those

### SVDModel (src/matrix_factor.py)
- **Purpose:** Matrix factorization via Singular Value Decomposition
- **Public API:**
  ```python
  class SVDModel:
      def __init__(self, n_factors: int = 50, n_epochs: int = 20, lr: float = 0.005, reg: float = 0.02)
      def fit(self, ratings: pd.DataFrame) -> None
      def predict(self, user_id: int, book_id: int) -> float
      def recommend(self, user_id: int, n: int = 10) -> list[tuple[int, float]]
  ```
- **Algorithm:**
  1. Initialize user and item factor matrices randomly
  2. SGD optimization: minimize (actual - predicted)^2 + regularization
  3. predicted = global_mean + user_bias + item_bias + dot(user_factors, item_factors)

### ContentModel (src/content.py)
- **Purpose:** Content-based filtering using book tags as features
- **Public API:**
  ```python
  class ContentModel:
      def __init__(self)
      def fit(self, books: pd.DataFrame, book_tags: pd.DataFrame, tags: pd.DataFrame) -> None
      def similar_books(self, book_id: int, n: int = 10) -> list[tuple[int, float]]
      def recommend(self, user_id: int, ratings: pd.DataFrame, n: int = 10) -> list[tuple[int, float]]
  ```
- **Algorithm:**
  1. Build book-tag matrix (weighted by tag count)
  2. Apply TF-IDF transformation
  3. Compute cosine similarity between books
  4. For a user, find books similar to their highly-rated books

### HybridModel (src/hybrid.py)
- **Purpose:** Weighted combination of multiple recommenders
- **Public API:**
  ```python
  class HybridModel:
      def __init__(self, models: list, weights: list[float])
      def recommend(self, user_id: int, n: int = 10) -> list[tuple[int, float]]
  ```
- **Algorithm:** Weighted sum of normalized scores from each model

### Evaluation (src/evaluate.py)
- **Purpose:** Compute recommendation quality metrics
- **Public API:**
  ```python
  def rmse(predictions: list[tuple[float, float]]) -> float
  def mae(predictions: list[tuple[float, float]]) -> float
  def precision_at_k(recommended: list[int], relevant: list[int], k: int) -> float
  def recall_at_k(recommended: list[int], relevant: list[int], k: int) -> float
  def ndcg_at_k(recommended: list[int], relevant: list[int], k: int) -> float
  def evaluate_model(model, test_ratings: pd.DataFrame, k: int = 10) -> dict
  ```

## Configuration

- CLI arguments (argparse):
  - `--data-dir` (default: `data/`)
  - `--model` (choices: `user-knn`, `item-knn`, `svd`, `content`, `hybrid`)
  - `--action` (choices: `train`, `evaluate`, `recommend`)
  - `--user-id` (for recommend action)
  - `--n` (number of recommendations, default: 10)
  - `--k` (KNN neighbors, default: 20)
  - `--factors` (SVD factors, default: 50)
  - `--min-ratings` (user filter threshold, default: 5)
  - `--test-size` (train/test split ratio, default: 0.2)
  - `--random-state` (seed, default: 42)

## Test Cases

### Functional Tests
1. **Data loading:** Load all 5 CSVs, verify row counts (books=10000, ratings>5M, tags>34K)
2. **User-item matrix:** Build sparse matrix, verify shape matches (users x books)
3. **UserKNN predict:** Train on sample data, predict rating for known user-book pair, verify within [1,5]
4. **SVD training:** Train SVD, verify RMSE on train set decreases over epochs
5. **Content similarity:** Get similar books to "The Hunger Games", verify results are YA fiction
6. **Hybrid recommend:** Get 10 recommendations for a user, verify no duplicates, all valid book_ids

### Edge Cases
1. **Cold start user:** User with 0 ratings -> model returns popular books as fallback
2. **Already rated:** Recommendations should exclude books the user already rated
3. **Sparse user:** User with only 1 rating -> KNN still returns valid recommendations

## Build & Run

### Setup
```bash
python -m venv venv
source venv/bin/activate
pip install -r requirements.txt
```

### Run
```bash
# Train and evaluate all models
python -m src.cli --action evaluate --model user-knn
python -m src.cli --action evaluate --model svd

# Get recommendations for user 42
python -m src.cli --action recommend --model hybrid --user-id 42 --n 10
```

### Test
```bash
pytest tests/ -v
```

## Acceptance Criteria

1. All 5 CSV files load without errors
2. Train/test split uses `random_state=42` and `test_size=0.2`
3. Scaler/encoder fit ONLY on training data (no data leakage)
4. UserKNN produces valid ratings in range [1, 5]
5. ItemKNN produces valid ratings in range [1, 5]
6. SVD model trains and RMSE < 1.0 on test set
7. Content-based model returns similar books with cosine similarity > 0
8. Hybrid model blends scores from at least 2 models
9. Precision@10 > 0 for at least 80% of test users
10. CLI accepts --model, --action, --user-id flags
11. Recommendations exclude already-rated books
12. All random operations use fixed seed (42)
13. No hardcoded file paths
14. requirements.txt lists all dependencies with versions
