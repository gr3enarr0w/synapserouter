from PIL import Image
import sys

img = Image.open('assets/synroute-logo.png')
w, h = img.size
print(f"Image: {w}x{h}, mode={img.mode}")

# Convert to ASCII representation at reduced resolution
block_w = w // 80
block_h = h // 40

for by in range(40):
    row = []
    for bx in range(80):
        x = bx * block_w + block_w // 2
        y = by * block_h + block_h // 2
        if x < w and y < h:
            r, g, b, a = img.getpixel((x, y))
            brightness = (r + g + b) / 3
            if a < 50:
                row.append(' ')
            elif brightness > 200:
                row.append(' ')
            elif brightness > 150:
                row.append('.')
            elif brightness > 100:
                row.append('+')
            elif brightness > 50:
                row.append('#')
            else:
                row.append('@')
    print(''.join(row))

print("\n--- Color samples ---")
for y_pct in [20, 30, 40, 50, 60, 70]:
    y = int(h * y_pct / 100)
    colors = []
    for x_pct in range(10, 90, 5):
        x = int(w * x_pct / 100)
        r, g, b, a = img.getpixel((x, y))
        if a > 50 and (r + g + b) / 3 < 200:
            colors.append(f"({r},{g},{b})")
    if colors:
        print(f"y={y_pct}%: {' '.join(colors[:8])}")
