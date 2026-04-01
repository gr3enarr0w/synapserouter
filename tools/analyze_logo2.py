from PIL import Image

img = Image.open('assets/synroute-logo.png')
w, h = img.size
print(f"Image: {w}x{h}, mode={img.mode}")

# Use smaller blocks for more detail
cols, rows = 100, 50
block_w = w // cols
block_h = h // rows

for by in range(rows):
    row_chars = []
    for bx in range(cols):
        x = bx * block_w + block_w // 2
        y = by * block_h + block_h // 2
        if x < w and y < h:
            r, g, b, a = img.getpixel((x, y))
            if a < 30 or (r > 240 and g > 240 and b > 240):
                row_chars.append(' ')
            elif r > 220 and g > 220 and b > 220:
                row_chars.append('.')
            else:
                # Show as character based on dominant color
                if b > r and b > g:
                    row_chars.append('B')  # blue/cyan
                elif r > b and r > g:
                    row_chars.append('R')  # red/magenta
                elif g > r:
                    row_chars.append('G')  # green/cyan
                else:
                    row_chars.append('#')
    line = ''.join(row_chars).rstrip()
    if line.strip():
        print(f"{by:2d}: {line}")

# Detailed color map of non-white areas
print("\n=== Detailed color scan ===")
for y in range(0, h, h // 30):
    for x in range(0, w, w // 60):
        r, g, b, a = img.getpixel((x, y))
        if a > 30 and not (r > 230 and g > 230 and b > 230):
            print(f"  px({x},{y}): rgb({r},{g},{b}) a={a}")
