# CIFAR-10 CNN Image Classifier — Reconstruction Spec

## Overview

A Jupyter notebook that builds, trains, and evaluates a Convolutional Neural Network (CNN) for classifying CIFAR-10 images into 10 categories: airplane, automobile, bird, cat, deer, dog, frog, horse, ship, truck. The notebook progresses through three model variants of increasing sophistication: a simple single-conv-layer baseline, a deeper multi-layer CNN, and a data-augmented version with learning rate scheduling and early stopping. Uses TensorFlow/Keras with the built-in CIFAR-10 dataset (50,000 training + 10,000 test images, 32x32 RGB).

## Scope

**IN SCOPE:**
- Dataset loading via `keras.datasets.cifar10`
- Data exploration (shape inspection, sample visualization grid)
- Preprocessing (float32 conversion, /255 normalization, one-hot encoding)
- Model 1 — Simple CNN: 1 Conv2D block + Dense head (~67% test accuracy)
- Model 2 — Improved CNN: 4 Conv2D layers in 2 blocks + Dense head (~78% test accuracy)
- Model 3 — Augmented CNN: Model 2 architecture + ImageDataGenerator + LR decay + EarlyStopping (~80% test accuracy)
- Training history visualization (accuracy and loss curves for each model)
- Test set evaluation with printed loss and accuracy
- Per-class evaluation: classification report (precision, recall, F1 per class) and confusion matrix heatmap
- Model summary printout showing layer shapes and parameter counts

**OUT OF SCOPE:**
- Transfer learning (ResNet, VGG, etc.)
- GPU/TPU-specific configuration
- Model serialization/deployment beyond checkpoint saving
- Hyperparameter search (grid search, Optuna, etc.)
- Test-time augmentation
- Batch normalization (not in the reference architecture)
- Custom training loops (uses `model.fit` / `model.fit_generator`)

**TARGET:** Single `.ipynb` file, ~30-35 cells (markdown + code), 3 model variants

## Architecture

- **Format:** Jupyter Notebook (`.ipynb` — valid JSON, `nbformat` version 4)
- **Language:** Python 3.8+
- **Framework:** TensorFlow 2.x / Keras
- **Key dependencies:**
  - `tensorflow` (>=2.8) — includes `keras` as `tf.keras`
  - `numpy` — array operations
  - `matplotlib` — plotting (training curves, sample images, confusion matrix)
  - `scikit-learn` — classification report and confusion matrix
  - `seaborn` — confusion matrix heatmap

### Notebook Structure

```
cifar10-cnn/
  cifar10_cnn.ipynb        # The complete notebook
  spec.md                  # This spec (reference only)
```

### Cell Layout

```
Cell  1: [markdown] Title — "CIFAR-10 Image Classification with CNNs"
Cell  2: [markdown] Introduction — dataset description, 10 classes, 32x32x3 images
Cell  3: [code]     Imports (numpy, tensorflow/keras, matplotlib, sklearn, seaborn)
Cell  4: [code]     Load dataset — cifar10.load_data()
Cell  5: [code]     Constants — BATCH_SIZE=128, NUM_CLASSES=10, VALIDATION_SPLIT=0.2
Cell  6: [code]     Print shapes — X_train.shape, X_test.shape, sample counts
Cell  7: [code]     Visualize samples — 2x5 grid, one image per class with class name title
Cell  8: [markdown] Section — "Data Preprocessing"
Cell  9: [code]     One-hot encode labels — to_categorical(y_train/y_test, NUM_CLASSES)
Cell 10: [code]     Normalize pixels — astype('float32'), /= 255
Cell 11: [markdown] Section — "Model 1: Simple CNN (Baseline)"
Cell 12: [code]     Build simple model — 1x Conv2D(32,3,3) + MaxPool + Dense(512) + Dense(10)
Cell 13: [code]     Print model.summary()
Cell 14: [code]     Compile — categorical_crossentropy, rmsprop, metrics=['accuracy']
Cell 15: [code]     Train — model.fit(), 20 epochs, BATCH_SIZE=128, validation_split=0.2
Cell 16: [code]     Plot accuracy curves (train vs validation)
Cell 17: [code]     Plot loss curves (train vs validation)
Cell 18: [code]     Evaluate on test set — model.evaluate(), print loss and accuracy
Cell 19: [markdown] Section — "Model 2: Improved CNN (Deeper Architecture)"
Cell 20: [code]     Build improved model function create_cnn_model():
                      Conv2D(32)x2 + MaxPool + Dropout(0.25)
                      Conv2D(64)x2 + MaxPool + Dropout(0.25)
                      Dense(512) + Dropout(0.5) + Dense(10)
Cell 21: [code]     Print model.summary() — expect 1,676,842 total params
Cell 22: [code]     Compile — categorical_crossentropy, rmsprop
Cell 23: [code]     Train — 40 epochs, validation_split=0.2
Cell 24: [code]     Plot accuracy curves
Cell 25: [code]     Plot loss curves
Cell 26: [code]     Evaluate — print test loss and accuracy
Cell 27: [markdown] Section — "Model 3: Data Augmentation"
Cell 28: [code]     ImageDataGenerator — horizontal_flip=True, zoom_range=0.2
Cell 29: [code]     Build fresh model via create_cnn_model(), compile
Cell 30: [code]     LR decay function — lr * 0.1^(epoch//10), initial lr=0.01
Cell 31: [code]     Train with fit_generator/fit — datagen.flow(), callbacks:
                      ModelCheckpoint('model_aug.h5', save_best_only=True)
                      EarlyStopping(monitor='val_accuracy', patience=10)
Cell 32: [code]     Plot accuracy curves
Cell 33: [code]     Plot loss curves
Cell 34: [code]     Evaluate — print test loss and accuracy
Cell 35: [markdown] Section — "Per-Class Evaluation"
Cell 36: [code]     Generate predictions on test set — model.predict(), argmax
Cell 37: [code]     Classification report — sklearn.metrics.classification_report with class_names
Cell 38: [code]     Confusion matrix heatmap — sklearn.metrics.confusion_matrix + seaborn.heatmap
Cell 39: [markdown] Conclusion — compare 3 models, note accuracy progression
```

## Core Components

### 1. Data Loading and Exploration

**Purpose:** Load CIFAR-10 and verify dataset properties.

- Load via `keras.datasets.cifar10.load_data()`
- Returns `(X_train, y_train), (X_test, y_test)`
- X_train shape: `(50000, 32, 32, 3)` — 50K images, 32x32 pixels, 3 color channels (RGB)
- X_test shape: `(10000, 32, 32, 3)` — 10K test images
- y_train/y_test: integer labels 0-9
- Class names list: `['airplane', 'automobile', 'bird', 'cat', 'deer', 'dog', 'frog', 'horse', 'ship', 'truck']`
- Sample visualization: 2x5 subplot grid, one random image per class, class name as subplot title

### 2. Data Preprocessing

**Purpose:** Prepare data for neural network training.

- **One-hot encoding:** `to_categorical(y, 10)` converts integer labels to 10-dimensional binary vectors
- **Normalization:** Convert pixel values from uint8 [0, 255] to float32 [0.0, 1.0] by dividing by 255
- No mean subtraction or per-channel normalization (keep it simple)

### 3. Model 1 — Simple CNN (Baseline)

**Purpose:** Establish baseline accuracy with minimal architecture.

**Architecture:**
```
Conv2D(32, (3,3), padding='same', input_shape=(32,32,3)) -> ReLU
MaxPooling2D(2,2)
Dropout(0.25)
Flatten
Dense(512) -> ReLU
Dropout(0.5)
Dense(10) -> Softmax
```

**Training config:**
- Optimizer: RMSprop (default learning rate)
- Loss: categorical_crossentropy
- Epochs: 20
- Batch size: 128
- Validation split: 0.2 (40K train / 10K validation from training set)

**Expected results:**
- Total params: ~4,200,842
- Test accuracy: ~67% (range: 65-70%)
- Clear overfitting visible in training curves (training acc >> validation acc)

### 4. Model 2 — Improved CNN (Deeper Architecture)

**Purpose:** Improve accuracy with deeper convolutional feature extraction.

**Architecture (defined as `create_cnn_model()` function):**
```
# Block 1
Conv2D(32, (3,3), padding='same', input_shape=(32,32,3)) -> ReLU
Conv2D(32, (3,3), padding='same') -> ReLU
MaxPooling2D(2,2)
Dropout(0.25)

# Block 2
Conv2D(64, (3,3), padding='same') -> ReLU
Conv2D(64, (3,3)) -> ReLU                    # NOTE: no padding — output shrinks
MaxPooling2D(2,2)
Dropout(0.25)

# Classifier
Flatten
Dense(512) -> ReLU
Dropout(0.5)
Dense(10) -> Softmax
```

**Key detail:** The second Conv2D(64) uses default `padding='valid'` (no padding), reducing spatial dimensions from 16x16 to 14x14 before the second MaxPool. This yields a Flatten output of 7x7x64 = 3,136 units.

**Training config:**
- Optimizer: RMSprop
- Loss: categorical_crossentropy
- Epochs: 40
- Batch size: 128
- Validation split: 0.2

**Expected results:**
- Total params: 1,676,842
- Test accuracy: ~78% (range: 76-80%)
- Less overfitting than Model 1 due to regularization (dropout) and efficient architecture

### 5. Model 3 — Data Augmentation

**Purpose:** Further improve generalization using augmented training data.

**Data augmentation (ImageDataGenerator):**
- `horizontal_flip=True` — randomly flip images left-right
- `zoom_range=0.2` — random zoom in/out by up to 20%
- Applied only to training data; test data is not augmented

**Learning rate schedule:**
```python
lr = 0.01
def learning_rate_decay(epoch):
    return lr * (0.1 ** int(epoch / 10))
```
Decays by 10x every 10 epochs: 0.01 -> 0.001 -> 0.0001 -> 0.00001

**Callbacks:**
- `ModelCheckpoint('model_aug.h5', save_best_only=True)` — save best weights by validation loss
- `EarlyStopping(monitor='val_accuracy', patience=10)` — stop if no improvement for 10 epochs

**Training config:**
- Uses `model.fit()` with `datagen.flow(X_train, y_train, batch_size=BATCH_SIZE)`
- `steps_per_epoch = X_train.shape[0] // BATCH_SIZE`
- Validation data: `(X_test, y_test)` — full test set used as validation during augmented training
- Epochs: 40 (may stop early)

**Expected results:**
- Test accuracy: ~80% (range: 78-82%)
- Better generalization (smaller train-val accuracy gap)
- Training typically stops around epoch 30-34 via EarlyStopping

### 6. Training History Visualization

**Purpose:** Plot training dynamics for each model.

For each model, produce two plots:
1. **Accuracy plot:** `history['accuracy']` and `history['val_accuracy']` vs epoch
   - Title: "Model Accuracy"
   - Legend: ['train', 'validation'], upper left
2. **Loss plot:** `history['loss']` and `history['val_loss']` vs epoch
   - Title: "Model Loss"
   - Legend: ['train', 'validation'], upper right

**Implementation notes:**
- Use `matplotlib.pyplot` for all plots
- Modern Keras uses `'accuracy'` and `'val_accuracy'` keys (not `'acc'`/`'val_acc'`)
- Each plot is a separate cell for clear notebook output

### 7. Per-Class Evaluation

**Purpose:** Analyze model performance per class to identify strengths and weaknesses.

**Classification report:**
- Use `sklearn.metrics.classification_report(y_true, y_pred, target_names=class_names)`
- Shows per-class precision, recall, F1-score, and support
- Typically: vehicles (airplane, automobile, ship, truck) classify better than animals (cat, dog, deer)

**Confusion matrix:**
- Compute via `sklearn.metrics.confusion_matrix(y_true, y_pred)`
- Visualize as heatmap via `seaborn.heatmap(cm, annot=True, fmt='d', xticklabels=class_names, yticklabels=class_names)`
- Figure size: approximately 10x8
- Title: "Confusion Matrix"
- Axis labels: "Predicted" (x), "True" (y)

**Common confusion pairs to expect:**
- cat <-> dog (visually similar animals)
- deer <-> horse (four-legged animals)
- automobile <-> truck (vehicles)

## Configuration

### Python Environment

```
tensorflow>=2.8
numpy
matplotlib
scikit-learn
seaborn
```

### Hyperparameters (Constants)

| Parameter | Value | Notes |
|---|---|---|
| BATCH_SIZE | 128 | All models |
| NUM_CLASSES | 10 | Fixed by dataset |
| VALIDATION_SPLIT | 0.2 | Models 1 & 2 (from training set) |
| Simple model epochs | 20 | No early stopping |
| Improved model epochs | 40 | No early stopping |
| Augmented model epochs | 40 | With early stopping (patience=10) |
| Augmentation zoom_range | 0.2 | +/- 20% |
| Augmentation horizontal_flip | True | Random left-right flip |
| LR initial | 0.01 | Augmented model only |
| LR decay | 10x every 10 epochs | Step decay schedule |
| Dropout (conv blocks) | 0.25 | After each MaxPool |
| Dropout (dense) | 0.5 | Before output layer |

### Notebook Metadata

```json
{
  "kernelspec": {
    "display_name": "Python 3",
    "language": "python",
    "name": "python3"
  },
  "language_info": {
    "name": "python",
    "version": "3.10.0"
  }
}
```

**IMPORTANT:** The output must be a valid `.ipynb` file — JSON format conforming to nbformat v4. Each cell must have:
- `"cell_type"`: `"code"` or `"markdown"`
- `"source"`: array of strings (lines of the cell content)
- `"metadata"`: object (can be empty `{}`)
- Code cells additionally need: `"execution_count": null`, `"outputs": []`

## Acceptance Criteria

### Structural

1. Output is a single valid `.ipynb` file that opens without errors in Jupyter Notebook/Lab
2. Notebook contains ~30-39 cells with interleaved markdown and code cells
3. Markdown cells provide clear section headers and explanatory text
4. All code cells are syntactically valid Python 3
5. Notebook uses modern Keras API (`tensorflow.keras` or `keras` with TF backend)

### Data Pipeline

6. CIFAR-10 dataset loads via `keras.datasets.cifar10.load_data()` (no manual download)
7. Training set shape is verified as `(50000, 32, 32, 3)`
8. Test set shape is verified as `(10000, 32, 32, 3)`
9. Labels are one-hot encoded to shape `(N, 10)`
10. Pixel values are normalized to [0.0, 1.0] range via float32 conversion and /255

### Visualization

11. Sample image grid displays one image per class (10 images total) with class name titles
12. Training accuracy plot shows train and validation curves with legend
13. Training loss plot shows train and validation curves with legend
14. Confusion matrix is rendered as an annotated heatmap with class name labels on both axes

### Model Architecture

15. Simple model (Model 1) has exactly: Conv2D(32) -> MaxPool -> Dense(512) -> Dense(10) with dropout
16. Improved model (Model 2) has exactly: 2x Conv2D(32) -> MaxPool -> 2x Conv2D(64) -> MaxPool -> Dense(512) -> Dense(10) with dropout
17. Both models use ReLU activation for hidden layers and Softmax for output
18. `model.summary()` is called and printed for at least the improved model
19. Improved model parameter count is approximately 1,676,842

### Training

20. All models compile with `categorical_crossentropy` loss and `rmsprop` optimizer
21. Simple model trains for 20 epochs with batch_size=128
22. Improved model trains for 40 epochs with batch_size=128
23. Augmented model uses `ImageDataGenerator` with `horizontal_flip=True` and `zoom_range=0.2`
24. Augmented model includes `ModelCheckpoint` callback saving best model
25. Augmented model includes `EarlyStopping` callback with patience >= 10
26. Augmented model includes learning rate decay (step decay every 10 epochs)

### Evaluation

27. Test accuracy is printed for all three models
28. Simple model achieves test accuracy >= 65%
29. Improved model achieves test accuracy >= 75%
30. Augmented model achieves test accuracy >= 78%
31. `sklearn.metrics.classification_report` is generated with all 10 class names
32. `sklearn.metrics.confusion_matrix` is computed and displayed as a heatmap
33. Accuracy improves across the three models (simple < improved < augmented)

### Code Quality

34. Imports are consolidated in a single cell at the top of the notebook
35. Hyperparameters are defined as named constants, not magic numbers inline
36. The improved model architecture is defined as a reusable function (`create_cnn_model()`)
37. No deprecated Keras API calls (use `model.fit()` not `model.fit_generator()` for TF2)
38. History keys use modern naming: `'accuracy'`/`'val_accuracy'` (not `'acc'`/`'val_acc'`)
