with open('assets/logo-ascii-full.txt') as f:
    lines = f.readlines()

print('=== BRAIN (lines 16-50) ===')
brain = lines[15:50]  # 0-indexed: lines 16-50
for i, l in enumerate(brain):
    stripped = l.rstrip('\n')
    content = stripped.strip()
    lstrip = len(stripped) - len(stripped.lstrip())
    rstrip_len = len(stripped) - len(stripped.rstrip())
    content_len = len(stripped) - lstrip - rstrip_len
    print(f'{i+16:3d} (indent={lstrip:3d}, content_len={content_len:3d}): {content[:120]}')

print()
print('=== TEXT (lines 54-61) ===')
text = lines[53:61]  # 0-indexed: lines 54-61
for i, l in enumerate(text):
    stripped = l.rstrip('\n')
    content = stripped.strip()
    lstrip = len(stripped) - len(stripped.lstrip())
    rstrip_len = len(stripped) - len(stripped.rstrip())
    content_len = len(stripped) - lstrip - rstrip_len
    print(f'{i+54:3d} (indent={lstrip:3d}, content_len={content_len:3d}): {content[:150]}')
