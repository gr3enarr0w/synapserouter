# Wine Quality EDA -- Reconstruction Spec

## Overview

Exploratory data analysis of the UCI Wine Quality dataset (red wine variant) using R Markdown. The deliverable is a single `.Rmd` file that renders to a self-contained HTML document containing 12 ggplot2 visualizations, statistical summaries, outlier detection, and narrative interpretation of findings.

Dataset: Portuguese "Vinho Verde" red wine (Cortez et al., 2009). 1,599 samples, 11 physicochemical input features + 1 sensory output (quality score 0-10).

**Input features:**
1. fixed.acidity (tartaric acid, g/dm^3)
2. volatile.acidity (acetic acid, g/dm^3)
3. citric.acid (g/dm^3)
4. residual.sugar (g/dm^3)
5. chlorides (sodium chloride, g/dm^3)
6. free.sulfur.dioxide (mg/dm^3)
7. total.sulfur.dioxide (mg/dm^3)
8. density (g/cm^3)
9. pH
10. sulphates (potassium sulphate, g/dm^3)
11. alcohol (% by volume)

**Output:** quality (integer score, 0-10, median of >= 3 expert evaluations)

---

## Scope

### IN SCOPE

- Single R Markdown file (`wine_quality_eda.Rmd`) targeting HTML output
- Download/load the red wine dataset from UCI ML Repository CSV
- 12 specific ggplot2 visualizations (enumerated below)
- Summary statistics: mean, median, standard deviation, min, max, quartiles for all 12 variables
- Correlation matrix of all numeric features
- Outlier detection using IQR method (1.5x IQR rule) with reporting
- Narrative text interpreting each visualization and statistical finding
- Conclusions section synthesizing key findings
- Reproducible: knitting the .Rmd produces the complete HTML with no manual steps

### OUT OF SCOPE

- White wine dataset (reference used white; this spec targets red)
- Predictive modeling (random forest, SVM, neural nets) -- EDA only
- Interactive widgets (Shiny, plotly) -- static ggplot2 only
- PDF or Word output formats
- Feature engineering beyond what is needed for the 12 plots
- Cross-validation, train/test splits, or model evaluation metrics
- Deployment or hosting of the rendered HTML
- Custom CSS/themes beyond default R Markdown HTML styling

---

## Architecture

### Rmd Structure

The document follows standard R Markdown structure with YAML frontmatter, setup chunk, and sequential analysis sections.

```
wine_quality_eda.Rmd
|
+-- YAML Header
|   - title, author, date, output: html_document
|
+-- Setup Chunk
|   - library() calls
|   - knitr::opts_chunk$set() defaults
|   - Data loading (read.csv from UCI URL or local file)
|
+-- Section 1: Data Overview
|   - str(), dim(), head()
|   - Summary statistics table
|   - Missing value check
|
+-- Section 2: Univariate Analysis (Plots 1-2, 10)
|   - Plot 1: Quality score distribution
|   - Plot 2: Alcohol content histogram
|   - Plot 10: Residual sugar distribution (log-scaled)
|
+-- Section 3: Bivariate Analysis (Plots 3, 5, 6, 7)
|   - Plot 3: Quality vs alcohol boxplot
|   - Plot 5: Alcohol vs density scatter (colored by quality)
|   - Plot 6: Quality vs volatile acidity violin
|   - Plot 7: Mean alcohol by quality group bar chart
|
+-- Section 4: Correlation Analysis (Plot 4)
|   - Plot 4: Correlation heatmap of all features
|   - Identify top correlated pairs
|
+-- Section 5: Multivariate Analysis (Plots 8, 9, 11)
|   - Plot 8: Pairs plot of top 5 correlated features
|   - Plot 9: Citric acid density plot by quality category
|   - Plot 11: Scatter matrix -- sulphates vs chlorides vs pH
|
+-- Section 6: Regression Analysis (Plot 12)
|   - Plot 12: Linear regression quality ~ alcohol + volatile.acidity
|   - Model summary, coefficients, R-squared
|   - Residual diagnostics (fitted vs residuals plot)
|
+-- Section 7: Outlier Detection
|   - IQR-based outlier identification per feature
|   - Summary table: feature, count of outliers, percentage
|   - Discussion of which outliers are genuine vs data errors
|
+-- Section 8: Conclusions
|   - Key findings (3-5 bullets)
|   - Strongest predictors of quality
|   - Limitations of the dataset
|   - Suggestions for further analysis
```

### Data Flow

1. Load CSV (UCI URL with local fallback: `winequality-red.csv`)
2. Validate: confirm 1599 rows, 12 columns, no NAs
3. Create derived columns:
   - `quality.factor`: quality as ordered factor (for categorical plots)
   - `quality.category`: "low" (3-4), "medium" (5-6), "high" (7-8) grouping
4. Compute summary statistics and correlation matrix
5. Generate each visualization in its own named chunk
6. Fit linear model and extract diagnostics
7. Run IQR outlier detection across all features
8. Render narrative and conclusions

---

## Core Components

### Plot 1: Distribution of Quality Scores

- **Type:** Bar chart (geom_bar)
- **X-axis:** quality score (as factor, discrete)
- **Y-axis:** count
- **Fill:** gradient by count or single color
- **Narrative:** Discuss the concentration around 5-6, near-normal shape, class imbalance at extremes
- **Chunk name:** `plot-quality-distribution`

### Plot 2: Histogram of Alcohol Content

- **Type:** Histogram (geom_histogram)
- **X-axis:** alcohol (% by volume)
- **Y-axis:** count
- **Binwidth:** 0.2
- **Fill:** warm gradient (low to high count)
- **Narrative:** Note right skew, peak around 9.5%, range discussion
- **Chunk name:** `plot-alcohol-histogram`

### Plot 3: Boxplot -- Quality vs Alcohol

- **Type:** Boxplot with jittered points (geom_boxplot + geom_jitter)
- **X-axis:** quality (as factor)
- **Y-axis:** alcohol
- **Extras:** stat_summary for mean point (blue star), outliers in red
- **Narrative:** Higher quality wines tend toward higher alcohol; compute and display correlation coefficient
- **Chunk name:** `plot-quality-alcohol-boxplot`

### Plot 4: Correlation Heatmap

- **Type:** Tile-based heatmap (geom_tile or corrplot)
- **Data:** cor() of all 11 input features + quality
- **Color scale:** diverging (blue-white-red), range -1 to 1
- **Labels:** correlation coefficients displayed in cells
- **Narrative:** Identify strongest positive and negative correlations; highlight pairs |r| > 0.5
- **Chunk name:** `plot-correlation-heatmap`

### Plot 5: Scatter -- Alcohol vs Density, Colored by Quality

- **Type:** Scatter (geom_point)
- **X-axis:** alcohol
- **Y-axis:** density
- **Color:** quality (as factor, sequential palette)
- **Alpha:** 0.6 for overplotting
- **Extras:** geom_smooth(method="lm") optional trend line
- **Narrative:** Inverse relationship alcohol-density; higher quality clusters at high-alcohol/low-density
- **Chunk name:** `plot-alcohol-density-scatter`

### Plot 6: Violin Plot -- Quality vs Volatile Acidity

- **Type:** Violin with inner boxplot (geom_violin + geom_boxplot width=0.1)
- **X-axis:** quality (as factor)
- **Y-axis:** volatile.acidity
- **Fill:** quality factor with sequential palette
- **Narrative:** Higher volatile acidity associated with lower quality; discuss vinegar taste threshold
- **Chunk name:** `plot-quality-volatile-acidity-violin`

### Plot 7: Bar Chart -- Mean Alcohol by Quality Group

- **Type:** Bar chart (geom_col) with error bars (geom_errorbar for +/- 1 SD)
- **X-axis:** quality (as factor)
- **Y-axis:** mean alcohol
- **Data:** Pre-aggregated with dplyr (group_by quality, summarise mean and sd)
- **Fill:** sequential palette by quality
- **Narrative:** Clear upward trend from quality 3 to 8; error bars show variance
- **Chunk name:** `plot-mean-alcohol-by-quality`

### Plot 8: Pairs Plot of Top 5 Correlated Features

- **Type:** Pairs/scatterplot matrix (GGally::ggpairs)
- **Features:** Select 5 features with highest absolute correlation to quality
- **Lower triangle:** scatter with smooth
- **Upper triangle:** correlation coefficients
- **Diagonal:** density plots
- **Narrative:** Identify which feature pairs show strongest joint relationships
- **Chunk name:** `plot-pairs-top5`

### Plot 9: Density Plot -- Citric Acid by Quality Category

- **Type:** Overlapping density (geom_density)
- **X-axis:** citric.acid
- **Color/fill:** quality.category ("low", "medium", "high"), alpha=0.4
- **Narrative:** Higher quality wines tend to have slightly higher citric acid (freshness); discuss distribution shapes
- **Chunk name:** `plot-citric-acid-density`

### Plot 10: Residual Sugar Distribution (Log-Scaled)

- **Type:** Histogram + boxplot side-by-side (gridExtra::grid.arrange)
- **Left panel:** Boxplot of residual.sugar with jittered points
- **Right panel:** Histogram with scale_x_log10
- **Narrative:** Right-skewed distribution; log transform reveals near-normal shape; discuss outlier at max
- **Chunk name:** `plot-residual-sugar-log`

### Plot 11: Scatter Matrix -- Sulphates vs Chlorides vs pH

- **Type:** Three pairwise scatters arranged in grid (gridExtra or patchwork)
- **Panels:** sulphates vs chlorides, sulphates vs pH, chlorides vs pH
- **Color:** quality.category
- **Alpha:** 0.5
- **Extras:** geom_smooth(method="loess") per panel
- **Narrative:** Examine mineral/acidity relationships; note any quality-dependent clustering
- **Chunk name:** `plot-sulphates-chlorides-ph`

### Plot 12: Linear Regression -- quality ~ alcohol + volatile.acidity

- **Type:** Two-panel figure
- **Panel 1:** Actual vs predicted scatter (geom_point + geom_abline for perfect prediction line)
- **Panel 2:** Residuals vs fitted (geom_point + geom_hline at 0)
- **Model:** `lm(quality ~ alcohol + volatile.acidity, data = wine)`
- **Output:** Print summary() showing coefficients, R-squared, p-values
- **Narrative:** Both predictors significant; R-squared modest (~0.3); discuss limitations of linear model for ordinal response
- **Chunk name:** `plot-linear-regression`

### Statistical Summaries

- **Descriptive stats table:** All 12 variables with mean, median, SD, min, max, Q1, Q3
- **Format:** knitr::kable() or kableExtra for styled HTML table
- **Correlation matrix:** Full 12x12 matrix printed and discussed
- **Per-quality-group stats:** Mean of key features (alcohol, volatile.acidity, sulphates, citric.acid) by quality level

### Outlier Detection

- **Method:** IQR rule -- values below Q1 - 1.5*IQR or above Q3 + 1.5*IQR
- **Output:** Table with columns: Feature, N_Outliers, Pct_Outliers, Min_Outlier, Max_Outlier
- **Discussion:** Which features have the most outliers; whether extreme values are plausible (e.g., residual sugar max) or likely data errors

---

## Configuration

### Required R Packages

```r
# Core
library(ggplot2)        # All visualizations
library(dplyr)          # Data manipulation, summarise, group_by
library(tidyr)          # Reshaping for heatmap data

# Correlation and pairs
library(corrplot)       # Alternative correlation visualization (optional)
library(GGally)         # ggpairs for Plot 8
library(reshape2)       # melt() for correlation heatmap tile plot

# Layout
library(gridExtra)      # grid.arrange for multi-panel plots (Plots 10, 11)

# Tables
library(knitr)          # kable() for summary tables
library(kableExtra)     # Styled HTML tables (optional enhancement)

# Color
library(RColorBrewer)   # Color palettes
library(viridis)        # Colorblind-friendly palettes (optional)
```

### R Version

- Minimum: R >= 4.0.0
- Tested with: R 4.3.x or 4.4.x

### knitr Chunk Defaults

```r
knitr::opts_chunk$set(
  echo = FALSE,
  warning = FALSE,
  message = FALSE,
  fig.width = 10,
  fig.height = 6,
  fig.align = "center",
  cache = FALSE
)
```

### Dataset Source

- **Primary:** `https://archive.ics.uci.edu/ml/machine-learning-databases/wine-quality/winequality-red.csv`
- **Separator:** semicolon (`;`)
- **Fallback:** Local file `winequality-red.csv` in same directory as .Rmd
- **Load pattern:**
  ```r
  wine <- tryCatch(
    read.csv("https://archive.ics.uci.edu/ml/machine-learning-databases/wine-quality/winequality-red.csv", sep = ";"),
    error = function(e) read.csv("winequality-red.csv", sep = ";")
  )
  ```

---

## Acceptance Criteria

### Rendering

- [ ] `wine_quality_eda.Rmd` knits to HTML without errors using `rmarkdown::render()`
- [ ] Output HTML is self-contained (no external dependencies needed to view)
- [ ] All R chunks execute without warnings or errors in the rendered output

### Data

- [ ] Dataset loads successfully (1,599 rows, 12 columns)
- [ ] No hardcoded file paths -- uses URL with local fallback
- [ ] Derived columns (quality.factor, quality.category) created correctly

### Visualizations (12 required)

- [ ] Plot 1: Quality score bar chart -- all quality levels (3-8) shown, counts labeled or readable
- [ ] Plot 2: Alcohol histogram -- binwidth ~0.2, fill gradient applied
- [ ] Plot 3: Quality vs alcohol boxplot -- jittered points visible, mean marked
- [ ] Plot 4: Correlation heatmap -- all 12 variables, coefficients in cells, diverging color scale
- [ ] Plot 5: Alcohol vs density scatter -- points colored by quality factor, legend present
- [ ] Plot 6: Quality vs volatile acidity violin -- inner boxplot visible, one violin per quality level
- [ ] Plot 7: Mean alcohol by quality bar chart -- error bars (+/- 1 SD), ascending trend visible
- [ ] Plot 8: Pairs plot of top 5 features -- scatters + densities + correlations via GGally::ggpairs
- [ ] Plot 9: Citric acid density by quality category -- 3 overlapping densities (low/medium/high)
- [ ] Plot 10: Residual sugar log-scaled -- two-panel layout (boxplot + histogram), log10 x-axis on histogram
- [ ] Plot 11: Sulphates/chlorides/pH scatter matrix -- 3 pairwise panels, colored by quality category
- [ ] Plot 12: Linear regression -- model summary printed, actual vs predicted and residual plots shown

### Statistical Content

- [ ] Descriptive statistics table for all 12 variables (mean, median, SD, min, max, Q1, Q3)
- [ ] Full correlation matrix computed and displayed
- [ ] Per-quality-group summary statistics for key features
- [ ] Linear model coefficients, R-squared, and p-values reported

### Outlier Detection

- [ ] IQR-based outlier counts computed for every feature
- [ ] Summary table listing feature, outlier count, and percentage
- [ ] Narrative discussion of outlier significance

### Narrative

- [ ] Each plot has accompanying interpretation text (minimum 2 sentences)
- [ ] Conclusions section with at least 3 key findings
- [ ] Limitations of the dataset acknowledged
- [ ] Suggestions for further analysis provided

### Code Quality

- [ ] All chunks have descriptive names (no unnamed chunks)
- [ ] echo=FALSE for all chunks (code hidden in output)
- [ ] No hardcoded absolute paths
- [ ] Package installation not performed inside the Rmd (assume packages are installed)

---

## Citation

P. Cortez, A. Cerdeira, F. Almeida, T. Matos and J. Reis.
Modeling wine preferences by data mining from physicochemical properties.
In Decision Support Systems, Elsevier, 47(4):547-553, 2009. ISSN: 0167-9236.
