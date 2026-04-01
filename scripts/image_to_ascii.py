#!/usr/bin/env python3
"""Convert an image to ASCII art — optimized for logos with transparent/white backgrounds."""
import sys
from PIL import Image

def image_to_ascii_alpha(path, width=40):
    """Convert image using alpha channel — non-transparent pixels become visible."""
    img = Image.open(path).convert("RGBA")

    w, h = img.size
    aspect = h / w
    new_h = int(width * aspect * 0.45)
    img = img.resize((width, new_h))

    pixels = list(img.getdata())

    lines = []
    for y in range(new_h):
        line = []
        for x in range(width):
            r, g, b, a = pixels[y * width + x]
            if a < 50:  # transparent
                line.append(' ')
            else:
                # Use brightness to pick character
                brightness = (r + g + b) / 3
                if brightness > 200:
                    line.append('·')  # light
                elif brightness > 150:
                    line.append('○')
                elif brightness > 100:
                    line.append('●')
                else:
                    line.append('█')

        lines.append("".join(line).rstrip())

    while lines and not lines[0].strip():
        lines.pop(0)
    while lines and not lines[-1].strip():
        lines.pop()

    return "\n".join(lines)


def image_to_ascii_color(path, width=40):
    """Convert image preserving color as ANSI — for terminal preview."""
    img = Image.open(path).convert("RGBA")

    w, h = img.size
    aspect = h / w
    new_h = int(width * aspect * 0.45)
    img = img.resize((width, new_h))

    pixels = list(img.getdata())

    lines = []
    for y in range(new_h):
        line = []
        for x in range(width):
            r, g, b, a = pixels[y * width + x]
            if a < 50:
                line.append(' ')
            else:
                brightness = (r + g + b) / 3
                if brightness > 200:
                    ch = '·'
                elif brightness > 150:
                    ch = '○'
                elif brightness > 100:
                    ch = '●'
                else:
                    ch = '█'
                line.append(f"\033[38;2;{r};{g};{b}m{ch}\033[0m")

        lines.append("".join(line).rstrip())

    while lines and not lines[0].strip():
        lines.pop(0)
    while lines and not lines[-1].strip():
        lines.pop()

    return "\n".join(lines)


if __name__ == "__main__":
    path = sys.argv[1] if len(sys.argv) > 1 else "assets/synroute-logo.png"
    width = int(sys.argv[2]) if len(sys.argv) > 2 else 40

    print(f"=== ALPHA-BASED (width={width}) ===")
    print(image_to_ascii_alpha(path, width))
    print()
    print(f"=== WITH COLOR (width={width}) ===")
    print(image_to_ascii_color(path, width))
    print()
    # Try different widths
    for w in [25, 30, 35]:
        print(f"=== ALPHA (width={w}) ===")
        print(image_to_ascii_alpha(path, w))
        print()
