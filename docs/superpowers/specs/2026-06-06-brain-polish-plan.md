# Brain Page — Bug Fix & Improvement Plan (polish iteration)

- **Date:** 2026-06-06
- **Status:** Approved for implementation (follow-up to brain-neural-architecture)
- **Source:** 5-agent comprehensive review + opus synthesis

## Critical bugs
- **CB1** — Detail panel collapses to ~0 height: `NodeDetailPanel` root is `absolute … max-h-[calc(100%-1.5rem)] overflow-hidden`; wrapped in a zero-height `relative` div in brain-page → clips all content. Fix: `variant='panel'` (drop absolute, `w-full`, real `max-h` + `overflow-y-auto`); keep `variant='overlay'` default for legacy.
- **CB2** — Detail content 404: synthetic node uses `id:-1` → `getObservation(-1)` 404. Fix: reuse `numId` from `obs:(\d+)`; `enabled: node.id>0` guard + no-data branch.
- **CB3** — Tweens timer-driven not frame-driven: `tween()` uses `setInterval`; `motionStore.tick()` only sweeps, never `step()`. Fix: expose `step(deltaMs)`, drive from `tick()`, remove production interval, `document.hidden` guard.
- **CB4** — Worker reads `currentLevel` from stale closure (`[levelKey]` deps). Fix: re-derive level inside the effect.
- **CB5** — Model `filters` accepted but never applied. Verified intentional (dim-only v1). Fix: rename `_reservedFilters` + JSDoc (doc-only, no behavior change).

## Requested features
- **A — colorBy recolor:** `BrainLevelNodeMesh` ignores `colorBy` (`_colorBy`), reads baked `node.color`. Fix: render-time recolor from `node.meta.project|type` via `buildHexPalette`, add `colorBy` to colors `useEffect` deps so toggling re-stamps instantly. Aggregates keep baked color (v1).
- **B — hover tooltip:** thread `onNodeHover(node,x,y)` from mesh → page; change-gated by instanceId; fixed `pointer-events-none` DOM tooltip with label/kind/project/type/weight/childCount/topicKey.
- **C — detail content:** = CB1 + CB2.

## Improvements (I1–I18)
i18n sweep (I1), observationCount at aggregate levels (I2), organico lastAssignment into closure (I3), legend/Orgánico label fidelity (I4), Escape conflict panel↔nav (I5), outside-close pointerup+canvas guard (I6), ViewSwitcher aria/roving (I7), AccessibleNodeList arrow keys (I8), LevelStats live region (I9), FacetGroup aria-controls (I10), delete orphan project-rail (I11), remove/gate legacy summary cards (I12), worker cluster indices (I13), tunnel break→continue (I14), tunnel geometry rebuild-on-hover → shader uniform (I15), popTo(-1)=reset guard (I16), cross-fade incoming opacity flash init 0 (I17), DRY halos/views (I18).

## Slices (ordered)
1. Motion engine (CB3) — isolated, highest risk. [unit]
2. Pure model/util (CB5 doc, I2, I3, I16, I18-model). [unit]
3. i18n + legend labels + dead code + summary cards (I1, I4, I11, I12, I14). [unit+browser]
4. Detail panel = Feature C (CB1, CB2, I5, I6). [browser]
5. colorBy recolor = Feature A (+ I17). [browser]
6. Hover tooltip = Feature B (+ hover perf). [browser]
7. Worker correctness (CB4, I13). [browser]
8. a11y polish (I7, I8, I9, I10). [a11y]
9. Tunnel geometry perf (I15). [browser]

## Out of scope / deferred
Model-level facet pruning (stays dim-only), aggregate-node recolor via model re-bake, facet groups at aggregate levels, legacy EdgeParticles LOD, search() facet narrowing.

## Risks
Tween rewrite is load-bearing (keep tests green via fake timers); CB5 interpretation (dim-only confirmed from design); colorBy at aggregate levels keeps baked color (v1, flag in PR); panel `variant` must not regress legacy overlay; i18n sweep breadth (key-existence test as guardrail).
