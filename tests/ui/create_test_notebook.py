#!/usr/bin/env python3
"""Create a minimal Jupyter notebook for testing."""
import json, sys
nb = {
    "cells": [
        {"cell_type": "markdown", "source": ["# Test Notebook\n", "Review and fill in the code cell below."], "metadata": {}},
        {"cell_type": "code", "source": ["# Write your code here\n"], "metadata": {}, "outputs": [], "execution_count": None}
    ],
    "metadata": {"kernelspec": {"name": "python3", "display_name": "Python 3", "language": "python"},
                 "language_info": {"name": "python", "version": "3.12.0"}},
    "nbformat": 4, "nbformat_minor": 5
}
path = sys.argv[1] if len(sys.argv) > 1 else "test.ipynb"
json.dump(nb, open(path, "w"), indent=1)
print(f"Created {path}")
