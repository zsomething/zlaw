# Hub Web UI: htmx + DaisyUI Integration Plan

## Overview

Migrate the existing Hub web UI from custom CSS + vanilla JS to **htmx + DaisyUI** while keeping **pongo2** for server-side templating.

## Why This Approach

| Technology | Role |
|------------|------|
| **pongo2** | Server-side Go templates (already in use) |
| **htmx** | Frontend interactivity (requests, swaps, SSE) |
| **Tailwind CSS** | Utility CSS framework (DaisyUI dependency) |
| **DaisyUI** | Pre-built UI components (cards, badges, tables, tabs, etc.) |

This keeps Go on the server, minimizes client-side JS, and uses battle-tested CDNs for fast load times.

---

## Current State

- **Templating**: pongo2 with extraction to temp dir
- **Styling**: ~200 lines of custom CSS per page
- **Interactivity**: Vanilla JS `fetch()` + DOM manipulation
- **Pages**: index, agents, agent_detail, tools, audit

```
internal/hub/web/
├── templates/
│   └── pages/
│       ├── base.html
│       ├── index.html
│       ├── agents.html
│       ├── agent_detail.html
│       ├── tools.html
│       └── audit.html
├── server.go
├── templates.go
└── state.go
```

---

## Target Directory Structure

```
internal/hub/web/
├── templates/
│   ├── pages/          # Full page templates
│   │   ├── base.html
│   │   ├── index.html
│   │   ├── agents.html
│   │   ├── agent_detail.html
│   │   ├── tools.html
│   │   └── audit.html
│   └── partials/      # HTMX swap targets
│       ├── agent_cards.html
│       ├── agent_row.html
│       ├── audit_rows.html
│       ├── tool_list.html
│       └── stats.html
├── server.go          # Updated routes + hx-request handling
├── templates.go       # Keep pongo2, add partials support
└── state.go
```

---

## Implementation Steps

### Phase 1: Base Layout

**File**: `templates/pages/base.html`

Changes:
- Add Tailwind CDN (`cdn.tailwindcss.com`)
- Add DaisyUI CDN
- Add htmx CDN + SSE extension
- Replace custom CSS nav/header with DaisyUI `navbar` component
- Replace custom footer with DaisyUI `footer`
- Add `hx-boost="true"` on body for SPA-like navigation
- Add global htmx loading indicator

### Phase 2: Partial Templates

Create reusable fragments in `templates/partials/`:

| Partial | Purpose |
|---------|---------|
| `agent_cards.html` | Agent grid for overview/agents page |
| `agent_row.html` | Single agent row for detail page tabs |
| `audit_rows.html` | Table rows for audit log pagination |
| `tool_list.html` | Tool list with descriptions |
| `stats.html` | Hub stats cards (NATS addr, agent count, etc.) |

### Phase 3: Server Updates

**File**: `server.go`

1. Add `request_path` to template context for nav active states
2. Add new route: `GET /partials/agents` → returns `agent_cards.html`
3. Detect `HX-Request` header → return partial instead of full page
4. SSE endpoint for real-time audit log: `GET /events/audit`

### Phase 4: Page Migrations

Convert each page from custom CSS + vanilla JS to DaisyUI + htmx:

#### index.html
- DaisyUI `card` components for hub stats
- Include `stats.html` partial
- Include `agent_cards.html` partial
- Remove custom CSS

#### agents.html
- DaisyUI `card` for container
- Include `agent_cards.html` partial
- htmx polling for auto-refresh (10s)
- Remove vanilla JS fetch/render

#### agent_detail.html
- DaisyUI `tabs` component for section navigation
- DaisyUI `table` for sessions
- DaisyUI `menu` for file browser
- htmx tab content loading

#### audit.html
- DaisyUI `table` + `select` for filters
- htmx form submission for filter changes
- SSE for real-time log streaming (optional, Phase 2)
- Pagination with htmx

#### tools.html
- DaisyUI `table` for tool list
- DaisyUI `collapse` for tool descriptions
- Static page, minimal htmx needed

### Phase 5: Theme Support (Optional)

Add DaisyUI theme switcher:
- Persist theme choice in localStorage
- Use `data-theme` attribute on `<html>`

---

## HTMX Patterns Used

### Navigation
```html
<body hx-boost="true">  <!-- SPA-like page transitions -->
```

### Auto-refresh
```html
<div hx-get="/partials/agents" hx-trigger="every 10s" hx-swap="innerHTML">
```

### Form Filters
```html
<select name="type" hx-get="/partials/audit" hx-trigger="change" hx-target="#audit-body">
```

### Partial Returns
```go
if r.Header.Get("HX-Request") == "true" {
    s.servePartialTemplate(w, "agent_cards.html", data)
    return
}
s.serveTemplate(w, "agents.html", data)
```

### SSE Streaming
```html
<div hx-sse="connect:/events/audit" hx-swap="innerHTML">
```

---

## DaisyUI Components Used

| Component | Usage |
|-----------|-------|
| `card` | Agent cards, stat blocks |
| `badge` | Status indicators, capabilities |
| `table` | Audit log, tools, sessions |
| `tabs` | Agent detail sections |
| `menu` | File browser, navigation |
| `navbar` | Top navigation |
| `footer` | Page footer |
| `select` | Audit filters |
| `loading` | Loading indicators |
| `collapse` | Expandable tool descriptions |
| `stats` | Hub overview stats |
| `tooltip` | Hover info |

---

## Rollout Strategy

1. **Phase 1 ✅** Base layout + partials + htmx routing
2. **Phase 2 ✅** SSE for real-time audit/agent updates
3. **Phase 3 ✅** HX-Request detection for partial page swaps
4. **Phase 4 ✅** Error states, empty states, animations, theme switcher

---

## Files to Create/Modify

### Create ✅
- [x] `templates/partials/agent_cards.html`
- [x] `templates/partials/agent_row.html`
- [x] `templates/partials/audit_rows.html`
- [x] `templates/partials/tool_list.html`
- [x] `templates/partials/session_list.html`
- [x] `templates/partials/stats.html`

### Server
- [x] `server.go` — SSE endpoints (`/events/audit`, `/events/agents`)
- [x] Templates updated to use htmx SSE extension (`hx-ext="sse"`, `sse-connect`, `sse-swap`)
- [x] SSE returns HTML partials for direct DOM swap (no client-side JSON parsing)

### Base Layout (Phase 1-4)
- [x] DaisyUI + Tailwind via CDN
- [x] htmx v2 + SSE extension via CDN
- [x] Theme switcher with localStorage persistence
- [x] CSS animations for page transitions (`fadeIn`, `settleIn`, `slideIn`)
- [x] SSE connection status indicators
- [x] Error states with retry buttons
- [x] Page loading states

---

## Notes

- **CDN vs bundled**: Using CDNs for Tailwind/DaisyUI/htmx for simplicity. For production, consider bundling if offline access is needed.
- **No build step**: Templates remain in Go `embed.FS`, extracted to temp dir at startup (existing pattern).
- **SSE optional**: Audit streaming via SSE is Phase 2. Polling is sufficient for v1.
- **pongo2 filters**: Keep using pongo2 filters (`urlencode`, `date`, `default`). Custom filters can be added if needed.
