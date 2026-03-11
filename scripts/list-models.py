#!/usr/bin/env python3
"""List models organized by category from Synapse Router."""

import json
import urllib.request
from collections import defaultdict

def main():
    print("🧠 Synapse Router - Available Models by Category")
    print("=" * 70)
    print()

    # Fetch models
    try:
        with urllib.request.urlopen('http://localhost:8090/v1/models') as response:
            data = json.loads(response.read())
            models = data['data']
    except Exception as e:
        print(f"❌ Error: Could not connect to proxy at http://localhost:8090")
        print(f"   Make sure the router is running: ./synroute")
        return 1

    # Group by category
    by_category = defaultdict(list)
    for m in models:
        cat = m.get('category', 'uncategorized')
        by_category[cat].append(m)

    # Category order and display
    categories = [
        ('frontier', '🚀 FRONTIER MODELS - Ultra Long Context'),
        ('code', '💻 CODE SPECIALISTS'),
        ('reasoning', '🧠 REASONING MODELS - Extended Thinking'),
        ('general', '⚡ GENERAL PURPOSE'),
    ]

    for cat_id, cat_name in categories:
        if cat_id in by_category:
            print(f"\n{cat_name}")
            print("─" * 70)
            for m in by_category[cat_id]:
                tokens_str = f"{m['max_tokens']:,}".rjust(10)
                print(f"  {m['id']:<50} {tokens_str} tokens")
                print(f"    └─ {m['description']}")
            print()

    # Summary
    print("\n📊 SUMMARY")
    print("─" * 70)
    for cat_id, cat_name in categories:
        if cat_id in by_category:
            count = len(by_category[cat_id])
            label = cat_name.split(' - ')[0]
            print(f"  {label:45} {count} models")
    print(f"\n  {'TOTAL':45} {len(models)} models")
    print()

    return 0

if __name__ == '__main__':
    exit(main())
