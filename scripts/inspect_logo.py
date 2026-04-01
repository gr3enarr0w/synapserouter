with open('assets/logo-ascii-full.txt', 'r') as f:
    content = f.read()

lines = content.split('\n')
print(f'Total lines: {len(lines)}')
print()
for i, line in enumerate(lines, 1):
    # Find first and last non-space char
    stripped = line.rstrip()
    first_nonspace = len(line) - len(line.lstrip())
    print(f'Line {i:2d}: len={len(stripped):4d} first_nonspace={first_nonspace}')
    # Print the first 120 chars visible
    if stripped:
        print(f'       [{stripped[:120]}]')
