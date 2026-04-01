#!/bin/bash
# Overnight reconstruction tests — runs sequentially through synapserouter

echo "=== Starting overnight tests: $(date) ==="

# Test 1: CIFAR-10 CNN (Jupyter notebook)
echo ""
echo "========== TEST 1: CIFAR-10 CNN (.ipynb) =========="
echo "Start: $(date)"
cd /Users/ceverson/Development/cifar10-cnn
/Users/ceverson/Development/synapserouter/synroute chat --spec-file spec.md --message "Build the CIFAR-10 CNN notebook from the spec. Create a valid .ipynb Jupyter notebook file with all cells." 2>&1 | tee /Users/ceverson/Development/cifar10-cnn/run_output.log
echo "End: $(date)"
echo "Files created:"
find . -not -name spec.md -not -path "./.synroute/*" -not -name "run_output.log" -type f

# Test 2: Wine Quality EDA (R Markdown)
echo ""
echo "========== TEST 2: Wine Quality EDA (.Rmd) =========="
echo "Start: $(date)"
cd /Users/ceverson/Development/wine-quality-eda
/Users/ceverson/Development/synapserouter/synroute chat --spec-file spec.md --message "Build the wine quality EDA from the spec. Create a valid .Rmd R Markdown file with all R code chunks and narrative text." 2>&1 | tee /Users/ceverson/Development/wine-quality-eda/run_output.log
echo "End: $(date)"
echo "Files created:"
find . -not -name spec.md -not -path "./.synroute/*" -not -name "run_output.log" -type f

echo ""
echo "=== All tests complete: $(date) ==="
