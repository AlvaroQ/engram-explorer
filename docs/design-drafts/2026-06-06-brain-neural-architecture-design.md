a# Brain — Deep Neural Architecture Redesign

- **Date:** 2026-06-06
- **Status:** Reviewed (adversarial spec review passed: _Approved with fixes_ — fixes applied below; pending user approval)
- **Topic:** `/brain` (TAB BRAIN) — categorized/segmented information as a navigable neural structure
- **Branch:** `feature/brain`
- **Next step after approval:** SDD (`/sdd-new`), not writing-plans

> **Module status (read first):** The `interpolator`/motion layer, the `brain-model` abstraction, the navigation stack, the view modes, and the segmentation panel **do not exist yet** — this design is ahead of the codebase. Sections describing them use "will" deliberately. What exists today is the flat R3F renderer (§2). The interpolator is the **lead SDD task** because D4/D5/D7 depend on it.

---

## 1. Context & Goal

The `/brain` page renders all Engram observations as a single flat 3D force-directed knowledge graph (React Three Fiber + Three.js). It is visually rich but flat: ~490 observations (server-capped at 800), no hierarchical drill-down, and mostly instant (non-eased) transitions.

**Goal:** turn `/brain` into a _deep brain architecture_ where information is clearly **categorized and segmented**, animations are **fluid via a single interpolation layer**, and the user **drills progressively into "neurons" that store clusters of information** — a living neural-interconnection structure.

This is a frontend-centric redesign of an existing, working feature. It **reuses** the current R3F rendering stack and **adds** four new modules on top. No backend changes are required for v1.

---

## 2. Current State (grounded)

| Area          | File                                                              | Today                                                                                                                                                                                                      |
| ------------- | ----------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------- |
| Page shell    | `apps/frontend/src/pages/brain-page.tsx`                          | Summary cards + canvas + rails; disabled "conversation mode"                                                                                                                                               |
| Scene         | `apps/frontend/src/components/brain/graph-scene.tsx`              | `<Canvas frameloop="demand">`, OrbitControls, `CameraFocus` (direct linear `0.12` lerp inside `useFrame`, `graph-scene.tsx:94`, calls `invalidate()` each frame)                                           |
| Nodes         | `apps/frontend/src/components/brain/graph-nodes.tsx`              | One `InstancedMesh`, per-instance color, fresnel rim; static halos                                                                                                                                         |
| Edges         | `apps/frontend/src/components/brain/tunnel-flow.tsx`              | 4 layers (semantic + topic, base `lineSegments` + GPU particles), constant-speed flow; `deriveSimilarityEdges` (TF-IDF/k-NN, `tunnel-flow.tsx:131`) builds synthetic topic roads client-side via `useMemo` |
| Layout        | `apps/frontend/src/components/brain/force-layout.worker.ts`       | Off-main-thread 3D force layout, ~200 iters, normalized ±30                                                                                                                                                |
| Colors        | `apps/frontend/src/components/brain/node-colors.ts`               | HSL palette by project or type                                                                                                                                                                             |
| Color toggle  | `apps/frontend/src/components/brain/color-by-toggle.tsx`          | Either/or `project                                                                                                                                                                                         | type` (controls **color only**) |
| Project rails | `apps/frontend/src/components/brain/project-rail.tsx`             | 45+ projects split **L/R**, isolate-and-dim                                                                                                                                                                |
| Detail        | `apps/frontend/src/components/brain/node-detail-panel.tsx`        | Metadata + lazy markdown + wikilinks; **no neighbors**                                                                                                                                                     |
| Markdown      | `apps/frontend/src/components/brain/markdown-content.tsx`         | Sections + `[[wikilinks]]`                                                                                                                                                                                 |
| A11y          | `apps/frontend/src/components/brain/accessible-node-list.tsx`     | sr-only list, capped at 150                                                                                                                                                                                |
| API types     | `apps/frontend/src/lib/api.ts`                                    | `GraphNode`, `GraphEdge`, `GraphMeta`                                                                                                                                                                      |
| Backend       | `internal/services/graph.go` + `internal/httpapi/routes_graph.go` | Flat graph, dedupe by `normalized_hash`, node cap (`internal/services/graph.go`)                                                                                                                           |

**Data available per node** (`GraphNode`): `id, project, type, scope, topicKey, label, weight, duplicateCount, createdAt`. **Per edge** (`GraphEdge`): `source, target, relation ('related'|'scoped'|'compatible'), confidence?, reason?`.

**Categorization fields present but not surfaced for segmentation:** `scope`, `topicKey` (only a layout hint today, not a cluster), `tool_name`/`session_id` (in DB, **not in `GraphNode`** — verified), edge `confidence`/`reason` (never rendered).

---

## 3. Design Decisions (validated with the user)

| #   | Decision                    | Choice                                                                                                                                                            | Rationale                                                                                                                                      |
| --- | --------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| D1  | What is a "neuron"          | **All three as switchable Views**: Lóbulos (neuron=project), Temas (neuron=`topic_key`), Orgánico (neuron=similarity cluster)                                     | `topic_key` already organizes Engram memory; views cover simple→deep→organic                                                                   |
| D2  | View switcher placement     | Horizontal tabs **right of the title**; **replaces** the old color toggle as the primary control                                                                  | Clean, signals "configure the brain architecture"                                                                                              |
| D3  | Color control               | **Survives as a secondary, compact control**; per-view sensible default, still offers "by type"                                                                   | View ≠ color; orthogonal axes — keep both lenses                                                                                               |
| D4  | Drill-down mechanic         | **Immersive zoom** — camera flies in, neuron explodes into members, rest recedes; breadcrumb to exit                                                              | Only model that delivers "deep architecture"; each level renders only its own nodes → dissolves the 800 cap                                    |
| D5  | Motion language             | **Smooth / organic** (easeOutExpo), no spring; centralized                                                                                                        | User picked "Suave" — fluid, no bounce                                                                                                         |
| D6  | Where hierarchy is computed | **Hybrid (C)** — client aggregation behind a `brain-model` interface, swappable to backend later                                                                  | ~90% of work (UI) is identical either way; local-first app holds thousands of nodes fine; don't freeze a backend contract before validating UX |
| D7  | Synapses                    | **Meaningful + focus (B)** — subtle ambient, light up on hover/select; color=relation, radius=count, brightness=confidence; semantic vs topic-road distinguished  | "Alive but legible"                                                                                                                            |
| D8  | Search/segmentation         | **Segmentation panel (B)** — search + **stackable** facets (Project AND Type AND Scope AND Tool AND Date) + legend + per-level stats; `⌘K` quick-jump v2-deferred | Fulfills "categorized and segmented"; stackable beats either/or                                                                                |
| D9  | Layout                      | **Unified panel** — left = segmentation, center = brain + breadcrumb + stats, right = detail + neighbors; side rails removed                                      | Stackable facets need their own panel; freeing the right side for detail beats a floating card                                                 |

**Defaults assumed (user-confirmed):** node entrance smooth (no spring); 800 cap raised to ~5000 (config); sr-only a11y list preserved and extended to levels; `⌘K` is v2-deferred.

---

## 4. Architecture

### 4.1 Module map (new, layered over the reused renderer)

**Migration policy — Option A (committed for v1):** create `features/brain/{model,motion,navigation}`; **keep existing `components/brain/*` in place** and extend them; new components live in `features/brain/components/`. **No bulk move** of existing files. A full migration into `features/brain/components` is deferred to a dedicated post-feature refactor PR (recorded in sdd-init conventions).

```
apps/frontend/src/
  features/brain/                 ← NEW
    model/
      brain-model.ts              ← interface (swappable: client now, backend later)
      client-brain-model.ts       ← builds the tree from the flat /graph response
      types.ts                    ← BrainNode, BrainEdge, BrainLevel, ViewMode, Facets
      facets.ts                   ← stackable facet index + predicate composition
      views/{lobulos,temas,organico}.ts  ← per-view grouping (organico REUSES deriveSimilarityEdges)
    motion/
      interpolator.ts             ← tweens + easing registry + DURATIONS + useReducedMotion
      use-tween.ts                ← R3F-friendly tween hook (single master invalidator)
      motion-store.ts             ← plain mutable singleton (NOT Zustand): active tweens, drives the one useFrame
    navigation/
      use-brain-navigation.ts     ← level path stack (push/pop), breadcrumb state
    components/                    ← view-switcher, segmentation-panel, breadcrumb, level-stats
  components/brain/*               ← REUSED + extended in place (Option A)
```

### 4.2 `brain-model` — the swappable abstraction (D6)

The UI never consumes the raw `/graph` response. It consumes an abstract, view-aware, hierarchical model:

```ts
type ViewMode = 'lobulos' | 'temas' | 'organico';
type NodeKind = 'lobe' | 'neuron' | 'cluster' | 'observation';

interface BrainNodeRef {
  id: string;
  kind: NodeKind;
  label: string;
}

interface BrainNode {
  id: string; // synthetic for aggregates: "lobe:engram", "neuron:engram/auth-model"; "obs:<numericId>" for leaves
  kind: NodeKind;
  label: string;
  color: string; // resolved per active color-by
  weight: number; // aggregate size (member count) or observation weight for leaves
  childCount: number; // 0 ⇒ leaf
  meta: Record<string, unknown>; // dominant type, scope mix, date range, sourceIds count…
  sourceIds?: number[]; // underlying observation ids (stats/detail)
}

type EdgeFamily = 'semantic' | 'topic-road';
interface BrainEdge {
  source: string;
  target: string;
  family: EdgeFamily;
  relation?: GraphEdgeRelation; // dominant relation for semantic bundles
  weight: number; // count of underlying GraphEdge rows → tube RADIUS
  confidence?: number; // average → brightness
}

interface LevelStats {
  nodeCount: number;
  observationCount: number;
  crossSynapses: number;
  dominantType?: string;
}
interface BrainLevel {
  nodes: BrainNode[];
  edges: BrainEdge[];
  parentPath: BrainNodeRef[];
  stats: LevelStats;
}

interface BrainModel {
  view: ViewMode;
  root(filters: ActiveFacets): BrainLevel;
  children(nodeId: string, filters: ActiveFacets): BrainLevel; // empty level for leaves
  search(query: string, filters: ActiveFacets): BrainNodeRef[];
  availableFacets(): FacetGroupId[]; // which facet groups the loaded data supports
}
```

**Leaf contract (C2):** a leaf has `childCount === 0` and `kind === 'observation'`. `children(leafId)` returns an **empty level** `{ nodes: [], edges: [], parentPath, stats }` — never `undefined`/throw. The renderer MUST treat `childCount === 0` as a leaf: selecting it opens the detail panel **without** pushing a level or flying the camera.

**`relation` → `family` mapping (C4):** derived deterministically — `related` → `semantic`; `compatible` → `semantic`; `scoped` → `topic-road`. (Synthetic `deriveSimilarityEdges` roads are always `topic-road`.) _SDD must confirm this mapping against the backend schema before implementation._

**Edge aggregation math (C3 / M4):** when building a non-leaf level, `client-brain-model.ts` merges all underlying edges sharing `(source-aggregate, target-aggregate, family)` into one `BrainEdge`: `weight` = count of distinct underlying `GraphEdge` rows; `confidence` = average of underlying confidences; `relation` = **dominant** (most frequent) relation in the bundle (drives color). _Worked example:_ if observations A, B, C (same neuron) each have a `related` edge to observation D (other neuron), the aggregate edge `(neuron:ABC → neuron:D, family='semantic')` has `weight = 3`. **Edge budget:** if a level yields > 200 edges, cull weakest by `confidence × weight` (at brain level, render only bundles with `weight ≥ threshold`). `deriveSimilarityEdges` already sums weights and is the reference pattern.

- **Client impl** (`client-brain-model.ts`) groups the flat node list (`project`; then `topic_key` for Temas; reuse `deriveSimilarityEdges` clusters for Orgánico) and re-points/aggregates edges as above.
- **Future backend impl** implements the same interface against `/graph/lobes`, `/graph/lobe/:p`, `/graph/neuron/:t` — **UI unchanged**.

### 4.3 Navigation = generic level stack (D4)

Drill depth is **view-dependent** and not hardcoded. Per-view hierarchy matrix (M1):

| View         | Level 0 `root()`        | Level 1 `children()`    | Level 2 `children()`         |
| ------------ | ----------------------- | ----------------------- | ---------------------------- |
| **Lóbulos**  | all lobes (≈ #projects) | observations in lobe    | — (leaf)                     |
| **Temas**    | all lobes               | topic-neurons in lobe   | observations in topic (leaf) |
| **Orgánico** | all clusters            | observations in cluster | — (leaf)                     |

`use-brain-navigation.ts` keeps a **path stack** of `BrainNodeRef`. Entering a non-leaf node pushes; breadcrumb/Escape pops. The renderer always shows `children(top-of-stack)` (or `root()` when empty). Selecting a **leaf** opens the right detail panel without pushing a canvas level.

### 4.4 `interpolator` — the single motion layer (D5)

> NEW module. It **will** replace the direct linear `0.12` lerp in `CameraFocus` (`graph-scene.tsx:94`) with eased tweens.

```ts
type Easing = (t: number) => number;
export const easeOutExpo, easeInOutCubic, easeOutCubic, linear: Easing;
export const DURATIONS = { cameraFlyTo: 700, nodeEnter: 400, dimBrighten: 300 } as const; // ms
export function useReducedMotion(): boolean; // true ⇒ all tweens scale to ≤50ms
export interface TweenSpec<T> {
  from: T;
  to: T;
  durationMs: number;
  easing?: Easing;
}
export function tween<T>(spec, onUpdate, onComplete?): TweenHandle; // number | Vector3 | Color
```

**Single master invalidator (C5):** all frame-driving consolidates into ONE motion manager (`motion-store.ts`, Zustand). The existing `CameraFocus` `0.12` lerp is **absorbed** into the interpolator (700ms `easeOutExpo`). Exactly **one** `useFrame` calls `invalidate()` while any tween is active and stops when none are — preserving `frameloop="demand"` with no competing loops and no idle frames.

**Reduced motion (M5):** `useReducedMotion()` centralizes the `prefers-reduced-motion` check; components never re-query the media query. When reduced, all `DURATIONS` collapse to ≤50ms.

### 4.5 View modes (D1/D2/D3)

Each view module exports `group(nodes) → aggregates`, `defaultColorBy`, and optional `layoutHints` (cluster gravity keys for the worker). The switcher (`view-switcher.tsx`, right of title) sets the active view; changing view re-derives the model and re-runs layout with an eased re-settle. Color-by is a **secondary** compact control; default follows the view (`lobulos→project`, `temas→topic`, `organico→cluster`) but the user can still pick **type**.

**Orgánico determinism (M2):** clustering uses a **fixed seed** (config constant) for v1 — no user knobs. The same `/graph` response ⇒ identical cluster assignments. The model **caches** assignments keyed by observation id, so view switches (Lóbulos↔Orgánico↔…) preserve each observation's node identity (enabling morph, not pop). If Orgánico clustering fails, the model **falls back to Temas** (graceful, no hard error).

### 4.6 Synapses (D7)

Extends `tunnel-flow.tsx`:

- **Base ambient** opacity low (~0.12–0.18); the brain "breathes".
- **Focus state:** hovering/selecting a node raises opacity on its incident edges (eased), dims the rest (`TunnelFocusLayer`, new).
- **Encoding:** color = dominant relation, **radius** = `edge.weight` (underlying count; aggregate levels = "nerve bundles"), **brightness** = `confidence`.
- **Thickness is rendered via `TubeGeometry`, NOT `lineWidth` (C6)** — `gl.lineWidth` is clamped to 1px across browsers. Tubes are batched into a single merged geometry / instanced mesh to keep draw calls low. Base lines may stay as `lineSegments` only where radius is constant.
- **Two families distinguished:** semantic = solid/brighter tubes; topic-road = fainter/dashed.
- Particle flow stays GPU-driven (`uTime`), still distance-gated (LOD) per the graph3d-viz skill.

### 4.7 Segmentation panel + facets (D8/D9)

- Left collapsible panel: **search box** (v1: filters by `label.includes`, AND-composed with facet predicates) + facet groups (Project, Type, Scope, Tool, Date) each **multi-select**, composed as **AND across groups / OR within group** (`facets.ts`).
- **Legend** maps colors → names for the active color-by.
- Non-matching nodes **dim** (reuse isolate-and-dim, now eased) rather than disappear, preserving spatial memory.
- **Per-level stats** strip (e.g., "Lobe engram · 38 neurons · 214 obs · 9 cross-synapses").
- **Facet availability mechanism (C8):** `BrainModel.availableFacets()` inspects the loaded `GraphNode` shape once at init. Facet groups whose backing field is absent (`tool_name`, `session_id` today) are **hidden** (not grayed). v2 backend enablement unpins them with **no UI churn**. `⌘K` palette is **v2-deferred**.

### 4.8 Detail panel + neighbors (D7 payoff)

`node-detail-panel.tsx` gains a **Neighbors / Synapses** section listing the selected observation's related nodes, color-coded by relation, each a quick-jump that selects/enters the target. Markdown + wikilinks unchanged.

**Neighbor resolution (M9):** neighbors = edges incident to the selection in the **current visible level**. If the selection is a **leaf** (`childCount === 0`) and the current level has no incident edges, resolve neighbors from the **parent level's** edges where this observation is source/target — enabling cross-level quick-jump. Neighbors pointing outside the loaded data render as `[no cargado]` (graceful).

### 4.9 View-change & drill lifecycle (M3 / C7)

Deterministic sequence (prevents camera flying to stale centroids / nodes fading at unstable positions):

1. Fade-dim the current node set (`easeInOutCubic`, `DURATIONS.dimBrighten`).
2. Re-derive the target `BrainLevel` from the model.
3. Post node/edge data to the layout worker.
4. On worker `onmessage` (positions ready) set `layoutReady = true`. The interpolator **does not** fire entrance/camera tweens while `!layoutReady`.
5. Then fire **concurrently:** camera fly-to (`DURATIONS.cameraFlyTo`, `easeOutExpo`) + **staggered node entrance** (`DURATIONS.nodeEnter`, `easeOutCubic`). Positions update via refs — no per-frame `setState`.

**Perceived-latency target:** at normal load (≤ a few hundred nodes per level) the full drill transition (dim 300ms → worker layout → camera 700ms + entrance 400ms) should feel under ~1.2s end-to-end; the worker layout step is the only variable cost. If layout exceeds ~600ms, show the dimmed prior level (not a blank canvas) until `layoutReady`. This is an `sdd-verify` acceptance criterion.

**Drill node-set swap (C7) — cross-fade two instance sets:** because a single `InstancedMesh` has a fixed buffer, expanding/collapsing levels does **not** mutate one mesh. Instead: the **outgoing** instance set freezes at old positions and fades 1→0; the **incoming** set renders at the new layout and fades 0→1 over `DURATIONS.nodeEnter` (`easeOutCubic`); the outgoing mesh unmounts on completion. Costs ~one extra draw call during the transition. (Drop the earlier "morph, not pop" hand-waving — this is the committed mechanism; node _identity_ is still preserved across view switches via the model's id cache, §4.5.)

---

## 5. Data Flow

```
/graph (flat, ≤800)  ──►  client-brain-model (build tree per active view + facets)
                                   │
              ┌────────────────────┼─────────────────────┐
              ▼                    ▼                     ▼
        BrainLevel (root)   navigation stack        facet predicate (+ search includes)
              │                    │                     │
              ▼                    ▼                     ▼
        graph-scene (R3F) ◄── use-brain-navigation ──► segmentation panel
              │
   instanced nodes (cross-fade swap) + TubeGeometry synapses + interpolator-driven camera/opacity
```

Single TanStack Query fetch of `/graph` (unchanged). All aggregation, filtering, and leveling happen client-side in `brain-model`, memoized by `(view, filters, path)`.

---

## 6. States

- **Loading:** skeleton brain (dim instanced sphere cloud, slow ambient) + panel skeletons.
- **Empty (no observations):** centered "El cerebro está vacío" + hint to create memories.
- **Empty filter result:** breadcrumb **remains**; current level shown dimmed with "0 coincidencias" + a **Clear filters** action (preserves spatial memory).
- **Truncation:** when `meta.truncated`, non-blocking notice with `truncatedAt`. With LOD each level stays small (<50 nodes typical) so the cap is rarely material; neighbors outside the loaded set render as `[no cargado]`.
- **Error:** retry card (reuse existing query error handling). Orgánico clustering failure ⇒ silent fallback to Temas.

---

## 7. Accessibility

- `accessible-node-list.tsx` extended to be **level-aware** (lists current level's nodes; reflects breadcrumb).
- View switcher, facets, breadcrumb are real focusable controls with `aria-pressed`/`aria-current`.
- Keyboard: Escape pops a level; arrow/enter traverse the sr-only list.
- `prefers-reduced-motion`: centralized via `useReducedMotion()` (§4.4) — all tweens ≤50ms.

---

## 8. Performance (graph3d-viz constraints)

- Keep **one `InstancedMesh`** per node set, **demand frameloop**, **worker layout**, **GPU tunnels**. Never `setState` per frame; mutate refs in `useFrame`; `invalidate()` only while a tween runs (single master invalidator, §4.4).
- LOD wins: levels render far fewer nodes (≈45 lobes → ≈38 neurons → ≈14 observations) than today's 490–800 flat.
- Cap raised to ~5000 (config) is safe for a local-first single-binary Go + SQLite app; payload is lightweight node rows. Confirm the worker (~200 iters) settles < 2s at the 5000 cap (smoke test).
- Synapse tubes batched into one merged/instanced geometry; edge budget cull at >200 (§4.2).
- Budget: draw calls low, frame time < 16ms (verify with `r3f-perf`).

---

## 9. Reused vs New vs Changed (PR slicing)

- **Reused as-is:** `force-layout.worker.ts`, `markdown-content.tsx`, instancing core of `graph-nodes.tsx`, GPU shader core + `deriveSimilarityEdges` of `tunnel-flow.tsx`, `/graph` endpoint + `GraphNode/Edge/Meta`.
- **New modules:** `features/brain/model/*`, `features/brain/motion/{interpolator,use-tween,motion-store}.ts`, `features/brain/navigation/use-brain-navigation.ts`, `features/brain/components/{view-switcher,segmentation-panel,breadcrumb,level-stats}.tsx`.
- **Changed:** `graph-scene.tsx` (camera→interpolator, level transitions, cross-fade swap), `graph-nodes.tsx` (level-aware, eased entrance, animated halos), `tunnel-flow.tsx` (focus layer + tube encoding + family styling — **pre-sliced**, see below), `node-detail-panel.tsx` (neighbors), `brain-page.tsx` (new layout), `node-colors.ts` (legend), `accessible-node-list.tsx` (level-aware).
- **Refactored, not deleted:** `project-rail.tsx` → `ProjectFacetGroup` inside `segmentation-panel/`; `color-by-toggle.tsx` → secondary compact control.

**`tunnel-flow.tsx` decomposition (M6, file is ~758 lines — keep PRs < 400):**

- `TunnelFocusLayer` (focus-state dim/brighten) ~120 lines
- relation-coloring shader tweaks ~80 lines
- topic-road dashing + tube radius encoding ~100 lines
- aggregate edge bundling consumed from model ~60 lines

**Post-change page layout (M7):**

```
┌───────────────────────────────────────────────────────────────┐
│ 🧠 Brain  [Lóbulos·Temas·Orgánico]  Cerebro›engram›…   Color ▾ │
├──────────────┬─────────────────────────────────┬──────────────┤
│ Segmentation │            BRAIN CANVAS          │   Detail +   │
│  search      │     (breadcrumb + level-stats)   │   Neighbors  │
│  facets      │                                  │ (on select)  │
│  legend      │                                  │              │
└──────────────┴─────────────────────────────────┴──────────────┘
        (side L/R project rails REMOVED — Project is now a facet group)
```

Suggested PR chain: (1) interpolator + motion-store, (2) brain-model + facets + tests, (3) navigation + view-switcher + breadcrumb, (4) graph-scene/nodes level rendering + cross-fade, (5) tunnel-flow synapse slices, (6) segmentation-panel + detail neighbors + page layout.

---

## 10. Testing Strategy

- **Unit (Vitest):**
  - `brain-model` aggregation per view; assert `root().nodes.length ≈ #projects` (Lóbulos/Temas) and `> 0` clusters (Orgánico); edge merge weight/confidence/dominant-relation; `children(leafId)` returns empty level.
  - **Round-trip identity:** Lóbulos→Orgánico→Lóbulos preserves each observation's node id (M2).
  - `facets` predicate (AND across / OR within); `availableFacets()` hides Tool/Session when absent.
  - `interpolator` easing math + tween lifecycle; **reduced-motion toggles output durations to ≤50ms** (M5).
  - navigation stack push/pop; neighbor resolution falls back to parent level at leaves (M9).
- **Component (RTL):** view switcher re-derives model; facet selection dims correct nodes; breadcrumb pops; detail neighbors quick-jump; focus-state opacity assertions on the scene graph.
- **Smoke/visual (Chrome DevTools MCP):** load `/brain`, switch views, drill in/out, screenshot each level; GPU particle/animation verified visually + `r3f-perf` frame time.
- Test runner/config resolved at SDD `sdd-init`.

---

## 11. Risks & Mitigations

| Risk                                            | Mitigation                                                                                  |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------- |
| Orgánico clustering quality (TF-IDF is lexical) | Reuse `deriveSimilarityEdges`; treat Orgánico as exploratory; fixed seed; fallback to Temas |
| Re-layout on view change feels jarring          | Lifecycle sequence §4.9 + cross-fade; node identity preserved via model id cache            |
| Aggregate edge explosion at brain level         | Bundle by (src,tgt,family); cull > 200 by confidence×weight                                 |
| `lineWidth` clamp hides thickness               | TubeGeometry radius encoding (C6)                                                           |
| Competing `invalidate()` loops                  | Single master invalidator / motion-store (C5)                                               |
| Scope creep (Tool/Session facets need backend)  | `availableFacets()` hides them; backend-free per D6                                         |
| `tunnel-flow.tsx` mega-diff                     | Pre-sliced (M6); chained PRs < 400 lines                                                    |
| `features/brain/` move churn                    | Option A: extend in place, defer bulk move (M8)                                             |

---

## 12. Out of Scope (YAGNI for v1)

- Backend hierarchy endpoints (deferred per D6; interface ready).
- Embeddings/semantic vectors for Orgánico (lexical TF-IDF for v1).
- Editing/curating memories from the brain.
- Real-time updates / "conversation mode" (disabled toggle stays disabled).
- New DB columns; exposing `tool_name`/`session_id`. **v2 acceptance criterion:** when the backend adds these to `GraphNode`, `availableFacets()` surfaces the Tool/Session facet groups with **zero layout/UI changes**.
- `⌘K` command palette (v2).

---

## 13. SDD Handoff

This design becomes the input to the SDD cycle:

- **proposal** ← §1–3 (intent, scope, decisions)
- **spec** ← §4–8 (requirements & scenarios per module: brain-model incl. leaf/family/aggregation contracts, interpolator, views, navigation, synapses, segmentation, detail, states, a11y)
- **design** ← §4, §8, §9, §11 (architecture + performance + PR slicing + risks)
- **tasks** ← §9 PR chain + §10 (work units; chained PRs if > 400 lines)

**Confirm before implementation:** the `relation → family` mapping (§4.2 C4) against the backend schema.

Suggested change name: `brain-neural-architecture`.
