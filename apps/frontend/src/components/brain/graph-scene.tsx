// Main R3F canvas: demand frameloop, instanced nodes, merged tunnel edges.
// Spawns the force-layout worker, wires positions to GraphNodes + TunnelFlow.
// The canvas always uses frameloop="demand"; TweenDriver keeps frames coming
// only while a tween runs or the ambient particle flow is allowed (S9).
//
// S9 decision: the former BrainModel-driven branch (BrainModelScene, level
// drill-in) was removed. No caller ever passed `brainModel` and no BrainModel
// implementation exists in the app, so that branch was unreachable.
import { Canvas, invalidate, useFrame, useThree } from '@react-three/fiber';
import { OrbitControls, Stats } from '@react-three/drei';
import { useEffect, useRef, useState, useMemo } from 'react';
import * as THREE from 'three';
import type { GraphNode, GraphEdge } from '../../lib/api.ts';
import { GraphNodes } from './graph-nodes.tsx';
import { ProjectTitleOverlay } from './project-title-overlay.tsx';
import { TunnelFlow } from './tunnel-flow.tsx';
import type { ColorBy } from './node-colors.ts';
import { TweenDriver } from './motion/use-tween.ts';
import { tween, easeOutExpo, DURATIONS, useReducedMotion } from './motion/interpolator.ts';
import { motionStore } from './motion/motion-store.ts';
import { useAmbientMotion } from './motion/use-ambient-motion.ts';
import { RIM_LIGHT_FALLBACK, RIM_LIGHT_TOKEN, readThemeColor } from './theme-colors.ts';

interface WorkerResult {
  positions: Float32Array;
  count: number;
}

// Default framing the camera returns to when the isolation is cleared.
// Closer than the old z=80 so the overview itself reads with more zoom-in.
const DEFAULT_CAM = new THREE.Vector3(0, 0, 60);
const DEFAULT_TARGET = new THREE.Vector3(0, 0, 0);

// Closest the camera gets when zooming into a single node ("entering" it).
const FOCUS_MIN_DISTANCE = 22;

// Animates the camera to frame the isolated project's nodes. When matchedIds
// becomes a set, it flies to that cluster's centroid and zooms to fit; when it
// clears (back to null), it eases back to the default overview framing.
//
// Camera animation applies directly to the camera object during each tween tick.
// A `useFrame` is active ONLY while a tween is running (tracked via tweenActiveRef),
// so OrbitControls can freely move the camera once the fly-to completes.
function CameraFocus({
  nodes,
  positions,
  matchedIds,
  focusNodeId,
  focusInsetRight,
}: {
  nodes: GraphNode[];
  positions: Float32Array | null;
  matchedIds: Set<number> | null | undefined;
  /** When set, the camera zooms in on this node ("enters" it), framing its highlighted neighbors. */
  focusNodeId?: number | null | undefined;
  /** px reserved on the right by the detail panel — offsets the focus target left. */
  focusInsetRight?: number | null | undefined;
}) {
  const camera = useThree((s) => s.camera);
  const controls = useThree((s) => s.controls) as {
    target: THREE.Vector3;
    update: () => void;
  } | null;
  // Canvas pixel dimensions — needed to convert the panel's px width into world
  // units. Read width/height as PRIMITIVES, not the `size` object: R3F can hand
  // back a fresh `size` object on unrelated re-renders (e.g. a hover updating
  // React state) with identical values. Depending on the object would re-fire the
  // fly-to effect on every such render, snapping the camera back to the framing
  // distance — which, when the user has zoomed in past it, reads as an unwanted
  // "zoom out" on mouse-move. Primitives only change on a real resize.
  const { width: viewportWidth, height: viewportHeight } = useThree((s) => s.size);
  // Only ease back to the overview once the user has actually zoomed in.
  const hasIsolated = useRef(false);
  // Live tween targets — written by tween onUpdate, read in useFrame.
  const camPosRef = useRef<THREE.Vector3>(camera.position.clone());
  const targetRef = useRef<THREE.Vector3>(controls?.target.clone() ?? DEFAULT_TARGET.clone());
  // True only while a camera tween is in progress — drives useFrame guard.
  const tweenActiveRef = useRef(false);
  // This component's own camera tween handles — cancelled individually before a
  // new fly-to so overlapping clicks don't stack competing tweens.
  const posHandleRef = useRef<ReturnType<typeof tween> | null>(null);
  const tgtHandleRef = useRef<ReturnType<typeof tween> | null>(null);
  // Hoist useReducedMotion to component top level (React hygiene: W4a)
  const reducedMotion = useReducedMotion();

  useEffect(() => {
    if (!positions) return;

    // Start a camera fly-to (position + target), cancelling our previous tweens.
    const flyTo = (toPos: THREE.Vector3, toTarget: THREE.Vector3) => {
      const fromPos = camera.position.clone();
      const fromTarget = controls?.target.clone() ?? DEFAULT_TARGET.clone();
      camPosRef.current = fromPos.clone();
      targetRef.current = fromTarget.clone();
      tweenActiveRef.current = true;
      posHandleRef.current?.cancel();
      tgtHandleRef.current?.cancel();

      const ph = tween<THREE.Vector3>(
        { from: fromPos, to: toPos, durationMs: DURATIONS.cameraFlyTo, easing: easeOutExpo },
        (p) => {
          camPosRef.current = p;
        },
        undefined,
        reducedMotion,
      );
      posHandleRef.current = ph;
      motionStore.register(ph);

      const th = tween<THREE.Vector3>(
        { from: fromTarget, to: toTarget, durationMs: DURATIONS.cameraFlyTo, easing: easeOutExpo },
        (t) => {
          targetRef.current = t;
        },
        () => {
          tweenActiveRef.current = false;
        },
        reducedMotion,
      );
      tgtHandleRef.current = th;
      motionStore.register(th);
    };

    // Resolve the focus center + framing radius by priority:
    //  1. focusNodeId → center ON that node, radius spans its highlighted neighbors.
    //  2. matchedIds  → center on the matched cluster centroid (legacy isolate).
    //  3. neither     → ease back to the overview.
    let center: THREE.Vector3 | null = null;
    let radius = 0;

    if (focusNodeId != null) {
      const fi = nodes.findIndex((n) => n.id === focusNodeId);
      if (fi >= 0) {
        const cx = positions[fi * 3] ?? 0;
        const cy = positions[fi * 3 + 1] ?? 0;
        const cz = positions[fi * 3 + 2] ?? 0;
        center = new THREE.Vector3(cx, cy, cz);
        if (matchedIds) {
          nodes.forEach((node, i) => {
            if (node.id !== focusNodeId && matchedIds.has(node.id)) {
              const dx = (positions[i * 3] ?? 0) - cx;
              const dy = (positions[i * 3 + 1] ?? 0) - cy;
              const dz = (positions[i * 3 + 2] ?? 0) - cz;
              radius = Math.max(radius, Math.hypot(dx, dy, dz));
            }
          });
        }
      }
    } else if (matchedIds != null) {
      const idxs: number[] = [];
      nodes.forEach((node, i) => {
        if (matchedIds.has(node.id)) idxs.push(i);
      });
      if (idxs.length > 0) {
        const c = new THREE.Vector3();
        for (const i of idxs) {
          c.x += positions[i * 3] ?? 0;
          c.y += positions[i * 3 + 1] ?? 0;
          c.z += positions[i * 3 + 2] ?? 0;
        }
        c.multiplyScalar(1 / idxs.length);
        center = c;
        for (const i of idxs) {
          const dx = (positions[i * 3] ?? 0) - c.x;
          const dy = (positions[i * 3 + 1] ?? 0) - c.y;
          const dz = (positions[i * 3 + 2] ?? 0) - c.z;
          radius = Math.max(radius, Math.hypot(dx, dy, dz));
        }
      }
    }

    if (center == null) {
      if (!hasIsolated.current) return;
      hasIsolated.current = false;
      flyTo(DEFAULT_CAM.clone(), DEFAULT_TARGET.clone());
      return;
    }

    const fov = ((camera as THREE.PerspectiveCamera).fov ?? 50) * (Math.PI / 180);
    const currentTarget = controls?.target ?? DEFAULT_TARGET;
    // Tight framing (factor < 1 lets the outermost nodes sit at the very edge) so an
    // active project's nodes fill the view. Used for BOTH isolating a project (rail
    // click, centred on the cluster) and entering a single node (centred on the node).
    const paddingFactor = 0.8;
    let distance = Math.max(FOCUS_MIN_DISTANCE, (radius / Math.tan(fov / 2)) * paddingFactor);
    // When focusing a specific node, never zoom OUT past the current distance:
    // selecting a node inside an already-zoomed project should KEEP the zoom (just
    // re-centre on the node), not pull the camera back to frame the whole project.
    // Selecting from far away still zooms IN (min picks the smaller distance).
    if (focusNodeId != null) {
      distance = Math.min(distance, camera.position.distanceTo(currentTarget));
    }

    // Keep the current viewing direction, just move along it to the new distance.
    const dir = camera.position.clone().sub(currentTarget);
    if (dir.lengthSq() < 1e-6) dir.set(0, 0, 1);
    dir.normalize();

    // Offset the focus target sideways so a focused node lands in the visible area
    // to the LEFT of the detail panel (anchored right), not dead-center under it.
    // Shift the target RIGHT by half the reserved width → the node renders LEFT of
    // viewport center by that amount. Clamped so a narrow canvas never pushes the
    // node off-screen. Only applies when focusing a single node.
    let focusCenter = center;
    if (focusNodeId != null && focusInsetRight && focusInsetRight > 0 && viewportHeight > 0) {
      const forward = dir.clone().negate(); // camera → target
      const right = new THREE.Vector3().crossVectors(forward, camera.up);
      if (right.lengthSq() > 1e-6) {
        right.normalize();
        const shiftPx = Math.min(focusInsetRight / 2, viewportWidth * 0.4);
        const worldPerPx = (2 * distance * Math.tan(fov / 2)) / viewportHeight;
        focusCenter = center.clone().add(right.multiplyScalar(shiftPx * worldPerPx));
      }
    }

    const goalCamPos = focusCenter.clone().add(dir.multiplyScalar(distance));
    hasIsolated.current = true;
    flyTo(goalCamPos, focusCenter);
  }, [
    focusNodeId,
    focusInsetRight,
    matchedIds,
    positions,
    nodes,
    camera,
    controls,
    viewportWidth,
    viewportHeight,
    reducedMotion,
  ]);

  // Cancel our own camera tweens if the scene unmounts mid-flight (e.g. navigating
  // away from /brain) so orphaned handles don't keep ticking in the motion store.
  useEffect(
    () => () => {
      posHandleRef.current?.cancel();
      tgtHandleRef.current?.cancel();
    },
    [],
  );

  // Apply tween-driven values to the camera — ONLY while a tween is active.
  // When tweenActiveRef is false, this block is skipped so OrbitControls can
  // move the camera freely without being overwritten every frame.
  useFrame(() => {
    if (!tweenActiveRef.current) return;
    camera.position.copy(camPosRef.current);
    if (controls) {
      controls.target.copy(targetRef.current);
      controls.update();
    } else {
      camera.lookAt(targetRef.current);
    }
  });

  return null;
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface GraphSceneProps {
  nodes: GraphNode[];
  edges: GraphEdge[];
  debug?: boolean | undefined;
  colorBy?: ColorBy | undefined;
  onNodeClick?: ((node: GraphNode) => void) | undefined;
  /** Called when the hovered node identity changes; null on pointer-out. */
  onNodeHover?: ((node: GraphNode | null, clientX: number, clientY: number) => void) | undefined;
  /**
   * Called when the user STARTS manipulating the camera (drag / pan / wheel-zoom).
   * Fires ONLY on real user input via OrbitControls 'start' — NOT on programmatic
   * camera tweens (those bypass OrbitControls). Used to dismiss the floating detail
   * panel when the user moves the graph (Req 1).
   */
  onUserCameraStart?: (() => void) | undefined;
  /** Currently selected node — highlighted with a persistent halo. */
  selected?: GraphNode | null | undefined;
  /**
   * Currently hovered node. Drives a transient project-title overlay over the
   * hovered node's cluster, shown only when its project differs from the selected
   * node's project (so the same cluster never gets two titles).
   */
  hovered?: GraphNode | null | undefined;
  /**
   * True while the node detail dialog is open. When set, the floating project-title
   * overlays are suppressed so they never render over the dialog content (the dialog
   * already shows the project). The drei <Html> overlay otherwise wins the stacking
   * order against the DOM panel, so hiding is more reliable than a z-index tweak.
   */
  detailOpen?: boolean | undefined;
  /** Ids passing the active filters/search; others are dimmed. null = no filter. */
  matchedIds?: Set<number> | null | undefined;
  /** When set, the camera zooms in on this node ("enters" it). null = overview. */
  focusNodeId?: number | null | undefined;
  /**
   * Horizontal space (px) reserved on the RIGHT by an open detail panel. When a
   * node is focused, the fly-to shifts its target right by half this width so the
   * node lands in the visible area to the LEFT of the panel — not dead-center
   * where the panel would cover it. 0/undefined = center exactly on the node.
   */
  focusInsetRight?: number | null | undefined;
  /**
   * Allow the continuous edge-particle animation. The frameloop stays "demand";
   * frames are requested only while the canvas is on screen, the tab is visible
   * and the user has not asked for reduced motion.
   */
  animate?: boolean | undefined;
  /**
   * Fired once the force-layout worker delivers node positions (i.e. the nodes are
   * about to render). Lets the page dismiss its loading overlay exactly when the
   * graph becomes visible, instead of guessing. Stable callback expected.
   */
  onReady?: (() => void) | undefined;
}

// ---------------------------------------------------------------------------
// Inner scene
// ---------------------------------------------------------------------------

function SceneContents({
  nodes,
  edges,
  positions,
  debug,
  colorBy,
  onNodeClick,
  onNodeHover,
  onUserCameraStart,
  selected,
  hovered,
  detailOpen,
  matchedIds,
  focusNodeId,
  focusInsetRight,
  animate = true,
}: GraphSceneProps & { positions: Float32Array | null }) {
  // True while the user is actively orbiting/panning (OrbitControls start→end).
  // Passed to GraphNodes so hover raycasting is skipped during a drag — raycasting
  // the full InstancedMesh on every pointermove is what makes orbiting feel choppy.
  const draggingRef = useRef(false);
  // Ambient particle flow: on only when animating, visible, on screen and not
  // reduced-motion. Drives TweenDriver, the canvas's single invalidator.
  const ambient = useAmbientMotion(animate);
  // Rim light follows the theme token (read once per mount).
  const rimLight = useMemo(() => readThemeColor(RIM_LIGHT_TOKEN, RIM_LIGHT_FALLBACK), []);
  return (
    <>
      <ambientLight intensity={0.55} />
      <directionalLight position={[2, 3, 2]} intensity={1.7} />
      <directionalLight position={[-2, -1, -1]} intensity={0.45} color={rimLight} />
      <GraphNodes
        nodes={nodes}
        positions={positions}
        colorBy={colorBy}
        onNodeClick={onNodeClick}
        onNodeHover={onNodeHover}
        selectedId={selected?.id ?? null}
        matchedIds={matchedIds}
        draggingRef={draggingRef}
      />
      <TunnelFlow nodes={nodes} edges={edges} positions={positions} matchedIds={matchedIds} />
      {/* Floating project titles — hidden while the detail dialog is open so the
          drei <Html> pill never covers the dialog content (the dialog already shows
          the project). They reappear once the dialog is closed. */}
      {!detailOpen && (
        <>
          {/* Animated project title floating over the selected node's cluster. */}
          <ProjectTitleOverlay
            nodes={nodes}
            positions={positions}
            project={selected?.project ?? null}
          />
          {/* Transient title for the hovered node's cluster — only when it belongs to a
              DIFFERENT project than the selected one, so a cluster never shows two titles. */}
          {hovered?.project != null && hovered.project !== (selected?.project ?? null) && (
            <ProjectTitleOverlay
              nodes={nodes}
              positions={positions}
              project={hovered.project}
              subtle
            />
          )}
        </>
      )}
      <OrbitControls
        makeDefault
        enableDamping
        dampingFactor={0.08}
        onStart={() => {
          draggingRef.current = true;
          onUserCameraStart?.();
        }}
        onEnd={() => {
          draggingRef.current = false;
        }}
        onChange={() => invalidate()}
      />
      <CameraFocus
        nodes={nodes}
        positions={positions}
        matchedIds={matchedIds}
        focusNodeId={focusNodeId}
        focusInsetRight={focusInsetRight}
      />
      {/* Single master invalidator — the only useFrame that calls invalidate() */}
      <TweenDriver ambient={ambient} />
      {debug ? <Stats /> : null}
    </>
  );
}

// ---------------------------------------------------------------------------
// Derive a stable integer index for a nullable string key across a node list.
// Returns a map from value → index (null/undefined → index 0 as a "no group" bucket).
// ---------------------------------------------------------------------------
function buildIndexMap(values: (string | null | undefined)[]): Map<string | null, number> {
  const m = new Map<string | null, number>();
  let nextIdx = 1; // 0 reserved for null
  m.set(null, 0);
  for (const v of values) {
    const key = v ?? null;
    if (!m.has(key)) m.set(key, nextIdx++);
  }
  return m;
}

// ---------------------------------------------------------------------------
// Public GraphScene component
// ---------------------------------------------------------------------------

export function GraphScene({
  nodes,
  edges,
  debug = false,
  colorBy = 'project',
  onNodeClick,
  onNodeHover,
  onUserCameraStart,
  selected,
  hovered,
  detailOpen,
  matchedIds,
  focusNodeId,
  focusInsetRight,
  animate = true,
  onReady,
}: GraphSceneProps) {
  const [positions, setPositions] = useState<Float32Array | null>(null);
  const workerRef = useRef<Worker | null>(null);

  useEffect(() => {
    if (nodes.length === 0) return;

    const idToIndex = new Map<number, number>();
    nodes.forEach((n, i) => idToIndex.set(n.id, i));

    const projectMap = buildIndexMap(nodes.map((n) => n.project));
    const topicMap = buildIndexMap(nodes.map((n) => n.topicKey));

    const workerNodes = nodes.map((n, i) => ({
      id: i,
      cluster: projectMap.get(n.project ?? null) ?? 0,
      subCluster: topicMap.get(n.topicKey ?? null) ?? 0,
    }));

    const workerEdges = edges
      .map((e) => {
        const s = idToIndex.get(e.source);
        const t = idToIndex.get(e.target);
        return s !== undefined && t !== undefined ? { source: s, target: t } : null;
      })
      .filter((e): e is { source: number; target: number } => e !== null);

    const worker = new Worker(new URL('./force-layout.worker.ts', import.meta.url), {
      type: 'module',
    });
    workerRef.current = worker;

    worker.onmessage = (e: MessageEvent<WorkerResult>) => {
      setPositions(e.data.positions);
      // Nodes are about to render — let the page dismiss its loading overlay.
      onReady?.();
      invalidate();
    };

    worker.postMessage({ nodes: workerNodes, edges: workerEdges });

    return () => {
      worker.terminate();
      workerRef.current = null;
    };
  }, [nodes, edges, onReady]);

  return (
    <Canvas
      frameloop="demand"
      camera={{ position: [0, 0, 60], far: 5000 }}
      dpr={[1, 2]}
      gl={{ antialias: true }}
    >
      <SceneContents
        nodes={nodes}
        edges={edges}
        positions={positions}
        debug={debug}
        colorBy={colorBy}
        onNodeClick={onNodeClick}
        onNodeHover={onNodeHover}
        onUserCameraStart={onUserCameraStart}
        selected={selected}
        hovered={hovered}
        detailOpen={detailOpen}
        matchedIds={matchedIds}
        focusNodeId={focusNodeId}
        focusInsetRight={focusInsetRight}
        animate={animate}
      />
    </Canvas>
  );
}
