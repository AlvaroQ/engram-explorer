# Engram Explorer design system (synced from sources)

Synced on 2026-11-05 from the project's real token sources. Only values found in those files are recorded.
Sources read:

- `internal/ui/static/tokens.css` (single source: HSL color tokens, spacing, radii, elevation, state layers)
- `apps/frontend/tailwind.config.ts` (color mapping, radii, font stacks)
- `apps/frontend/src/styles.css` (Tailwind input: base layer, `.card`, brain keyframes; no tokens)
- `internal/ui/static/styles.css` (shell components)
- `internal/doctor/static/styles.css` (doctor module; consumes the shared tokens)

`tokens.css` is linked by the shared layout before `app.css` and `styles.css`. No other file may redefine a color token (`internal/ui/static_test.go` enforces this, and that the doctor stylesheet has no hex colors).

## Colors

Tokens are bare HSL triplets (`H S% L%`), consumed as `hsl(var(--token))` or `hsl(var(--token) / alpha)`. Dark is the default; `:root.light` switches the palette.

| Token | Dark | Light | Usage |
| --- | --- | --- | --- |
| `--bg` | `249 22% 12%` | `32 57% 95%` | Page background |
| `--surface` | `247 23% 15%` | `35 100% 98%` | Cards, sidebar |
| `--surface-2` | `248 25% 18%` | `28 40% 92%` | Inputs, pills, hover rows, menus, neutral badges |
| `--border` | `249 15% 28%` | `10 9% 86%` | Borders, dividers |
| `--fg` | `245 50% 91%` | `248 24% 26%` | Primary text |
| `--fg-muted` | `248 15% 61%` | `248 12% 42%` | Secondary text, labels, idle status |
| `--accent` | `343 80% 72%` | `343 45% 42%` | Links, active nav, primary button, focus ring |
| `--accent-fg` | `249 22% 12%` | `0 0% 100%` | Text and icons on a filled `--accent` background |
| `--ok` | `189 43% 73%` | `189 45% 30%` | Success badges, healthy status |
| `--warn` | `35 88% 72%` | `33 90% 28%` | Warnings, demo banner, issue bar |
| `--fail` | `4 78% 72%` | `355 60% 42%` | Errors, broken sync, destructive actions |
| `--info` | `267 57% 78%` | `267 40% 45%` | Informational badges (doctor severity low) |
| `--brand-engram` | `343 80% 72%` | `343 45% 42%` | Engram group dot (same as `--accent`) |
| `--brand-cc` | `18 72% 62%` | `16 65% 40%` | Claude Code dot, bar and caption text |
| `--brand-codex` | `212 70% 72%` | `212 60% 38%` | Codex dot, bar and caption text |

`--brand-codex` replaces the neutral `--fg-muted` stand-in used for Codex in `design/overview-shell/02-brain-first.html`. It is a cool blue so it never reads as Claude Code orange, Engram rose, `--ok` teal or `--warn` gold. Every brand color passes AA as text, so a single token serves dots, bars and labels; no `-ink` variant is needed.

Doctor status mapping (`internal/doctor/static/styles.css`): success = `--ok`, warning = `--warn`, danger = `--fail`, info = `--info`, idle = `--fg-muted` on `--surface-2`. Tints use `hsl(var(--token) / 0.14-0.18)`.

Rule: the brand and status colors are reserved. Project and chart series colors are not tokenized today (hard-coded in `cc-usage-charts.tsx`, `type-breakdown.tsx`, `node-colors.ts`); a design may propose a series palette but must label it as proposed.

### Contrast (WCAG AA)

Every pair below is checked per theme by `internal/ui/tokens_contrast_test.go`: normal text needs 4.5:1, large text and non-text UI (dots, focus rings) need 3:1. Translucent backgrounds are composited over their base before measuring; the 0.18 tint is the strongest one the UI paints, so it bounds the 0.14 badges too. Add a row there before using a new pairing.

| Text | Background | Min | Dark | Light | Usage |
| --- | --- | --- | --- | --- | --- |
| `--fg` | `--bg` | 4.5:1 | 13.10 | 10.90 | body text |
| `--fg` | `--surface` | 4.5:1 | 12.27 | 11.65 | card and sidebar text |
| `--fg` | `--surface-2` | 4.5:1 | 11.44 | 10.13 | inputs, menus, hover rows |
| `--fg-muted` | `--bg` | 4.5:1 | 5.47 | 5.60 | page subtitles |
| `--fg-muted` | `--surface` | 4.5:1 | 5.13 | 5.99 | labels, table heads, captions |
| `--fg-muted` | `--surface-2` | 4.5:1 | 4.78 | 5.21 | secondary button, neutral badge |
| `--accent` | `--bg` | 4.5:1 | 6.86 | 6.03 | links in main content |
| `--accent` | `--surface` | 4.5:1 | 6.43 | 6.44 | links in cards, active settings gear |
| `--accent` | `--surface-2` | 4.5:1 | 6.00 | 5.60 | links on hover rows |
| `--accent` | `--accent` at 0.18 over `--surface` | 4.5:1 | 4.73 | 4.89 | active nav link, accent badge |
| `--ok` | `--bg` | 4.5:1 | 10.27 | 6.00 | inline success text |
| `--ok` | `--surface` | 4.5:1 | 9.62 | 6.41 | status pill (all ok) |
| `--ok` | `--ok` at 0.18 over `--surface` | 4.5:1 | 6.35 | 4.92 | success badge |
| `--warn` | `--bg` | 4.5:1 | 10.77 | 6.08 | inline warning text |
| `--warn` | `--surface` | 4.5:1 | 10.09 | 6.50 | status pill (daemon offline) |
| `--warn` | `--warn` at 0.18 over `--surface` | 4.5:1 | 6.61 | 4.96 | warning badge |
| `--fail` | `--bg` | 4.5:1 | 7.06 | 6.07 | inline error text |
| `--fail` | `--surface` | 4.5:1 | 6.62 | 6.49 | status pill (db error), form errors |
| `--fail` | `--fail` at 0.18 over `--surface` | 4.5:1 | 4.84 | 4.84 | error badge, doctor banner |
| `--info` | `--info` at 0.18 over `--surface` | 4.5:1 | 5.42 | 5.12 | doctor severity-low badge |
| `--fg` | `--warn` at 0.15 over `--bg` | 4.5:1 | 9.37 | 8.77 | overview sync alert |
| `--fg` | `--surface` at 0.72 over `--bg` | 4.5:1 | 12.51 | 11.44 | KPI values, search text on glass |
| `--fg-muted` | `--surface` at 0.72 over `--bg` | 4.5:1 | 5.23 | 5.88 | KPI labels and captions on glass |
| `--fg` | `--accent` at 0.22 over `--bg` | 4.5:1 | 8.95 | 7.82 | tonal CTA (hover layer) |
| `--accent-fg` | `--accent` | 4.5:1 | 6.86 | 6.65 | primary button, active toggle button |
| `--brand-cc` | `--surface` | 4.5:1 | 5.96 | 5.65 | Claude Code caption text |
| `--brand-codex` | `--surface` | 4.5:1 | 7.60 | 6.51 | Codex caption text |
| `--brand-engram` | `--surface` | 3.0:1 | 6.43 | 6.44 | Engram group dot |
| `--brand-cc` | `--surface` | 3.0:1 | 5.96 | 5.65 | Claude Code group dot |
| `--brand-codex` | `--surface` | 3.0:1 | 7.60 | 6.51 | Codex group dot |
| `--accent` | `--bg` | 3.0:1 | 6.86 | 6.03 | focus ring |

Not covered by tokens: decorative borders (`--border`) are not held to 3:1, and text drawn with extra alpha (`hsl(var(--fg-muted) / 0.7)` and similar) is not allowed for readable copy.

## Typography

| Role | Value | Source |
| --- | --- | --- |
| Sans stack | `ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif` | `tailwind.config.ts:33` |
| Mono stack | `ui-monospace, SFMono-Regular, Menlo, Monaco, monospace` | `tailwind.config.ts:34` |
| Body | 14px / 1.5, antialiased | `internal/ui/static/styles.css` (`body`) |
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

4px scale, `internal/ui/static/tokens.css`: `--space-1: 4px`, `--space-2: 8px`, `--space-3: 12px`, `--space-4: 16px`, `--space-5: 20px`, `--space-6: 24px`, `--space-8: 32px`.

Layout constants: sidebar width 236px; main content padding `28px 32px`; card header/body padding `16px 20px`; table cell padding `9px 14px`; KPI tile body `14px 16px`; page header margin-bottom 24px.

## Radius

| Token | Value | Source |
| --- | --- | --- |
| `--radius-sm` / Tailwind `sm` | 6px (0.375rem) | `tokens.css`, `tailwind.config.ts:29` |
| `--radius-md` / Tailwind `md` | 8px (0.5rem) | `tokens.css`, `tailwind.config.ts:28` |
| `--radius-lg` / Tailwind `lg` | 12px (0.75rem) | `tokens.css`, `tailwind.config.ts:27` |
| Pills | 999px | `.status-pill`, `.count-pill` |
| Badge | 4px | `.badge` |

## Shadows and state layers

`internal/ui/static/tokens.css`:

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
- **Overview hero** (02-brain-first + 04-scroll-collapse): 500px edge-to-edge band with the knowledge-graph preview (non-interactive; the tonal CTA opens `/brain`), title, and a `GET /observations?q=` search. On wide screens it is sticky at `top: calc(72px - 500px)` and collapses over scroll 37 → 465px (with the sync alert above it) into a 72px bar: the graph scales to 0.55 and fades to 0.35 over an opaque `--surface`, the title goes 20px → 16px and the search docks beside it. Reduced motion: the hero scrolls away.
- **Glass**: `hsl(var(--surface) / 0.72)` with `backdrop-filter: blur(14px) saturate(1.15)` (0.95 without backdrop-filter support); used by the hero search and the KPI cards. Text on it is checked against `--surface` at 0.72 over `--bg`.
- **Overview KPI cards**: four glass cards (Memories saved, Projects, AI conversations, Agent spend this month) overlapping the hero by 64px. Each also has a strip version (label + value, a link to the card's target) shown once the cards pin as a 44px strip under the bar; the hidden version is `inert` + `aria-hidden`. Agent splits are SVG rects in `--brand-cc` / `--brand-codex`.
- **Tonal button** (`.btn-tonal`): `--fg` text on `--accent` at 0.14 (0.22 on hover) with an `--accent` 0.45 border; never white on accent.
- **Buttons**: `.btn` 6px/14px, radius 6px, 13px/500, `--surface-2` with `--border`; `.btn-primary` accent background with `--accent-fg` text; `.btn-secondary` muted text; `.btn-sm` 4px/10px 12px. Focus ring: 2px accent outline, offset 2px.
- **Badges**: 2px/7px, radius 4px, 11px/500; tinted variants use `color / 0.14` background and `color / 0.35` border for `ok`, `warn`, `fail`, `accent`; `neutral` uses `--surface-2` and `--border`.
- **Inputs**: `--surface-2`, `--border`, radius 6px, 13px, padding 7/11, accent border on focus.
- **Tables**: 13px, uppercase 11px head, 1px row dividers, `--surface-2` row hover, `.mono` cells 12px monospace.
- **Status pill**: 999px, 11px/600, dot 8px.
- **Banners**: demo banner uses `--warn / 0.15` background and `--warn / 0.3` bottom border.
- **Empty state**: 40px padding, centered, 13px muted.
- **Motion**: ease-out-expo `cubic-bezier(0.16, 1, 0.3, 1)` 0.45s for entrances; 5s idle float; transitions 0.12-0.15s ease. Every motion must respect `prefers-reduced-motion`.

## Self-check for artboards

List every color and font family the artboard uses. Each color must be one of the tokens above (or a state layer derived from one). Any proposed series palette for charts or graph nodes must be declared as "proposed" in the artboard and in `design.json`.
