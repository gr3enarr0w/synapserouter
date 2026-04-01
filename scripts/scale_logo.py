#!/usr/bin/env python3
"""Extract brain (lines 16-50) and text (lines 54-61) from logo-ascii-full.txt,
scale both to same height, fit side-by-side in 120 chars, output as Go string literals."""

import sys

with open('assets/logo-ascii-full.txt', 'r') as f:
    all_lines = f.readlines()

# Extract sections (1-indexed to 0-indexed)
brain_lines = [l.rstrip('\n') for l in all_lines[15:50]]  # lines 16-50
text_lines = [l.rstrip('\n') for l in all_lines[53:61]]   # lines 54-61

# Strip trailing spaces, find content bounds
def strip_and_bound(lines):
    stripped = [l.rstrip() for l in lines]
    # Find leftmost non-space
    min_left = 999
    max_right = 0
    for l in stripped:
        if l.strip():
            left = len(l) - len(l.lstrip())
            min_left = min(min_left, left)
            max_right = max(max_right, len(l))
    # Trim left margin
    trimmed = [l[min_left:] if len(l) > min_left else '' for l in stripped]
    max_w = max(len(l) for l in trimmed)
    return trimmed, max_w

brain, brain_w = strip_and_bound(brain_lines)
text, text_w = strip_and_bound(text_lines)

print(f"Brain: {len(brain)} lines x {brain_w} chars wide")
print(f"Text:  {len(text)} lines x {text_w} chars wide")

# Remove empty leading/trailing lines from brain
while brain and not brain[0].strip():
    brain.pop(0)
while brain and not brain[-1].strip():
    brain.pop()

while text and not text[0].strip():
    text.pop(0)
while text and not text[-1].strip():
    text.pop()

print(f"Brain trimmed: {len(brain)} lines x {brain_w} chars wide")
print(f"Text trimmed:  {len(text)} lines x {text_w} chars wide")

# Target: side by side in 120 chars with 4-char gap
# Brain target: ~52 chars wide, Text target: ~64 chars wide
# Scale horizontally by sampling columns
GAP = 4
TOTAL_W = 116  # leave some margin

# Scale factor for brain
brain_target_w = 50
text_target_w = TOTAL_W - brain_target_w - GAP

def scale_horizontal(lines, orig_w, target_w):
    """Sample columns to scale width."""
    result = []
    for line in lines:
        # Pad to orig_w
        padded = line.ljust(orig_w)
        # Sample target_w columns evenly
        scaled = ''
        for i in range(target_w):
            src_col = int(i * orig_w / target_w)
            scaled += padded[src_col]
        result.append(scaled.rstrip())
    return result

def scale_vertical(lines, target_h):
    """Sample rows to scale height."""
    orig_h = len(lines)
    if orig_h <= target_h:
        return lines
    result = []
    for i in range(target_h):
        src_row = int(i * orig_h / target_h)
        result.append(lines[src_row])
    return result

# Scale brain vertically to match a reasonable height
# Brain is 35 lines, text is 8 lines
# Target height: let's try 14 lines (a good terminal banner size)
TARGET_H = 14

brain_scaled = scale_vertical(brain, TARGET_H)
brain_scaled = scale_horizontal(brain_scaled, brain_w, brain_target_w)

text_scaled = scale_vertical(text, TARGET_H)  # will pad since text < 14
# Actually text has only 8 lines, so pad it to TARGET_H with centering
text_h_scaled = scale_horizontal(text, text_w, text_target_w)
# Vertically center the text
pad_top = (TARGET_H - len(text_h_scaled)) // 2
pad_bot = TARGET_H - len(text_h_scaled) - pad_top
text_final = [''] * pad_top + text_h_scaled + [''] * pad_bot

print(f"\nScaled brain: {len(brain_scaled)} lines x target {brain_target_w}")
print(f"Scaled text:  {len(text_final)} lines x target {text_target_w}")

# Combine side by side
print(f"\n--- Combined banner ({TARGET_H} lines) ---")
combined = []
for i in range(TARGET_H):
    b = brain_scaled[i] if i < len(brain_scaled) else ''
    t = text_final[i] if i < len(text_final) else ''
    row = b.ljust(brain_target_w) + ' ' * GAP + t
    combined.append(row)
    print(f'  [{row}]  len={len(row)}')

# Output as Go string literals
print("\n--- Go code ---")
print('bannerArt := []string{')
for row in combined:
    # Escape backticks if any
    escaped = row.replace('\\', '\\\\').replace('"', '\\"')
    print(f'    "{escaped}",')
print('}')
