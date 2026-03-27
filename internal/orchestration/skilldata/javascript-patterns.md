---
name: javascript-patterns
description: "Modern JS/TS development — React, Node.js, TypeScript-first, Next.js patterns."
triggers:
  - "javascript"
  - "typescript"
  - ".js"
  - ".ts"
  - ".tsx"
  - ".jsx"
  - "react"
  - "node.js"
  - "nodejs"
  - "next.js"
  - "nextjs"
role: coder
phase: analyze
language: javascript
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "README exists"
    command: "test -f README.md && echo 'OK' || echo 'MISSING'"
    expect_not: "MISSING"
---
# Skill: JavaScript & TypeScript Patterns

Modern JS/TS development — React, Node.js, TypeScript-first, Next.js patterns.

Source: [React Best Practices](https://mcpmarket.com/tools/skills/react-performance-best-practices-6), [TypeScript Best Practices](https://lobehub.com/skills/0xbigboss-claude-code-typescript-best-practices).

---

## When to Use

- Writing React components or hooks
- TypeScript type design
- Node.js backend development
- Next.js app router patterns

---

## Core Rules

1. **TypeScript-first** — strict mode, no `any`, explicit return types
2. **Functional components** — hooks over class components
3. **Immutability** — spread/map/filter, never mutate state directly
4. **Discriminated unions** — `type Result = { ok: true; data: T } | { ok: false; error: E }`
5. **Zod for validation** — runtime validation matching TypeScript types
6. **Async/await** — over raw promises, with proper error handling
7. **ESM modules** — `import/export` not `require/module.exports`

---

## Patterns

### React component with TypeScript
```tsx
interface ItemCardProps {
  item: Item;
  onSelect: (key: string) => void;
}

export function ItemCard({ item, onSelect }: ItemCardProps) {
  return (
    <div onClick={() => onSelect(ticket.key)}>
      <h3>{ticket.summary}</h3>
      <span>{ticket.status}</span>
    </div>
  );
}
```

### Custom hook
```tsx
function useItems(projectKey: string) {
  const [tickets, setItems] = useState<Item[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetch(`/api/tickets?project=${projectKey}`)
      .then(res => res.json())
      .then(setItems)
      .finally(() => setLoading(false));
  }, [projectKey]);

  return { tickets, loading };
}
```

### Zod schema validation
```ts
import { z } from "zod";

const ItemSchema = z.object({
  key: z.string(),
  summary: z.string().min(1),
  status: z.enum(["Open", "Closed", "Resolved"]),
  confidence: z.number().min(0).max(1),
});

type Item = z.infer<typeof ItemSchema>;
```

---

## Anti-Patterns

- `any` type — use `unknown` + type guards instead
- Inline styles — use CSS modules or Tailwind
- useEffect for derived state — compute during render
- Index as key in lists — use stable unique IDs
