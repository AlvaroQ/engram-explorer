/**
 * brain-page.tsx — Flat brain view.
 *
 * Layout:
 *   ┌────┬──────────────────────┬────┐
 *   │ L  │    3D GRAPH CANVAS   │ R  │
 *   │ r  │    [DETAIL overlay]  │ r  │
 *   │ a  │                      │ a  │
 *   │ i  │                      │ a  │
 *   │ l  │                      │ l  │
 *   └────┴──────────────────────┴────┘
 *
 * - Left/right rails: sorted project list split evenly (alphabetical, first half left).
 * - Clicking a project rail button zoom-isolates that project's nodes (matchedIds).
 * - Clicking a node (existing behaviour) zooms camera in + highlights neighbors;
 *   ADDITIVE: also opens NodeDetailPanel as floating overlay (variant="overlay").
 * - Escape/close on either mechanism resets appropriately.
 *
 * Navigation: uses window.location.href for full-page navigation to /ui/* templ pages.
 * No TanStack Router dependency — this component runs as a React island outside the SPA.
 */

import { lazy, Suspense, useMemo, useState, useCallback, useEffect, type JSX } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { X, Search } from 'lucide-react';
import { api, type GraphNode } from '../lib/api.ts';
import { computeNodeKeywords } from '../lib/tfidf.ts';
import { isWebGLAvailable } from '../lib/webgl.ts';
import { Card, CardBody } from '../components/ui/card.tsx';
import { ErrorBoundary } from '../components/ui/error-boundary.tsx';
import { Skeleton } from '../components/ui/skeleton.tsx';
import { BrainLoadingOverlay } from '../components/brain/brain-loading-overlay.tsx';
import { ColorByToggle } from '../components/brain/color-by-toggle.tsx';
import { AccessibleNodeList } from '../components/brain/accessible-node-list.tsx';
import { NodeDetailPanel } from '../components/brain/node-detail-panel.tsx';
import { ProjectRail, type ProjectRailItem } from '../components/brain/project-rail.tsx';
import { KnowledgeSpotlight } from '../components/brain/knowledge-spotlight.tsx';
import { WebGLUnavailableDialog } from '../components/brain/webgl-unavailable-dialog.tsx';
import {
  buildHexPalette,
  nodeColor,
  NULL_HEX,
  type ColorBy,
} from '../components/brain/node-colors.ts';

// Lazy-load the 3D canvas so three.js does NOT enter the initial chunk.
const GraphScene = lazy(() =>
  import('../components/brain/graph-scene.tsx').then((m) => ({ default: m.GraphScene })),
);

// ---------------------------------------------------------------------------
// NodeHoverTooltip — lightweight DOM overlay shown near the cursor.
// pointer-events-none so it never intercepts canvas raycasting.
// Clamped to viewport edges: tooltip width ~288px (max-w-xs), height ~96px est.
// ---------------------------------------------------------------------------

const TOOLTIP_OFFSET_X = 14;
const TOOLTIP_OFFSET_Y = 10;
const TOOLTIP_EST_WIDTH = 296;
const TOOLTIP_EST_HEIGHT = 100;

function NodeHoverTooltip({
  node,
  x,
  y,
  keywords,
  color,
}: {
  node: GraphNode;
  x: number;
  y: number;
  keywords: string[];
  /** Hovered node's color (current colorBy). Tints the tooltip border + background. */
  color?: string | undefined;
}): JSX.Element {
  const { t } = useTranslation();

  // Clamp position so the tooltip doesn't overflow the viewport.
  const left = Math.min(x + TOOLTIP_OFFSET_X, window.innerWidth - TOOLTIP_EST_WIDTH - 8);
  const top = Math.min(y + TOOLTIP_OFFSET_Y, window.innerHeight - TOOLTIP_EST_HEIGHT - 8);

  const label = node.label ?? String(node.id);

  // Format createdAt as a relative/short date.
  let dateLabel: string = t('brain.hover.noDate');
  if (node.createdAt != null) {
    const parsed = new Date(
      node.createdAt.includes('T') ? node.createdAt : node.createdAt.replace(' ', 'T'),
    );
    if (!Number.isNaN(parsed.getTime())) {
      dateLabel = parsed.toLocaleDateString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
      });
    }
  }

  return (
    <div
      role="tooltip"
      className="pointer-events-none fixed z-30 max-w-xs overflow-hidden rounded-lg border border-border bg-surface/80 px-3 py-2 shadow-xl backdrop-blur text-xs"
      // Inline borderColor wins over border-border only when a node color is set.
      style={{ left, top, borderColor: color || undefined }}
    >
      {/* Node-color tint behind the content (negative z-index → above the dark
          glass background, below the text). Keeps text legible while the card
          clearly reads as the hovered node's color. */}
      {color && (
        <div
          aria-hidden
          className="pointer-events-none absolute inset-0 -z-10"
          style={{ backgroundColor: color, opacity: 0.2 }}
        />
      )}

      {/* Label — drawn in the node color so the card's primary text matches its
          border + tint. Wraps to two lines before ellipsizing (line-clamp-2).
          Inline color overrides text-fg only when a color is set. */}
      <p
        className="font-semibold text-fg leading-snug line-clamp-2"
        style={{ color: color || undefined }}
      >
        {label}
      </p>

      {/* Breadcrumb: "project › tag" (e.g. alvaroq.github.io › ui-fix). Plain
          normal/muted text with NO border, so it does not compete with the label. */}
      {(node.project != null || node.type != null) && (
        <div className="mt-1.5 inline-flex items-center gap-1 text-[10px] font-normal text-fg-muted">
          {node.project != null && <span>{node.project}</span>}
          {node.project != null && node.type != null && <span className="text-fg-muted/50">›</span>}
          {node.type != null && <span>{node.type}</span>}
        </div>
      )}

      {/* Keyword chips — top-3 TF-IDF keywords; outlined in the node color
          (border + text), falling back to neutral when the node has no color. */}
      {keywords.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {keywords.slice(0, 3).map((kw) => (
            <span
              key={kw}
              className="rounded border border-border px-1.5 py-0.5 text-[10px] text-fg-muted"
              style={{ color: color || undefined, borderColor: color || undefined }}
            >
              {kw}
            </span>
          ))}
        </div>
      )}

      {/* Date + weight row */}
      <div className="mt-1.5 flex items-center justify-between gap-2 text-[10px] text-fg-muted">
        <span>{dateLabel}</span>
        <span>
          {t('brain.fields.weight')}: {node.weight}
        </span>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Flat brain page
// ---------------------------------------------------------------------------

export function BrainPage(): JSX.Element {
  const { t } = useTranslation();

  // Spotlight palette open state
  const [spotlightOpen, setSpotlightOpen] = useState(false);

  // Color dimension (project / type) — both are meaningful on the flat graph.
  const [colorBy, setColorBy] = useState<ColorBy>('project');

  // The node the camera is currently zoomed into ("entered"); null = overview.
  const [focusNode, setFocusNode] = useState<GraphNode | null>(null);

  // The node whose detail panel is currently open (floating overlay).
  // Kept separate from focusNode so closing the panel doesn't un-zoom the camera.
  const [detailNode, setDetailNode] = useState<GraphNode | null>(null);

  // Hover tooltip state — the hovered node + its last cursor position.
  // Only set when the node identity changes (ref-guard in GraphNodes prevents
  // per-frame setState churn on every mouse-move pixel).
  const [hoverNode, setHoverNode] = useState<GraphNode | null>(null);
  const [hoverPos, setHoverPos] = useState<{ x: number; y: number }>({ x: 0, y: 0 });

  // The project the user has isolated via a rail click. null = all projects visible.
  const [isolatedProject, setIsolatedProject] = useState<string | null>(null);

  // True once the force-layout worker has delivered positions and the nodes are
  // visible. Drives the loading overlay, which covers the whole gap between the
  // tab click and the graph appearing (query → lazy chunk → worker layout).
  const [graphReady, setGraphReady] = useState(false);
  const handleGraphReady = useCallback(() => setGraphReady(true), []);

  // WebGL availability — probed once on mount. When Chrome's "Use graphics
  // acceleration when available" is disabled, the browser may refuse to create a
  // WebGL context, so the R3F <Canvas> can never render. We detect that up-front
  // to show an actionable message (and skip lazy-loading the heavy three.js chunk)
  // instead of a blank canvas behind a perpetual loading overlay.
  const [webglAvailable] = useState(() => isWebGLAvailable());

  // When WebGL is unavailable the canvas is never mounted, so the layout worker
  // never posts positions and onReady never fires. Mark the graph "ready" right
  // away so the loading overlay dismisses immediately instead of spinning until
  // the 12s safety timeout below.
  useEffect(() => {
    if (!webglAvailable) setGraphReady(true);
  }, [webglAvailable]);

  // Auto-open the "enable hardware acceleration" dialog when WebGL can't start.
  // Dismissible — closing it leaves the inline placeholder + ⌘K spotlight usable.
  const [webglDialogDismissed, setWebglDialogDismissed] = useState(false);
  const webglDialogOpen = !webglAvailable && !webglDialogDismissed;
  const handleCloseWebglDialog = useCallback(() => setWebglDialogDismissed(true), []);

  // Fetch the full graph.
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['graph'],
    queryFn: () => api.graph(),
  });

  // Safety net: the overlay covers the whole view, so never trap the user behind it
  // if the layout worker fails to post positions (e.g. a worker error). Force-ready
  // after a generous ceiling; the normal path flips graphReady well before this.
  useEffect(() => {
    if (graphReady || isLoading || !data || data.nodes.length === 0) return;
    const id = window.setTimeout(() => setGraphReady(true), 12000);
    return () => window.clearTimeout(id);
  }, [graphReady, isLoading, data]);

  // Pre-compute top-5 TF-IDF keywords per node; recomputed only when the node
  // set changes.  Passed to NodeHoverTooltip so the tooltip never recomputes
  // keywords on every hover event.
  const keywordsById = useMemo(
    () => (data ? computeNodeKeywords(data.nodes) : new Map<number, string[]>()),
    [data],
  );

  // Pre-compute a lowercased search string per node id for the spotlight palette.
  // Built once per data load (O(n)), then per-keystroke filtering is O(n) .includes().
  const searchableIndex = useMemo(() => {
    const map = new Map<number, string>();
    if (!data) return map;
    for (const n of data.nodes) {
      map.set(
        n.id,
        [n.label, n.project, n.type, n.topicKey].filter(Boolean).join(' ').toLowerCase(),
      );
    }
    return map;
  }, [data]);

  // ---------------------------------------------------------------------------
  // matchedIds — union of project isolation AND node-focus highlight.
  //
  // Priority:
  //   1. If a node is focused: highlight ALL nodes in the SAME project as the
  //      selected node (so the whole project stays active, not just neighbors).
  //      Falls back to direct edge-neighbors when the node has no project.
  //   2. Else if a project is isolated: highlight that project's nodes.
  //   3. Else: null (all at full color).
  // ---------------------------------------------------------------------------
  const matchedIds = useMemo<Set<number> | null>(() => {
    if (!data) return null;

    if (focusNode) {
      const ids = new Set<number>([focusNode.id]);
      if (focusNode.project != null) {
        for (const n of data.nodes) {
          if (n.project === focusNode.project) ids.add(n.id);
        }
      } else {
        for (const e of data.edges) {
          if (e.source === focusNode.id) ids.add(e.target);
          else if (e.target === focusNode.id) ids.add(e.source);
        }
      }
      return ids;
    }

    if (isolatedProject !== null) {
      const ids = new Set<number>();
      for (const n of data.nodes) {
        if (n.project === isolatedProject) ids.add(n.id);
      }
      return ids;
    }

    return null;
  }, [focusNode, isolatedProject, data]);

  // ---------------------------------------------------------------------------
  // Project rail data — sorted by most-recently-used (max createdAt) descending,
  // split first-half (most recent) left / rest right.
  // ---------------------------------------------------------------------------
  const { leftProjects, rightProjects } = useMemo<{
    leftProjects: ProjectRailItem[];
    rightProjects: ProjectRailItem[];
  }>(() => {
    if (!data) return { leftProjects: [], rightProjects: [] };

    const palette = buildHexPalette(data.nodes.map((n) => n.project));
    const counts = new Map<string, number>();
    // Track last-used timestamp (max createdAt) per project.
    // null/invalid dates are treated as -Infinity (oldest possible).
    const lastUsed = new Map<string, number>();

    for (const n of data.nodes) {
      if (n.project == null) continue;
      counts.set(n.project, (counts.get(n.project) ?? 0) + 1);
      const ts = n.createdAt != null ? Date.parse(n.createdAt) : NaN;
      const t = Number.isNaN(ts) ? -Infinity : ts;
      const prev = lastUsed.get(n.project) ?? -Infinity;
      if (t > prev) lastUsed.set(n.project, t);
    }

    // Sort descending by last-used (most recent project first).
    const sorted: ProjectRailItem[] = [...counts.entries()]
      .sort(([a], [b]) => {
        const ta = lastUsed.get(a) ?? -Infinity;
        const tb = lastUsed.get(b) ?? -Infinity;
        return tb - ta; // descending: larger (more recent) timestamp first
      })
      .map(([project, count]) => ({
        project,
        count,
        color: palette.get(project) ?? NULL_HEX,
      }));

    const mid = Math.ceil(sorted.length / 2);
    return {
      leftProjects: sorted.slice(0, mid),
      rightProjects: sorted.slice(mid),
    };
  }, [data]);

  // Select a node → zoom into it and open the detail overlay.
  // Selecting the currently-active node re-confirms focus (the camera stays where
  // it is — focusNodeId is unchanged, so GraphScene fires no fly-to tween) instead
  // of toggling back to overview. Clicking an active node must never trigger a
  // zoom-out that loses the user's vantage point. To leave focus, use the
  // exit-focus "X" button or Escape (both reset focusNode).
  // Shared by the 3D graph (onNodeClick), the accessible list, and the spotlight.
  const handleNodeSelect = useCallback((node: GraphNode) => {
    setFocusNode(node);
    setDetailNode(node);
    // Selecting a node clears project isolation so the neighbor highlight takes precedence.
    setIsolatedProject(null);
  }, []);

  // The project currently "active" in the view — whether the user isolated it
  // from the rail OR selected a node (a node selection highlights its whole
  // project). The rail reflects this so the user can see which project is active.
  const activeProject = isolatedProject ?? focusNode?.project ?? null;

  // Click a rail project:
  //  - if it's already the active project (isolated OR via a selected node),
  //    restore the full view (clear isolation + focus + detail);
  //  - otherwise isolate that project.
  const handleToggleProject = useCallback(
    (project: string) => {
      if (project === activeProject) {
        setIsolatedProject(null);
        setFocusNode(null);
        setDetailNode(null);
      } else {
        setIsolatedProject(project);
        setFocusNode(null);
        setDetailNode(null);
      }
    },
    [activeProject],
  );

  // Close the detail panel without touching the camera focus.
  const handleCloseDetail = useCallback(() => {
    setDetailNode(null);
  }, []);

  // ---------------------------------------------------------------------------
  // Req 1 — Dismiss the detail panel when the user manipulates the camera
  // (drag / pan / wheel-zoom). Wired below via GraphScene's `onUserCameraStart`,
  // which forwards OrbitControls' 'start' event.
  //
  // Why not DOM listeners on the wrapper: OrbitControls captures the pointer on
  // the inner <canvas>, so wrapper-level pointermove never fires during a drag.
  // OrbitControls 'start' fires ONLY on real user input — never on the programmatic
  // fly-to tween — so it won't close the panel that a node click just opened
  // (the tween moves the camera without going through OrbitControls input).
  // ---------------------------------------------------------------------------

  // Navigate from the detail panel's title button or the spotlight.
  // Full-page navigation to /ui/* templ pages — no SPA router needed.
  const handleNavigate = useCallback((path: string) => {
    window.location.href = path;
  }, []);

  // Escape: zoom back out + clear project isolation.
  // Cmd/Ctrl+K: toggle the spotlight palette.
  // The NodeDetailPanel has its own Escape handler (stopPropagation) so closing the
  // panel from Escape won't also reset the graph — they are independent.
  // IMPORTANT: both handlers live in the SAME effect/listener to avoid one cleanup
  // clobbering the other.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setSpotlightOpen((prev) => !prev);
        return;
      }
      if (e.key === 'Escape') {
        // When the spotlight is open, let it handle Escape exclusively.
        // The spotlight's own onKeyDown (on the panel div) calls onClose(),
        // which sets spotlightOpen=false. We must NOT also reset graph state.
        if (spotlightOpen) return;
        setFocusNode(null);
        setIsolatedProject(null);
      }
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [spotlightOpen]);

  // Color of the selected node under the active colorBy (for the detail panel badge).
  const detailNodeColor = useMemo<string | undefined>(() => {
    if (!detailNode || !data) return undefined;
    return nodeColor(detailNode, data.nodes, colorBy);
  }, [detailNode, data, colorBy]);

  // Color of the hovered node under the active colorBy — tints the hover tooltip.
  // Only recomputes when the hovered node identity changes (ref-guarded upstream).
  const hoverNodeColor = useMemo<string | undefined>(() => {
    if (!hoverNode || !data) return undefined;
    return nodeColor(hoverNode, data.nodes, colorBy);
  }, [hoverNode, data, colorBy]);

  // The exit-focus "X" button should be shown when either a node is zoomed or a project isolated.
  const hasFocus = focusNode !== null || isolatedProject !== null;

  const handleExitFocus = useCallback(() => {
    setFocusNode(null);
    setIsolatedProject(null);
    setDetailNode(null);
  }, []);

  // Hover tooltip: called by GraphScene when the hovered node identity changes.
  // clientX/clientY are DOM coordinates from the ThreeEvent nativeEvent.
  const handleNodeHover = useCallback(
    (node: GraphNode | null, clientX: number, clientY: number) => {
      setHoverNode(node);
      if (node !== null) {
        setHoverPos({ x: clientX, y: clientY });
      }
      // hoverPos is intentionally not reset on pointer-out (node === null).
      // The tooltip is hidden by the `hoverNode !== null` guard, so stale
      // coordinates are never rendered.
    },
    [],
  );

  return (
    <div className="flex h-full min-h-[500px] flex-col bg-black">
      {/* Header: color-by toggle (left) + exit-focus X (right).
          Transparent over the black canvas so it reads as one continuous
          3D surface (no separator, no card background). */}
      <div className="flex items-center gap-3 px-4 py-2">
        {/* Spotlight trigger button — positioned as first child before ColorByToggle */}
        <button
          type="button"
          onClick={() => setSpotlightOpen(true)}
          aria-label={t('brain.spotlight.open')}
          title={t('brain.spotlight.open')}
          className="flex h-8 items-center gap-1.5 rounded-md border border-border bg-surface/80 px-2.5 text-xs text-fg-muted hover:text-fg hover:bg-surface-2 focus:outline-none focus:ring-2 focus:ring-accent shrink-0"
        >
          <Search className="h-3.5 w-3.5" />
          <span className="hidden sm:inline">{t('brain.spotlight.open')}</span>
          <kbd className="hidden sm:inline rounded border border-border px-1 py-px text-[9px] font-mono text-fg-muted/70">
            ⌘K
          </kbd>
        </button>
        <ColorByToggle colorBy={colorBy} onChange={setColorBy} compact />
        {hasFocus && (
          <button
            type="button"
            onClick={handleExitFocus}
            aria-label={t('brain.exitFocus')}
            title={t('brain.exitFocus')}
            className="ml-auto flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-border bg-surface/80 text-fg-muted hover:text-fg hover:bg-surface-2 focus:outline-none focus:ring-2 focus:ring-accent"
          >
            <X className="h-4 w-4" />
          </button>
        )}
      </div>

      {/* Full-bleed canvas area — rails float over the graph as overlays (Req 3) */}
      <div className="relative flex-1 min-h-0 overflow-hidden">
        {/* ---- Loading / error / empty states ---- */}
        {error ? (
          <Card className="m-4">
            <CardBody>
              <p className="text-sm text-red-500">
                {error instanceof Error ? error.message : String(error)}
              </p>
              <button
                type="button"
                onClick={() => {
                  void refetch();
                }}
                className="mt-2 rounded bg-accent px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-accent focus:ring-offset-2"
              >
                {t('brain.retry')}
              </button>
            </CardBody>
          </Card>
        ) : (
          <>
            {/* Graph — mounted as soon as data has nodes. While the force-layout
                worker computes positions the loading overlay below sits on top, so
                the "nodes stacked at origin" frame is never seen. */}
            {data && data.nodes.length > 0 && (
              <>
                {/* Canvas wrapper — full-bleed, behind the floating rails (Req 3) */}
                <div
                  className="absolute inset-0 bg-black"
                  role="img"
                  data-brain-canvas
                  aria-label={t('brain.a11y.canvasLabel', {
                    nodes: data.nodes.length,
                    edges: data.edges.length,
                  })}
                >
                  {webglAvailable ? (
                    <ErrorBoundary
                      fallback={
                        <div className="flex h-full w-full items-center justify-center text-sm text-fg-muted">
                          {t('brain.graphError')}
                        </div>
                      }
                    >
                      <Suspense fallback={<Skeleton className="h-full w-full" />}>
                        <GraphScene
                          nodes={data.nodes}
                          edges={data.edges}
                          colorBy={colorBy}
                          animate
                          onNodeClick={handleNodeSelect}
                          onNodeHover={handleNodeHover}
                          selected={focusNode}
                          hovered={hoverNode}
                          detailOpen={detailNode !== null}
                          matchedIds={matchedIds}
                          focusNodeId={focusNode?.id ?? null}
                          onUserCameraStart={handleCloseDetail}
                          onReady={handleGraphReady}
                        />
                      </Suspense>
                    </ErrorBoundary>
                  ) : (
                    // WebGL unavailable (e.g. Chrome hardware acceleration off).
                    // The 3D canvas can't render; show how to fix it. The spotlight
                    // (⌘K), project rails and the accessible node list below still work.
                    <div className="flex h-full w-full flex-col items-center justify-center gap-2 px-6 text-center">
                      <p className="text-base font-medium text-fg">
                        {t('brain.webglUnavailableTitle')}
                      </p>
                      <p className="max-w-md text-sm text-fg-muted">
                        {t('brain.webglUnavailableHint')}
                      </p>
                    </div>
                  )}

                  {/* Screen-reader / keyboard fallback for the WebGL canvas */}
                  <AccessibleNodeList
                    nodes={data.nodes}
                    matchedIds={matchedIds}
                    onSelect={handleNodeSelect}
                  />
                </div>

                {/* Left project rail — floating overlay with glassmorphism (Req 3) */}
                <div className="absolute left-0 top-0 bottom-0 z-10 pointer-events-none">
                  <div className="pointer-events-auto h-full">
                    <ProjectRail
                      items={leftProjects}
                      isolated={activeProject}
                      onToggle={handleToggleProject}
                    />
                  </div>
                </div>

                {/* Right project rail — floating overlay with glassmorphism (Req 3).
                    z-index 10 (same as left rail); detail panel is z-20 so it sits above. */}
                <div className="absolute right-0 top-0 bottom-0 z-10 pointer-events-none">
                  <div className="pointer-events-auto h-full">
                    <ProjectRail
                      items={rightProjects}
                      isolated={activeProject}
                      onToggle={handleToggleProject}
                    />
                  </div>
                </div>

                {/* Floating detail overlay — z-20 so it sits above the rails (Req 3).
                    Positioned top-right inside the canvas area, inset from the right rail. */}
                {detailNode !== null && (
                  <div className="absolute inset-0 pointer-events-none z-20">
                    <NodeDetailPanel
                      node={detailNode}
                      variant="overlay"
                      color={detailNodeColor}
                      onClose={handleCloseDetail}
                      onNavigate={handleNavigate}
                    />
                  </div>
                )}

                {/* Hover tooltip — DOM overlay, pointer-events-none so it never
                    intercepts canvas events. z-30 to appear above rails and detail
                    overlay while the user is just hovering. Clamped to viewport. */}
                {hoverNode !== null && (
                  <NodeHoverTooltip
                    node={hoverNode}
                    x={hoverPos.x}
                    y={hoverPos.y}
                    keywords={keywordsById.get(hoverNode.id) ?? []}
                    color={hoverNodeColor}
                  />
                )}
              </>
            )}

            {/* Empty state — data loaded but there are genuinely no nodes. */}
            {!isLoading && data && data.nodes.length === 0 && (
              <div className="flex h-full flex-col items-center justify-center text-slate-400">
                <p className="text-lg font-medium">{t('brain.emptyBrain')}</p>
                <p className="text-sm mt-2 text-slate-500">{t('brain.emptyBrainHint')}</p>
              </div>
            )}

            {/* Knowledge Spotlight — fixed-position DOM overlay, z-40 (above everything).
                Mounted unconditionally so Cmd/Ctrl+K opens even on an empty graph.
                The empty-state rails (overview/sessions/topics) work without graph nodes.
                Nodes and searchableIndex are guarded for the empty case. */}
            <KnowledgeSpotlight
              open={spotlightOpen}
              onClose={() => setSpotlightOpen(false)}
              nodes={data?.nodes ?? []}
              searchableIndex={searchableIndex}
              onSelectNode={handleNodeSelect}
              onNavigate={handleNavigate}
            />

            {/* WebGL unavailable — modal telling the user to enable hardware
                acceleration. Mounted unconditionally so it shows even on an empty
                graph; self-gates on its `open` prop. */}
            <WebGLUnavailableDialog open={webglDialogOpen} onClose={handleCloseWebglDialog} />

            {/* Unified loading animation — covers the whole "tab clicked → nodes
                visible" gap: the graph query loading AND the force-layout worker
                still computing positions (graphReady flips true via onReady). */}
            {(isLoading || (!!data && data.nodes.length > 0 && !graphReady)) && (
              <BrainLoadingOverlay />
            )}
          </>
        )}
      </div>
    </div>
  );
}
