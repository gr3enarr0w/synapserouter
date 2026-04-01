with open('assets/logo-ascii-full.txt') as f:
    lines = f.readlines()
# Brain lines 16-50 (1-indexed) = [15:50] 0-indexed
brain = [l.rstrip() for l in lines[15:50]]
# Find common left indent
min_indent = min(len(l) - len(l.lstrip()) for l in brain if l.strip())
brain_stripped = [l[min_indent:].rstrip() for l in brain]
max_brain_w = max(len(l) for l in brain_stripped)
print(f'Brain: {len(brain_stripped)} lines, max_width={max_brain_w}, min_indent={min_indent}')
for i,l in enumerate(brain_stripped):
    print(f'B{i:02d} ({len(l):3d}): |{l}|')
print()
# Text lines 54-61 (1-indexed) = [53:61] 0-indexed  
text = [l.rstrip() for l in lines[53:61]]
min_indent_t = min(len(l) - len(l.lstrip()) for l in text if l.strip())
text_stripped = [l[min_indent_t:].rstrip() for l in text]
max_text_w = max(len(l) for l in text_stripped)
print(f'Text: {len(text_stripped)} lines, max_width={max_text_w}, min_indent={min_indent_t}')
for i,l in enumerate(text_stripped):
    print(f'T{i:02d} ({len(l):3d}): |{l}|')
