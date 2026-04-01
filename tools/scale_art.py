#!/usr/bin/env python3
"""Extract brain and text art, scale for side-by-side 120-col banner, output as Go string literals."""
with open('assets/logo-ascii-full.txt') as f:
    lines = f.readlines()

# Brain: lines 16-50 (1-indexed)
brain_raw = [l.rstrip() for l in lines[15:50]]
min_indent_b = min(len(l) - len(l.lstrip()) for l in brain_raw if l.strip())
brain = [l[min_indent_b:] for l in brain_raw]
# Find actual rightmost content
max_w_b = max(len(l.rstrip()) for l in brain)

# Text: lines 54-61 (1-indexed) 
text_raw = [l.rstrip() for l in lines[53:61]]
min_indent_t = min(len(l) - len(l.lstrip()) for l in text_raw if l.strip())
text = [l[min_indent_t:] for l in text_raw]
max_w_t = max(len(l.rstrip()) for l in text)

print(f"Brain: {len(brain)} lines, max_width={max_w_b}")
print(f"Text: {len(text)} lines, max_width={max_w_t}")
print()

# Scale brain horizontally to ~50 chars
brain_scale = 50 / max_w_b
print(f"Brain horizontal scale: {brain_scale:.2f}")

# Scale text horizontally to ~65 chars  
text_scale = 65 / max_w_t
print(f"Text horizontal scale: {text_scale:.2f}")

# For brain: take every line but compress horizontally
def hscale(line, factor, target_w):
    """Horizontally scale a string by sampling characters."""
    if not line.rstrip():
        return " " * target_w
    padded = line.ljust(int(target_w / factor) + 1)
    result = []
    for i in range(target_w):
        src_idx = int(i / factor)
        if src_idx < len(padded):
            result.append(padded[src_idx])
        else:
            result.append(' ')
    return ''.join(result).rstrip()

print("\n=== BRAIN SCALED ===")
brain_scaled = []
for l in brain:
    s = hscale(l, brain_scale, 50)
    brain_scaled.append(s)
    print(f"|{s}|")

print("\n=== TEXT SCALED ===")
text_scaled = []
for l in text:
    s = hscale(l, text_scale, 65)
    text_scaled.append(s)
    print(f"|{s}|")

# Now output as Go string literals for side-by-side
# Brain is 35 lines, text is 8 lines
# Pad text to 35 lines, center vertically
pad_top = (len(brain_scaled) - len(text_scaled)) // 2
text_padded = [""] * pad_top + text_scaled + [""] * (len(brain_scaled) - len(text_scaled) - pad_top)

print(f"\n=== SIDE BY SIDE (brain_w=50, gap=2, text_w=65, total=117) ===")
for i in range(len(brain_scaled)):
    b = brain_scaled[i].ljust(50)
    t = text_padded[i] if i < len(text_padded) else ""
    combined = b + "  " + t
    print(f"|{combined.rstrip()}|")

print("\n=== GO STRING LITERALS ===")
for i in range(len(brain_scaled)):
    b = brain_scaled[i].ljust(50)
    t = text_padded[i] if i < len(text_padded) else ""
    combined = b + "  " + t
    # Escape backslashes for Go
    escaped = combined.rstrip().replace('\\', '\\\\').replace('"', '\\"')
    print(f'\t\t"{escaped}",')
