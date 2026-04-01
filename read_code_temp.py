with open('/Users/ceverson/Development/synapserouter/internal/agent/coderenderer.go') as f:
    content = f.read()

# Find the Init function section
start = content.find('func (cr *CodeRenderer) Init()')
end = content.find('\nfunc ', start + 1)
section = content[start:end]
print(section)
