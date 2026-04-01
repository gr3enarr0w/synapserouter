with open('assets/logo-ascii-full.txt', 'r') as f:
    lines = f.readlines()
print(f'Total lines: {len(lines)}')
for i, line in enumerate(lines, 1):
    stripped = line.rstrip()
    if stripped:
        print(f'Line {i:3d} (len={len(stripped):3d}): {repr(stripped[:120])}')
    else:
        print(f'Line {i:3d}: (empty)')
