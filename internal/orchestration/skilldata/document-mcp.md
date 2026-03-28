---
name: document-mcp
description: "Create consulting-quality PPTX presentations and DOCX documents with charts, stats, flows, tables, and AI-generated images."
triggers:
  - "presentation"
  - "slide"
  - "pptx"
  - "powerpoint"
  - "document"
  - "docx"
  - "report"
role: coder
phase: implement
mcp_tools:
  - "document-mcp.create_presentation_tool"
  - "document-mcp.create_document_tool"
  - "document-mcp.generate_image_tool"
---

> **Spec Override:** These patterns are defaults. If a project spec defines different
> architecture, package structure, or scope, follow the spec instead.
# Skill: Document MCP

Create professionally formatted PPTX presentations and DOCX documents using the document-mcp server. Presentations use the Red Hat template with consulting-quality visual elements — not walls of bullet points.

---

## When to Use

- When the user asks to create a presentation, slide deck, or PPTX
- When the user asks to create a document, report, or DOCX
- When the user wants to visualize data as charts, stats, or process flows in slides
- When converting markdown or outline content into formatted documents
- When the user wants to upload documents to Google Drive or share them
- When the user asks to generate images, diagrams, or illustrations for slides
- When the user wants a presentation with visuals beyond text and charts

---

## MCP Tools

The `document-mcp` server provides five tools:

1. **`create_presentation_tool`** — Creates a PPTX from structured slide data. Params: `title`, `slides`, `output_path`, `theme`, `upload` (bool), `share_with` (email)
2. **`create_document_tool`** — Creates a DOCX from structured content blocks. Params: `title`, `content`, `output_path`, `style`, `upload` (bool), `share_with` (email)
3. **`generate_image_tool`** — Generates an AI image via Vertex AI Imagen. Params: `prompt`, `output_path`, `aspect_ratio` ("16:9", "1:1", "4:3", "3:4", "9:16")
4. **`upload_to_google_tool`** — Uploads an existing PPTX/DOCX to Google Drive (Shared Drive). Params: `file_path`, `share_with` (email), `folder_id`
5. **`list_layouts`** — Returns all available layouts, element types, and examples

---

## Google Drive Workflow

Both `create_presentation_tool` and `create_document_tool` support direct upload:
- Set `upload=True` to auto-upload to Google Drive after creation
- Set `share_with="user@example.com"` to grant writer access
- Files are converted to Google Slides/Docs automatically
- Result includes a `google.url` field with the shareable link

For existing files, use `upload_to_google_tool` directly.

---

## AI Image Generation

Use `generate_image_tool` to create visuals for slides — diagrams, illustrations, backgrounds, icons.

### Standalone workflow (recommended for multiple images)
Call `generate_image_tool` first (can generate multiple images in parallel), then reference the paths in slide `image` elements. Save images to `/tmp/` with descriptive names.

1. Call `generate_image_tool` with `prompt`, `output_path` (e.g. `/tmp/slide3_arch.png`), `aspect_ratio`
2. Use the returned `path` in the slide's `image` element

Example image element referencing a pre-generated image:
```json
{"type": "image", "path": "/tmp/slide3_arch.png",
 "position": {"left": 1.0, "top": 2.0, "width": 11.0, "height": 4.5}}
```

### Inline workflow (simpler for single images)
The `image` element supports `ai_prompt` for auto-generation during slide rendering — no separate tool call needed:
```json
{"type": "image",
 "ai_prompt": "professional diagram of microservices architecture, minimal style",
 "aspect_ratio": "16:9",
 "position": {"left": 1.0, "top": 2.0, "width": 11.0, "height": 4.5}}
```
If both `path` and `ai_prompt` are set, the local file takes priority.

### Imagen prompt tips
- Be specific: "clean flat illustration of X" not just "X"
- Include style: "corporate", "professional", "minimal", "modern"
- Specify background: "white background", "transparent"
- Good for: architecture diagrams, conceptual illustrations, abstract visuals, icons
- Avoid: text-heavy images, screenshots, photos of specific people
- Use `16:9` for full-slide images, `1:1` for thumbnails/icons

---

## Presentation Workflow

### Step 1: Plan the slides
Analyze the content and decide which layout and element types best represent each piece of information. Avoid defaulting to bullet-point text slides.

### Step 2: Choose element types strategically

| Content Type | Best Element |
|---|---|
| Key metrics / KPIs | `stat_callout` or `stat_row` |
| Data comparisons | `chart` (column, bar, line, pie) |
| Tabular data | `table` |
| Sequential process | `process_flow` |
| Feature lists / capabilities | `icon_grid` |
| Before/after, pros/cons | `comparison` |
| Project milestones | `timeline` |
| Key quotes / takeaways | `callout_box` |
| Visuals / diagrams | `image` (with `ai_prompt` or local path) |
| Explanatory text | `text` (keep minimal) |

### Step 3: Generate any needed images
If slides need visuals (diagrams, illustrations), call `generate_image_tool` for each image **before** building the slide array. Save to `/tmp/` with descriptive names. Multiple images can be generated in parallel.

### Step 4: Build the slide array
Call `create_presentation_tool` with structured slides. Reference generated image paths in `image` elements. Always set `output_path` to a location the user can access (e.g., `~/Documents/` or `~/Desktop/`). Set `upload=True` to auto-upload to Google Drive.

---

## Presentation Layouts

| Layout | Use For |
|---|---|
| `title` | Opening slide (title + subtitle + presenter) |
| `closing` | Thank you / closing slide |
| `section` | Section divider (red background, large text) |
| `divider` | Section divider with supporting text |
| `content` | Main content slide (title + elements) |
| `content_simple` | Content without title placeholder |
| `two_column` | Side-by-side content |
| `image_content` | Content with image area |

---

## Element Types with Examples

### stat_row — KPI dashboard
```json
{"type": "stat_row", "stats": [
  {"value": "99.9%", "label": "Uptime"},
  {"value": "< 2s", "label": "Response Time"},
  {"value": "47%", "label": "Cost Reduction"}
], "position": {"left": 1.0, "top": 2.5, "width": 11.0, "height": 2.0}}
```

### chart — Data visualization
```json
{"type": "chart", "chart_type": "column_clustered",
 "categories": ["Q1", "Q2", "Q3", "Q4"],
 "series": [{"name": "Revenue", "values": [120, 145, 160, 190]}],
 "position": {"left": 1.0, "top": 2.5, "width": 8.0, "height": 3.7}}
```
Chart types: `bar_clustered`, `bar_stacked`, `column_clustered`, `column_stacked`, `line`, `line_markers`, `pie`, `doughnut`, `area`, `area_stacked`

### process_flow — Sequential steps
```json
{"type": "process_flow", "style": "chevron",
 "steps": [
   {"title": "Discovery", "description": "Assess current state"},
   {"title": "Design", "description": "Architecture & planning"},
   {"title": "Build", "description": "Implementation"},
   {"title": "Deploy", "description": "Go live"}
 ], "position": {"left": 1.0, "top": 3.0, "width": 11.0, "height": 2.0}}
```
Styles: `chevron`, `circles`, `arrows`, `numbered`

### comparison — Before/after
```json
{"type": "comparison",
 "left": {"heading": "Before", "items": ["Manual process", "4-hour turnaround"]},
 "right": {"heading": "After", "items": ["Automated", "Real-time"]},
 "position": {"left": 1.0, "top": 2.5, "width": 11.0, "height": 3.7}}
```

### icon_grid — Feature grid
```json
{"type": "icon_grid", "columns": 3,
 "items": [
   {"icon": "S", "title": "Security", "description": "End-to-end encryption"},
   {"icon": "P", "title": "Performance", "description": "Sub-second response"}
 ], "position": {"left": 1.0, "top": 2.5, "width": 11.0, "height": 3.7}}
```

### table — Styled data table
```json
{"type": "table", "headers": ["Metric", "Current", "Target"],
 "rows": [["Response Time", "4.2s", "< 2s"], ["Uptime", "99.1%", "99.9%"]],
 "position": {"left": 1.0, "top": 2.5, "width": 11.0, "height": 3.5}}
```

### timeline — Project milestones
```json
{"type": "timeline", "milestones": [
   {"date": "Q1 2026", "title": "Phase 1", "description": "Pilot launch"},
   {"date": "Q2 2026", "title": "Phase 2", "description": "Full rollout"}
 ], "position": {"left": 1.0, "top": 3.0, "width": 11.0, "height": 2.5}}
```

### callout_box — Key takeaway
```json
{"type": "callout_box", "text": "Key insight: automated triage reduced resolution time by 47%",
 "style": "accent", "position": {"left": 1.0, "top": 5.0, "width": 5.0, "height": 1.0}}
```
Styles: `accent` (left red border), `filled` (red background), `quote` (italic)

### image — Local or AI-generated
```json
{"type": "image", "path": "/path/to/image.png",
 "position": {"left": 7.0, "top": 2.0, "width": 5.0, "height": 3.7}}
```
Or with AI generation:
```json
{"type": "image",
 "ai_prompt": "professional cloud architecture diagram, flat style, white background",
 "aspect_ratio": "16:9",
 "position": {"left": 1.0, "top": 2.0, "width": 11.0, "height": 4.5}}
```

---

## Positioning & Safe Zone

Slide dimensions: 13.33" x 7.50". All positions use inches: `{"left", "top", "width", "height"}`.

**Safe zone** — the Red Hat template has a footer bar (version text + Red Hat logo) starting at ~6.85". Content is automatically clamped to stay within:
- **Max bottom**: 6.30" (`SAFE_BOTTOM`) — content cannot extend below this
- **Max right**: 12.36" (`SAFE_RIGHT`) — content cannot extend past this
- **Default content area**: left=0.97, top=2.50, width=11.39, height=3.70 (bottom at 6.20")

When stacking multiple elements vertically on one slide, keep the bottom element's `top + height` under 6.30". The system clamps automatically, but planning positions avoids content being squished.

---

## Document Workflow

Call `create_document_tool` with content blocks:

```json
{"type": "heading1", "text": "Section Title"}
{"type": "paragraph", "text": "Body text with **bold** and *italic*."}
{"type": "bullet_list", "items": ["Point A", "Point B"]}
{"type": "numbered_list", "items": ["Step 1", "Step 2"]}
{"type": "table", "table": {"headers": ["Col1", "Col2"], "rows": [["a", "b"]]}}
{"type": "blockquote", "text": "An important quote."}
{"type": "page_break"}
```

Block types: `heading1`, `heading2`, `heading3`, `paragraph`, `bullet_list`, `numbered_list`, `table`, `page_break`, `blockquote`

---

## Key Design Principles

1. **Visual variety** — Never create all-text slides. Mix charts, stats, flows, and tables
2. **One idea per slide** — Each slide should have a clear message
3. **Use stat_row for metrics** — Big numbers are more impactful than bullet points
4. **Use process_flow for sequences** — Shows progression visually
5. **Use comparison for contrasts** — Side-by-side is clearer than listing differences
6. **Position elements thoughtfully** — Avoid overlap, use the full content area
7. **Combine elements** — A slide can have a stat_row at top + chart below
8. **Use AI images for visuals** — Generate diagrams and illustrations instead of leaving slides text-only

---

## Tools

- document-mcp MCP — `create_presentation_tool`, `create_document_tool`, `generate_image_tool`, `upload_to_google_tool`, `list_layouts`
