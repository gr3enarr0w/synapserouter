#!/bin/bash

# List models organized by category

echo "🧠 Synapse Router - Available Models by Category"
echo "================================================"
echo ""

curl -s http://localhost:8090/v1/models | python3 << 'EOF'
import json
import sys
from collections import defaultdict

data = json.load(sys.stdin)
models = data['data']

# Group by category
by_category = defaultdict(list)
for m in models:
    cat = m.get('category', 'uncategorized')
    by_category[cat].append(m)

# Category order and display names
categories = [
    ('frontier', '🚀 FRONTIER MODELS - Ultra Long Context'),
    ('code', '💻 CODE SPECIALISTS'),
    ('reasoning', '🧠 REASONING MODELS - Extended Thinking'),
    ('general', '⚡ GENERAL PURPOSE'),
]

for cat_id, cat_name in categories:
    if cat_id in by_category:
        print(f"\n{cat_name}")
        print("=" * 60)
        for m in by_category[cat_id]:
            tokens_str = f"{m['max_tokens']:,}".rjust(10)
            print(f"  {m['id']:<45} {tokens_str} tokens")
            print(f"    └─ {m['description']}")
        print()

# Summary
print("\n📊 SUMMARY")
print("=" * 60)
for cat_id, cat_name in categories:
    if cat_id in by_category:
        count = len(by_category[cat_id])
        print(f"  {cat_name.split(' - ')[0]:40} {count} models")
print(f"\n  {'TOTAL':40} {len(models)} models")
print()
EOF
