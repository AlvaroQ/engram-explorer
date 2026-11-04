# Engram Explorer design system (synced from sources)

Synced on 2026-11-04 from the project's real token sources. Only values found in those files are recorded.
Sources read:

- `apps/frontend/src/styles.css` (canonical HSL tokens, `.card`, brain keyframes)
- `apps/frontend/tailwind.config.ts` (color mapping, radii, font stacks)
- `internal/ui/static/styles.css` (mirrored tokens, spacing, radii, elevation, state layers, shell components)
- `internal/doctor/static/styles.css` (doctor module status palette; `pico.min.css` is shipped but no longer loaded)

Known drift: the token triplets are duplicated by hand in `apps/frontend/src/styles.css` and `internal/ui/static/styles.css`. Treat `apps/frontend/src/styles.css` as canonical when they differ.

## Colors

Tokens are bare HSL triplets (`H S% L%`), consumed as `hsl(var(--token))` or `hsl(var(--token) / alpha)`. Dark is the default; `:root.light` switches the palette.

| Token | Dark | Light | Usage | Source |
| --- | --- | --- | --- | --- |
| `--bg` | `249 22% 12%` | `32 57% 95%` | Page background | `apps/frontend/src/styles.css:7,21` |
| `--surface` | `247 23% 15%` | `35 100% 98%` | Cards, sidebar | `apps/frontend/src/styles.css:8,22` |
| `--surface-2` | `248 25% 18%` | `28 40% 92%` | Inputs, pills, hover rows, menus | `apps/frontend/src/styles.css:9,23` |
| `--border` | `249 15% 28%` | `10 9% 86%` | Borders, dividers | `apps/frontend/src/styles.css:10,24` |
| `--fg` | `245 50% 91%` | `248 19% 40%` | Primary text | `apps/frontend/src/styles.css:11,25` |
| `--fg-muted` | `248 15% 61%` | `248 12% 52%` | Secondary text, labels | `apps/frontend/src/styles.css:12,26` |
| `--accent` | `343 76% 68%` | `343 35% 55%` | Links, active nav, primary button, focus ring | `apps/frontend/src/styles.css:13,27` |
| `--ok` | `189 43% 73%` | `189 30% 48%` | Success badges, healthy status | `apps/frontend/src/styles.css:14,28` |
| `--warn` | `35 88% 72%` | `35 81% 56%` | Warnings, demo banner, issue bar | `apps/frontend/src/styles.css:15,29` |
| `--fail` | `343 35% 55%` | `343 35% 55%` | Errors, broken sync | `apps/frontend/src/styles.css:16,30` |
| `--brand-engram` | `343 76% 68%` | `343 35% 55%` | Engram group dot | `internal/ui/static/styles.css:20,34` |
| `--brand-cc` | `18 72% 62%` | `16 60% 50%` | Claude Code group dot | `internal/ui/static/styles.css:21,35` |

Doctor status palette (hex, dark only; Rosé Pine derived), `internal/doctor/static/styles.css:11-15`:

| Token | Value | Usage |
| --- | --- | --- |
| `--doc-green` | `#9ccfd8` | success |
| `--doc-amber` | `#f6c177` | warning |
| `--doc-red` | `#eb6f92` | danger |
| `--doc-blue` | `#c4a7e7` | info |
| `--doc-gray` | `#6e6a86` | idle |

Primary button text is the only non-token color in the shell: `hsl(220, 25%, 98%)` (`internal/ui/static/styles.css`, `.btn-primary`).

Rule: the brand and status colors are reserved. Project and chart series colors are not tokenized today (hard-coded in `cc-usage-charts.tsx`, `type-breakdown.tsx`, `node-colors.ts`); a design may propose a series palette but must label it as proposed.

## Typography

| Role | Value | Source |
| --- | --- | --- |
| Sans stack | `ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif` | `tailwind.config.ts:33` |
| Mono stack | `ui-monospace, SFMono-Regular, Menlo, Monaco, monospace` | `tailwind.config.ts:34` |
| Body | 14px / 1.5, antialiased | `internal/ui/static/styles.css:95-98` |
| Page title (h1) | 20px / 600 | `.page-header h1` |
| Page subtitle | 13px, `--fg-muted` | `.page-header p` |
| Card title (h2) | 14px / 600 | `.card-header h2` |
| Card subtitle | 12px, `--fg-muted` | `.card-header p` |
| KPI value | 24px / 600 / 1.1 | `.kpi-value` |
| KPI label | 11px uppercase, 0.04em, `--fg-muted` | `.kpi-label` |
| Section label (sidebar group, table head) | 10-11px / 600-700 uppercase, 0.06-0.09em, `--fg-muted` | `.sidebar-group-label`, `.data-table thead th` |
| Brand title | 16px / 700, -0.01em | `.sidebar-brand-title` |
| Nav link | 13px / 500 (600 when active) | `.sidebar-nav a` |
| Badge, pill | 11px / 500-600 | `.badge`, `.count-pill`, `.status-pill` |

No web fonts are declared anywhere. System font stacks only.

## Spacing

4px scale, `internal/ui/static/styles.css:46-53`: `--space-1: 4px`, `--space-2: 8px`, `--space-3: 12px`, `--space-4: 16px`, `--space-5: 20px`, `--space-6: 24px`, `--space-8: 32px`.

Layout constants: sidebar width 236px; main content padding `28px 32px`; card header/body padding `16px 20px`; table cell padding `9px 14px`; KPI tile body `14px 16px`; page header margin-bottom 24px.

## Radius

| Token | Value | Source |
| --- | --- | --- |
| `--radius-sm` / Tailwind `sm` | 6px (0.375rem) | `styles.css:56`, `tailwind.config.ts:29` |
| `--radius-md` / Tailwind `md` | 8px (0.5rem) | `styles.css:57`, `tailwind.config.ts:28` |
| `--radius-lg` / Tailwind `lg` | 12px (0.75rem) | `styles.css:58`, `tailwind.config.ts:27` |
| Pills | 999px | `.status-pill`, `.count-pill` |
| Badge | 4px | `.badge` |

## Shadows and state layers

`internal/ui/static/styles.css:61-82`:

| Token | Dark | Light |
| --- | --- | --- |
| `--elevation-1` | `0 1px 2px hsl(222 47% 4% / .35), 0 1px 1px hsl(222 47% 4% / .25)` | `0 1px 2px hsl(222 47% 30% / .08), 0 1px 1px hsl(222 47% 30% / .06)` |
| `--elevation-2` | `0 2px 6px hsl(222 47% 4% / .40), 0 1px 2px hsl(222 47% 4% / .30)` | `0 2px 6px hsl(222 47% 30% / .10), 0 1px 2px hsl(222 47% 30% / .08)` |
| `--elevation-3` | `0 6px 16px hsl(222 47% 4% / .45), 0 2px 6px hsl(222 47% 4% / .35)` | `0 8px 24px hsl(222 47% 30% / .12), 0 2px 8px hsl(222 47% 30% / .08)` |
| `--state-hover` | `0.08` | `0.06` |
| `--state-active` | `0.14` | `0.10` |

Hover and active states are built as `hsl(var(--fg) / var(--state-hover))` and `hsl(var(--accent) / 0.14)` state layers, never new colors.

## Component rules

- **App shell**: `display:flex; height:100vh`, fixed 236px sidebar on `--surface` with a right border, scrollable `.main-content` on `--bg`. Mobile breakpoint 639px: sidebar becomes a drawer with scrim.
- **Sidebar**: brand block (title + uppercase label + settings gear 32px), grouped nav with group labels (dot or 18px brand icon), links 8px/12px padding, radius-sm, active = accent 14% layer + 3px accent bar on the left. Footer with status pill.
- **Card**: `--surface`, 1px `--border`, radius-md, elevation-1, header with bottom border, body padding 16/20. React `.card` variant uses radius-lg and padding 20px.
- **KPI tile**: card with label (11px uppercase) over value (24px/600) and an optional caption link.
- **Buttons**: `.btn` 6px/14px, radius 6px, 13px/500, `--surface-2` with `--border`; `.btn-primary` accent background; `.btn-secondary` muted text; `.btn-sm` 4px/10px 12px. Focus ring: 2px accent outline, offset 2px.
- **Badges**: 2px/7px, radius 4px, 11px/500; tinted variants use `color / 0.14` background and `color / 0.35` border for `ok`, `warn`, `fail`, `accent`; `neutral` uses `--surface-2` and `--border`.
- **Inputs**: `--surface-2`, `--border`, radius 6px, 13px, padding 7/11, accent border on focus.
- **Tables**: 13px, uppercase 11px head, 1px row dividers, `--surface-2` row hover, `.mono` cells 12px monospace.
- **Status pill**: 999px, 11px/600, dot 8px.
- **Banners**: demo banner uses `--warn / 0.15` background and `--warn / 0.3` bottom border.
- **Empty state**: 40px padding, centered, 13px muted.
- **Motion**: ease-out-expo `cubic-bezier(0.16, 1, 0.3, 1)` 0.45s for entrances; 5s idle float; transitions 0.12-0.15s ease. Every motion must respect `prefers-reduced-motion`.

## Self-check for artboards

List every color and font family the artboard uses. Each color must be one of the tokens above (or a state layer derived from one). Any proposed series palette for charts or graph nodes must be declared as "proposed" in the artboard and in `design.json`.
