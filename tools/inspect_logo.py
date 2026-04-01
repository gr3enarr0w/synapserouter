with open('assets/logo-ascii-full.txt', 'rb') as f:
    content = f.read()

lines = content.split(b'\n')
print(f'Total lines: {len(lines)}')

# Show all non-blank lines with full content
for i, line in enumerate(lines, 1):
    line_str = line.rstrip(b' \r')
    if line_str.strip():
        print(f'L{i:2d}: {line_str.decode("latin-1")}')
