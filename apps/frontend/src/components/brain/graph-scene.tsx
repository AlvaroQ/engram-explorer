// Main R3F canvas: demand frameloop, instanced nodes, merged tunnel edges.
// C4: Level-aware scene driven by BrainModel + use-brain-navigation.
// Spawns the force-layout worker, wires positions to GraphNodes + TunnelFlow.
// M3 lifecycle: dim → re-derive → post worker → gate tweens on layoutReady
//               → fire camera fly-to + staggered node entrance concurrently.
import { Canvas, invalidate, useFrame, useThree } from '@react-three/fiber';
import { OrbitControls, Stats } from '@react-three/drei';
import { useEffect, useRef, useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import * as THREE from 'three';
import type { GraphNode, GraphEdge } from '../../lib/api.ts';
import { GraphNodes, BrainLevelNodes } from './graph-nodes.tsx';
import { ProjectTitleOverlay } from './project-title-overlay.tsx';
import { TunnelFlow } from './tunnel-flow.tsx';
import { BrainTunnelFlow } from './brain-tunnel-flow.tsx';
import type { ColorBy } from './node-colors.ts';
import { TweenDriver } from './motion/use-tween.ts';
import { tween, easeOutExpo, DURATIONS, useReducedMotion } from './motion/interpolator.ts';
import { motionStore } from './motion/motion-store.ts';
import type { BrainModel, BrainLevel, BrainNavigation, BrainNode } from './types.ts';

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
// Level-aware camera focus: flies to centroid of the current level's nodes
// after layoutReady (M3 step 6).
//
// useFrame is guarded by tweenActiveRef so it only overrides the camera while
// the fly-to is in progress. Once it completes, OrbitControls can orbit freely.
// ---------------------------------------------------------------------------
function LevelCameraFocus({
  positions,
  nodeCount,
  trigger,
}: {
  positions: Float32Array | null;
  nodeCount: number;
  /** Increment to trigger a new fly-to (changes when level changes + layout ready). */
  trigger: number;
}) {
  const camera = useThree((s) => s.camera);
  const controls = useThree((s) => s.controls) as {
    target: THREE.Vector3;
    update: () => void;
  } | null;

  const camPosRef = useRef<THREE.Vector3>(camera.position.clone());
  const targetRef = useRef<THREE.Vector3>(controls?.target.clone() ?? DEFAULT_TARGET.clone());
  // True only while the fly-to tween is in progress.
  const tweenActiveRef = useRef(false);
  // This component's own camera tween handles — cancelled individually on a new
  // fly-to so we never wipe sibling tweens (node burst, cross-fade) via cancelAll.
  const posHandleRef = useRef<ReturnType<typeof tween> | null>(null);
  const tgtHandleRef = useRef<ReturnType<typeof tween> | null>(null);
  // Hoist useReducedMotion to component top level (React hygiene: W4a)
  const reducedMotion = useReducedMotion();

  // Fly to centroid of current level's layout
  useEffect(() => {
    if (!positions || nodeCount === 0 || trigger === 0) return;

    const center = new THREE.Vector3();
    for (let i = 0; i < nodeCount; i++) {
      center.x += positions[i * 3] ?? 0;
      center.y += positions[i * 3 + 1] ?? 0;
      center.z += positions[i * 3 + 2] ?? 0;
    }
    center.multiplyScalar(1 / nodeCount);

    // Compute bounding radius
    let radius = 0;
    for (let i = 0; i < nodeCount; i++) {
      const dx = (positions[i * 3] ?? 0) - center.x;
      const dy = (positions[i * 3 + 1] ?? 0) - center.y;
      const dz = (positions[i * 3 + 2] ?? 0) - center.z;
      radius = Math.max(radius, Math.hypot(dx, dy, dz));
    }

    const fov = ((camera as THREE.PerspectiveCamera).fov ?? 50) * (Math.PI / 180);
    const distance = Math.max(25, (radius / Math.tan(fov / 2)) * 1.5);

    const currentTarget = controls?.target ?? DEFAULT_TARGET;
    const dir = camera.position.clone().sub(currentTarget);
    if (dir.lengthSq() < 1e-6) dir.set(0, 0, 1);
    dir.normalize();

    const goalCamPos = center.clone().add(dir.multiplyScalar(distance));
    const fromPos = camera.position.clone();
    const fromTarget = controls?.target.clone() ?? DEFAULT_TARGET.clone();

    // Initialise refs to current values so useFrame starts from the right place.
    camPosRef.current = fromPos.clone();
    targetRef.current = fromTarget.clone();
    tweenActiveRef.current = true;

    // Cancel only THIS component's previous camera tweens — never cancelAll(),
    // which would also kill the concurrent node burst / cross-fade tweens.
    posHandleRef.current?.cancel();
    tgtHandleRef.current?.cancel();

    const posHandle = tween<THREE.Vector3>(
      { from: fromPos, to: goalCamPos, durationMs: DURATIONS.cameraFlyTo, easing: easeOutExpo },
      (pos) => {
        camPosRef.current = pos;
      },
      undefined,
      reducedMotion,
    );
    posHandleRef.current = posHandle;
    motionStore.register(posHandle);

    const tgtHandle = tween<THREE.Vector3>(
      { from: fromTarget, to: center, durationMs: DURATIONS.cameraFlyTo, easing: easeOutExpo },
      (t) => {
        targetRef.current = t;
      },
      // When the tween finishes, release the camera so OrbitControls can take over.
      () => {
        tweenActiveRef.current = false;
      },
      reducedMotion,
    );
    tgtHandleRef.current = tgtHandle;
    motionStore.register(tgtHandle);
  }, [trigger, positions, nodeCount, camera, controls, reducedMotion]);

  // Apply tween-driven values ONLY while the fly-to is active.
  // Skipping this block when tweenActiveRef is false lets OrbitControls orbit freely.
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
// Props for legacy mode (flat graph, no BrainModel)
// ---------------------------------------------------------------------------

interface LegacyProps {
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
   * Run the continuous edge-particle animation (frameloop="always"). Off for the
   * small preview cards so the overview page doesn't drive several 60fps canvases.
   */
  animate?: boolean | undefined;
  /**
   * Fired once the force-layout worker delivers node positions (i.e. the nodes are
   * about to render). Lets the page dismiss its loading overlay exactly when the
   * graph becomes visible, instead of guessing. Stable callback expected.
   */
  onReady?: (() => void) | undefined;
  /** BrainModel is NOT provided in legacy mode. */
  brainModel?: undefined;
  navigation?: undefined;
}

// ---------------------------------------------------------------------------
// Props for BrainModel-driven mode (C4.3)
// ---------------------------------------------------------------------------

interface BrainModelProps {
  brainModel: BrainModel;
  navigation: BrainNavigation;
  debug?: boolean | undefined;
  colorBy?: ColorBy | undefined;
  onNodeClick?: ((node: BrainNode) => void) | undefined;
  /** Called when the pointer enters/moves over a new node; null when leaving. */
  onNodeHover?: ((node: BrainNode | null, x: number, y: number) => void) | undefined;
  selected?: BrainNode | null | undefined;
  /** Ids passing the active filters/search; others are dimmed. null = no filter. */
  matchedIds?: Set<string> | null | undefined;
  animate?: boolean | undefined;
  /** Loading state from TanStack Query */
  isLoading?: boolean | undefined;
  /** Error from TanStack Query */
  queryError?: unknown;
  /** Whether the graph was truncated at the server. */
  truncated?: boolean | undefined;
  nodes?: undefined;
  edges?: undefined;
}

type Props = LegacyProps | BrainModelProps;

// ---------------------------------------------------------------------------
// Inner scene (legacy mode)
// ---------------------------------------------------------------------------

function LegacyScene({
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
}: LegacyProps & { positions: Float32Array | null }) {
  // True while the user is actively orbiting/panning (OrbitControls start→end).
  // Passed to GraphNodes so hover raycasting is skipped during a drag — raycasting
  // the full InstancedMesh on every pointermove is what makes orbiting feel choppy.
  const draggingRef = useRef(false);
  return (
    <>
      <ambientLight intensity={0.55} />
      <directionalLight position={[2, 3, 2]} intensity={1.7} />
      <directionalLight position={[-2, -1, -1]} intensity={0.45} color="#a78bfa" />
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
      <TweenDriver />
      {debug ? <Stats /> : null}
    </>
  );
}

// ---------------------------------------------------------------------------
// Inner scene (BrainModel-driven mode, C4.3)
// ---------------------------------------------------------------------------

interface BrainSceneProps {
  level: BrainLevel;
  positions: Float32Array | null;
  levelKey: string;
  cameraTrigger: number;
  /** Drill-in focal point (world coords) — children burst out from here; null = no burst. */
  burstOrigin: THREE.Vector3 | null;
  colorBy: ColorBy;
  onNodeClick?: ((node: BrainNode) => void) | undefined;
  onNodeHover?: ((node: BrainNode | null, x: number, y: number) => void) | undefined;
  selected?: BrainNode | null | undefined;
  matchedIds?: Set<string> | null | undefined;
  debug?: boolean | undefined;
}

function BrainScene({
  level,
  positions,
  levelKey,
  cameraTrigger,
  burstOrigin,
  colorBy,
  onNodeClick,
  onNodeHover,
  selected,
  matchedIds,
  debug,
}: BrainSceneProps) {
  return (
    <>
      <ambientLight intensity={0.55} />
      <directionalLight position={[2, 3, 2]} intensity={1.7} />
      <directionalLight position={[-2, -1, -1]} intensity={0.45} color="#a78bfa" />

      {/* Level-aware instanced nodes with cross-fade + drill-in burst (C4.1 / C4.2).
          The committed level is always a complete (level, positions) pair, so the
          previous level stays visible while the next layout is computed — no blanking. */}
      <BrainLevelNodes
        level={level}
        positions={positions}
        colorBy={colorBy}
        onNodeClick={onNodeClick}
        onNodeHover={onNodeHover}
        selectedId={selected?.id ?? null}
        matchedIds={matchedIds}
        levelKey={levelKey}
        burstOrigin={burstOrigin}
      />

      {/* Edges — C5: BrainTunnelFlow consumes BrainLevel.edges (BrainEdge[]) from the model.
          Positions indexed in same order as level.nodes (worker output). */}
      <BrainTunnelFlow
        level={level}
        positions={positions}
        hoveredId={null}
        selectedId={selected?.id ?? null}
      />

      <OrbitControls makeDefault enableDamping dampingFactor={0.08} onChange={() => invalidate()} />

      {/* Level-aware camera fly-to (C4.3 / M3 step 6) */}
      <LevelCameraFocus
        positions={positions}
        nodeCount={level.nodes.length}
        trigger={cameraTrigger}
      />

      {/* Single master invalidator */}
      <TweenDriver />
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
// Loading state canvas (S9.1): dim sphere cloud skeleton
// ---------------------------------------------------------------------------
function LoadingScene() {
  // Render a small set of dim spheres as a skeleton brain
  const count = 30;
  const ref = useRef<THREE.InstancedMesh>(null!);
  const dummy = useMemo(() => new THREE.Object3D(), []);

  useEffect(() => {
    if (!ref.current) return;
    const mesh = ref.current;
    for (let i = 0; i < count; i++) {
      const t = i * 2.399963;
      const r = (i % 10) * 3;
      dummy.position.set(Math.cos(t) * r, Math.sin(t) * r, (i % 5) - 2);
      dummy.scale.setScalar(0.6 + Math.random() * 0.6);
      dummy.updateMatrix();
      mesh.setMatrixAt(i, dummy.matrix);
    }
    mesh.instanceMatrix.needsUpdate = true;
    invalidate();
  }, [dummy]);

  return (
    <>
      <ambientLight intensity={0.3} />
      <instancedMesh ref={ref} args={[undefined, undefined, count]}>
        <sphereGeometry args={[1.5, 8, 8]} />
        <meshStandardMaterial color="#334155" roughness={0.8} transparent opacity={0.4} />
      </instancedMesh>
    </>
  );
}

// ---------------------------------------------------------------------------
// Public GraphScene component
// ---------------------------------------------------------------------------

export function GraphScene(props: Props) {
  // ---- LEGACY MODE (no brainModel) ----
  if (!props.brainModel) {
    const {
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
    } = props;

    // eslint-disable-next-line react-hooks/rules-of-hooks
    const [positions, setPositions] = useState<Float32Array | null>(null);
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const workerRef = useRef<Worker | null>(null);

    // eslint-disable-next-line react-hooks/rules-of-hooks
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
        frameloop={animate ? 'always' : 'demand'}
        camera={{ position: [0, 0, 60], far: 5000 }}
        dpr={[1, 2]}
        gl={{ antialias: true }}
      >
        <LegacyScene
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

  // ---- BRAIN MODEL MODE (C4.3) ----
  return <BrainModelScene {...props} />;
}

// ---------------------------------------------------------------------------
// BrainModelScene — level-aware scene with "node opens" drill lifecycle (C4.3)
// ---------------------------------------------------------------------------

/**
 * A fully-resolved level on screen: nodes + their layout positions committed
 * atomically. Holding the previous CommittedLevel until the next one is ready is
 * what keeps the canvas populated during the drill transition (no blank frame).
 */
interface CommittedLevel {
  level: BrainLevel;
  positions: Float32Array | null;
  /** Cross-fade key (view + path) — a change drives the outgoing/incoming swap. */
  key: string;
  /** Drill-in focal point (world coords); null for first load / pop / view change. */
  burstOrigin: THREE.Vector3 | null;
  /** navigation.path.length at commit time — used to detect push vs pop. */
  depth: number;
}

/**
 * Translate an origin-centered layout so its centroid sits at `origin`, so the
 * children of a drilled-into node appear where that node was ("opens in place").
 */
function recenterPositions(
  positions: Float32Array,
  count: number,
  origin: THREE.Vector3,
): Float32Array {
  if (count === 0) return positions;
  let cx = 0,
    cy = 0,
    cz = 0;
  for (let i = 0; i < count; i++) {
    cx += positions[i * 3] ?? 0;
    cy += positions[i * 3 + 1] ?? 0;
    cz += positions[i * 3 + 2] ?? 0;
  }
  cx /= count;
  cy /= count;
  cz /= count;
  const ox = origin.x - cx,
    oy = origin.y - cy,
    oz = origin.z - cz;
  const out = positions.slice();
  for (let i = 0; i < count; i++) {
    out[i * 3] = (positions[i * 3] ?? 0) + ox;
    out[i * 3 + 1] = (positions[i * 3 + 1] ?? 0) + oy;
    out[i * 3 + 2] = (positions[i * 3 + 2] ?? 0) + oz;
  }
  return out;
}

function BrainModelScene({
  brainModel,
  navigation,
  debug = false,
  colorBy = 'project',
  onNodeClick,
  onNodeHover,
  selected,
  matchedIds,
  animate: _animate = true,
  isLoading = false,
  queryError,
  truncated = false,
}: BrainModelProps) {
  const { t } = useTranslation();

  // Target level derived from the navigation path (what we WANT to show).
  // Used only for empty-state detection — rendering uses the committed level.
  const targetLevel = useMemo(() => {
    const path = navigation.path;
    if (path.length === 0) {
      return brainModel.root({});
    }
    const top = path[path.length - 1];
    return top ? brainModel.children(top.id, {}) : brainModel.root({});
  }, [brainModel, navigation.path]);

  // Level key includes the view so switching views (at any depth) still drives a
  // cross-fade even when the navigation path is unchanged.
  const levelKey = useMemo(() => {
    const path = navigation.path;
    return `${brainModel.view}:${path.map((r) => r.id).join('/') || 'root'}`;
  }, [brainModel, navigation.path]);

  // Committed level = what is actually rendered. It updates (atomically with its
  // positions) only once the worker layout is ready, so the previous level stays
  // on screen during computation — the canvas is never blanked.
  const [committed, setCommitted] = useState<CommittedLevel | null>(null);
  const committedRef = useRef<CommittedLevel | null>(null);
  const [cameraTrigger, setCameraTrigger] = useState(0);
  const workerRef = useRef<Worker | null>(null);

  // Drill lifecycle: capture focal point P (drill-in) → post worker → on layout
  // ready, re-center children around P and commit atomically + fire camera fly-to.
  useEffect(() => {
    // CB4: re-derive level INSIDE the effect from captured brainModel + path so the
    // posted node/edge data and levelKey are always in lockstep.
    const path = navigation.path;
    const derivedLevel =
      path.length === 0
        ? brainModel.root({})
        : (() => {
            const top = path[path.length - 1];
            return top ? brainModel.children(top.id, {}) : brainModel.root({});
          })();

    const depth = path.length;
    const prev = committedRef.current;

    // Empty level — commit immediately (nothing to lay out).
    if (derivedLevel.nodes.length === 0) {
      const next: CommittedLevel = {
        level: derivedLevel,
        positions: null,
        key: levelKey,
        burstOrigin: null,
        depth,
      };
      committedRef.current = next;
      setCommitted(next);
      return;
    }

    // Drill-in detection: deeper than the committed level → burst the children out
    // from the parent node's current world position ("the node opens").
    let burstOrigin: THREE.Vector3 | null = null;
    if (prev && prev.positions && depth > prev.depth) {
      const parentRef = path[path.length - 1];
      if (parentRef) {
        const idx = prev.level.nodes.findIndex((n) => n.id === parentRef.id);
        if (idx >= 0) {
          burstOrigin = new THREE.Vector3(
            prev.positions[idx * 3] ?? 0,
            prev.positions[idx * 3 + 1] ?? 0,
            prev.positions[idx * 3 + 2] ?? 0,
          );
        }
      }
    }

    // Post node/edge data to the layout worker.
    const nodes = derivedLevel.nodes;

    // I13: derive cluster from node.kind (strong separation by node type)
    // and subCluster from node.meta.project (soft separation by project).
    const kindMap = buildIndexMap(nodes.map((n) => n.kind));
    const projectMap = buildIndexMap(
      nodes.map((n) => (typeof n.meta['project'] === 'string' ? n.meta['project'] : null)),
    );

    const workerNodes = nodes.map((n, i) => ({
      id: i,
      cluster: kindMap.get(n.kind) ?? 0,
      subCluster:
        projectMap.get(typeof n.meta['project'] === 'string' ? n.meta['project'] : null) ?? 0,
    }));
    const workerEdges = derivedLevel.edges
      .map((e) => {
        const si = nodes.findIndex((n) => n.id === e.source);
        const ti = nodes.findIndex((n) => n.id === e.target);
        return si >= 0 && ti >= 0 ? { source: si, target: ti } : null;
      })
      .filter((e): e is { source: number; target: number } => e !== null);

    if (workerRef.current) {
      workerRef.current.terminate();
      workerRef.current = null;
    }

    const worker = new Worker(new URL('./force-layout.worker.ts', import.meta.url), {
      type: 'module',
    });
    workerRef.current = worker;

    worker.onmessage = (e: MessageEvent<WorkerResult>) => {
      // Re-center the new level on the drill origin so the children appear where
      // the parent node was; first load / pop / view change stay origin-centered.
      const positions = burstOrigin
        ? recenterPositions(e.data.positions, derivedLevel.nodes.length, burstOrigin)
        : e.data.positions;

      const next: CommittedLevel = {
        level: derivedLevel,
        positions,
        key: levelKey,
        burstOrigin,
        depth,
      };
      committedRef.current = next;
      setCommitted(next);

      // Camera fly-to (concurrent with the node burst). For a drill-in the new
      // centroid is the burst origin, so the camera zooms into the opened node.
      setCameraTrigger((c) => c + 1);
      invalidate();
    };

    worker.postMessage({ nodes: workerNodes, edges: workerEdges });

    return () => {
      worker.terminate();
      workerRef.current = null;
    };
    // levelKey encodes view + path; brainModel is captured to re-derive the level.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [levelKey, brainModel]);

  // S9.1 Loading state
  if (isLoading) {
    return (
      <div className="relative w-full h-full">
        <Canvas
          frameloop="demand"
          camera={{ position: [0, 0, 80], far: 5000 }}
          dpr={[1, 2]}
          gl={{ antialias: true }}
        >
          <LoadingScene />
          <TweenDriver />
          <OrbitControls makeDefault />
        </Canvas>
        {/* Panel skeleton overlay */}
        <div className="absolute inset-0 pointer-events-none flex items-end justify-center pb-8">
          <div className="h-6 w-48 rounded bg-slate-700/40 animate-pulse" />
        </div>
      </div>
    );
  }

  // S9.5 Error state (delegate to TanStack Query error UI — just render nothing here)
  if (queryError) {
    return null;
  }

  // S9.2 Empty state — only at the root when there is genuinely nothing to show.
  if (!isLoading && targetLevel.nodes.length === 0 && navigation.path.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center w-full h-full text-slate-400">
        <p className="text-lg font-medium">{t('brain.emptyBrain')}</p>
        <p className="text-sm mt-2 text-slate-500">{t('brain.emptyBrainHint')}</p>
      </div>
    );
  }

  return (
    <div className="relative w-full h-full">
      {/* S9.4 Truncation notice — non-blocking banner */}
      {truncated && (
        <div className="absolute top-2 left-1/2 -translate-x-1/2 z-10 px-3 py-1 rounded bg-amber-900/60 text-amber-200 text-xs pointer-events-none select-none">
          {t('brain.truncatedNotice')}
        </div>
      )}

      {/* frameloop="always": the brain "breathes" continuously — synapse particles
          flow without needing an interaction to wake the loop, matching the live
          feel of the overview preview. Background tabs are paused via the
          document.hidden guards in the particle/tween useFrames. */}
      <Canvas
        frameloop="always"
        camera={{ position: [0, 0, 80], far: 5000 }}
        dpr={[1, 2]}
        gl={{ antialias: true }}
      >
        {committed ? (
          <BrainScene
            level={committed.level}
            positions={committed.positions}
            levelKey={committed.key}
            cameraTrigger={cameraTrigger}
            burstOrigin={committed.burstOrigin}
            colorBy={colorBy}
            onNodeClick={onNodeClick}
            onNodeHover={onNodeHover}
            selected={selected}
            matchedIds={matchedIds}
            debug={debug}
          />
        ) : (
          /* Data is loaded but the first layout hasn't committed yet — show the
             skeleton brain instead of a blank canvas. */
          <LoadingScene />
        )}
      </Canvas>
    </div>
  );
}
