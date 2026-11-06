// ONE InstancedMesh for the whole graph — single draw call regardless of node
// count. Renders GraphNode[] at positions supplied by the force-layout worker,
// with persistent selection and hover halos. All transform/opacity changes
// mutate refs in useFrame — no setState per frame.
import { useMemo, useRef, useEffect, useState, useCallback } from 'react';
import * as THREE from 'three';
import { invalidate, useFrame, type ThreeEvent } from '@react-three/fiber';
import type { GraphNode } from '../../lib/api.ts';
import { NULL_HEX, buildHexPalette, dynamicKey } from './node-colors.ts';
import type { ColorBy } from './node-colors.ts';
import { tween, easeOutCubic, DURATIONS, useReducedMotion } from './motion/interpolator.ts';
import { motionStore } from './motion/motion-store.ts';

// Reused for lightening node colors into halo colors.
const WHITE = new THREE.Color('#ffffff');

// World radius of one node = base sphere radius × per-instance weight scale.
const NODE_BASE_RADIUS = 1.5;

// Map a node's effective weight to its instance scale, normalized into [0.4 .. 1.2].
// duplicateCount amplifies the weight so collapsed duplicate groups appear larger.
function effectiveWeight(node: GraphNode): number {
  return node.weight + Math.max(0, node.duplicateCount - 1) * 2;
}

function weightScale(w: number, minW: number, maxW: number): number {
  const range = maxW - minW || 1;
  return 0.4 + ((w - minW) / range) * 0.8;
}

const NULL_COLOR = new THREE.Color(NULL_HEX);

// ---------------------------------------------------------------------------
// Stagger helper — computes a per-instance entrance delay (fraction of total).
// Entrance delay index [0, count) maps to a fraction in [0, 0.4] so the last
// instance starts its tween at 40% of the total duration, creating an overlap.
// ---------------------------------------------------------------------------
const STAGGER_FRACTION = 0.4;

function staggerDelay(index: number, count: number): number {
  if (count <= 1) return 0;
  return (index / (count - 1)) * STAGGER_FRACTION;
}

interface LegacyProps {
  nodes: GraphNode[];
  positions: Float32Array | null;
  colorBy?: ColorBy | undefined;
  onNodeClick?: ((node: GraphNode) => void) | undefined;
  /** id of the currently selected node — rendered with a persistent halo. */
  selectedId?: number | null | undefined;
  /**
   * Set of node ids that pass the active filters/search. Nodes not in the set
   * are dimmed. `null` means no filter is active (all nodes at full color).
   */
  matchedIds?: Set<number> | null | undefined;
  /**
   * Called when the hovered node identity changes. Receives the resolved
   * GraphNode (or null on pointer-out) and the DOM cursor coordinates.
   * Only emitted when the hovered node IDENTITY changes — not every mouse-move
   * pixel — so callers can safely call setState without per-frame churn.
   */
  onNodeHover?: ((node: GraphNode | null, clientX: number, clientY: number) => void) | undefined;
  /**
   * Ref flag set true while the user is orbiting/panning the camera (OrbitControls
   * start→end). While true, node raycasting (hover) is skipped so dragging stays
   * fluid — raycasting the whole InstancedMesh on every pointermove is the main
   * source of drag jank. null/undefined → hover always active (e.g. preview cards).
   */
  draggingRef?: React.MutableRefObject<boolean> | undefined;
}

// Factor applied to a node's color when it is filtered out (dimmed, not hidden).
const DIM_FACTOR = 0.1;

// ---------------------------------------------------------------------------
// Internal InstancedMesh renderer for legacy (GraphNode[]) mode
// Used by the public GraphNodes export.
// ---------------------------------------------------------------------------

interface LegacyMeshProps {
  nodes: GraphNode[];
  positions: Float32Array | null;
  colorBy: ColorBy;
  onNodeClick?: ((node: GraphNode) => void) | undefined;
  selectedId?: number | null | undefined;
  matchedIds?: Set<number> | null | undefined;
  /** Called when the hovered node identity changes; null on pointer-out. */
  onNodeHover?: ((node: GraphNode | null, clientX: number, clientY: number) => void) | undefined;
  /** Opacity multiplier applied via MeshStandardMaterial.opacity (cross-fade). */
  opacityRef?: React.MutableRefObject<number>;
  /** If true, animate entrance (opacity 0→1 staggered). If false, show at full opacity immediately. */
  animateEntrance?: boolean;
  /** While true (camera orbit/pan in progress), hover raycasting is skipped. */
  draggingRef?: React.MutableRefObject<boolean> | undefined;
}

function LegacyNodeMesh({
  nodes,
  positions,
  colorBy,
  onNodeClick,
  selectedId,
  matchedIds,
  onNodeHover,
  opacityRef: externalOpacityRef,
  animateEntrance = false,
  draggingRef,
}: LegacyMeshProps) {
  const ref = useRef<THREE.InstancedMesh>(null!);
  const nodeMatRef = useRef<THREE.MeshStandardMaterial>(null!);

  // Skip hover raycasting while the camera is being orbited/panned. R3F raycasts
  // the whole InstancedMesh on every pointermove; doing that during a drag is the
  // main jank source. Returning early (no intersects pushed) also makes R3F fire
  // pointerout once, which clears any stuck hover halo. When draggingRef is absent
  // (e.g. preview cards) it falls back to the native InstancedMesh raycast.
  const raycastGate = useCallback(
    (raycaster: THREE.Raycaster, intersects: THREE.Intersection[]) => {
      if (draggingRef?.current) return;
      const mesh = ref.current;
      // Default nearest-surface pick: R3F sorts intersections by distance, so the
      // node whose surface is closest to the camera (the one visually in front,
      // occluding the rest) wins — that is the node the user sees under the cursor.
      if (mesh) THREE.InstancedMesh.prototype.raycast.call(mesh, raycaster, intersects);
    },
    [draggingRef],
  );
  const dummy = useMemo(() => new THREE.Object3D(), []);
  const colorScratch = useMemo(() => new THREE.Color(), []);
  // Per-instance opacity values for staggered entrance (C4.1)
  // In legacy mode without entrance animation these stay at 1.
  const instanceOpacitiesRef = useRef<Float32Array>(new Float32Array(nodes.length).fill(1));

  const [hovered, setHovered] = useState<number | null>(null);
  // Tracks the last reported hover instanceId — prevents redundant onNodeHover
  // calls on every mouse-move pixel (ref-guarded on the hovered index).
  const hoveredRefLegacy = useRef<number | null>(null);

  const effectiveWeights = useMemo(() => nodes.map(effectiveWeight), [nodes]);
  const [minW, maxW] = useMemo(() => {
    let min = Infinity;
    let max = -Infinity;
    for (const w of effectiveWeights) {
      if (w < min) min = w;
      if (w > max) max = w;
    }
    return [min, max] as const;
  }, [effectiveWeights]);

  // Hoist useReducedMotion to component top level (React hygiene: W4a)
  const reducedMotion = useReducedMotion();

  const dynamicPalette = useMemo(() => {
    const hex = buildHexPalette(nodes.map((n) => dynamicKey(n, colorBy)));
    const colors = new Map<string, THREE.Color>();
    for (const [key, value] of hex) colors.set(key, new THREE.Color(value));
    return colors;
  }, [nodes, colorBy]);

  const colorForNode = useCallback(
    (node: GraphNode): THREE.Color => {
      const key = dynamicKey(node, colorBy);
      return (key !== null && dynamicPalette?.get(key)) || NULL_COLOR;
    },
    [colorBy, dynamicPalette],
  );

  const selectedIndex = useMemo(() => {
    if (selectedId === null || selectedId === undefined) return null;
    const i = nodes.findIndex((n) => n.id === selectedId);
    return i >= 0 ? i : null;
  }, [nodes, selectedId]);

  const selectedHaloColor = useMemo(() => {
    const node = selectedIndex !== null ? nodes[selectedIndex] : undefined;
    return node ? colorForNode(node).clone().lerp(WHITE, 0.15).getStyle() : '#ffffff';
  }, [selectedIndex, nodes, colorForNode]);

  const hoverHaloColor = useMemo(() => {
    const node = hovered !== null ? nodes[hovered] : undefined;
    return node ? colorForNode(node).clone().lerp(WHITE, 0.6).getStyle() : '#e0e7ff';
  }, [hovered, nodes, colorForNode]);

  // Staggered entrance animation (C4.1): animate each instance opacity 0→1.
  useEffect(() => {
    if (!animateEntrance || nodes.length === 0) {
      instanceOpacitiesRef.current = new Float32Array(nodes.length).fill(1);
      return;
    }
    const opacities = new Float32Array(nodes.length).fill(0);
    instanceOpacitiesRef.current = opacities;
    const count = nodes.length;

    for (let i = 0; i < count; i++) {
      const delay = staggerDelay(i, count);
      const idx = i;
      // Build tween per instance: opacity 0 → 1.
      // Effective duration accounts for stagger: total visible animation = nodeEnter.
      // Items staggered in the first STAGGER_FRACTION of the duration, so
      // each item's own tween runs for (1 - delay) fraction of total.
      const itemDuration = DURATIONS.nodeEnter * (1 - delay * 0.5);
      setTimeout(() => {
        if ((opacities[idx] ?? 0) >= 1) return; // already done (e.g. unmounted)
        const handle = tween<number>(
          { from: 0, to: 1, durationMs: itemDuration, easing: easeOutCubic },
          (v) => {
            opacities[idx] = v;
          },
          undefined,
          reducedMotion,
        );
        motionStore.register(handle);
      }, delay * DURATIONS.nodeEnter);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [animateEntrance, nodes.length]);

  // Positions + colors effect
  useEffect(() => {
    if (!positions || !ref.current) return;
    const count = nodes.length;

    for (let i = 0; i < count; i++) {
      const node = nodes[i];
      if (!node) continue;

      dummy.position.set(
        positions[i * 3] ?? 0,
        positions[i * 3 + 1] ?? 0,
        positions[i * 3 + 2] ?? 0,
      );
      const ew = effectiveWeights[i] ?? 1;
      const scale = weightScale(ew, minW, maxW);
      dummy.scale.setScalar(scale);
      dummy.updateMatrix();
      ref.current.setMatrixAt(i, dummy.matrix);

      colorScratch.copy(colorForNode(node));
      if (matchedIds && !matchedIds.has(node.id)) {
        colorScratch.multiplyScalar(DIM_FACTOR);
      }
      ref.current.setColorAt(i, colorScratch);
    }

    ref.current.instanceMatrix.needsUpdate = true;
    if (ref.current.instanceColor) {
      ref.current.instanceColor.needsUpdate = true;
    }
    ref.current.computeBoundingSphere();
    invalidate();
  }, [
    nodes,
    positions,
    dummy,
    colorScratch,
    effectiveWeights,
    minW,
    maxW,
    colorForNode,
    matchedIds,
  ]);

  // W2: Dispose geometry + material on unmount to prevent GPU leaks on level transitions.
  useEffect(() => {
    return () => {
      ref.current?.geometry?.dispose();
      nodeMatRef.current?.dispose();
    };
  }, []);

  useEffect(() => {
    return () => {
      document.body.style.cursor = 'auto';
    };
  }, []);

  useEffect(() => {
    invalidate();
  }, [hovered, selectedIndex]);

  // Apply overall fade opacity each frame (driven by externalOpacityRef from cross-fade).
  // Never calls setState per frame — only mutates material ref (graph3d-viz rule).
  useFrame(() => {
    const mat = nodeMatRef.current;
    if (!mat) return;
    // Sync external fade opacity (from cross-fade swap)
    const externalOp = externalOpacityRef?.current ?? 1;
    mat.opacity = Math.max(0, externalOp);
  });

  // Fresnel rim shader
  useEffect(() => {
    const mat = nodeMatRef.current;
    if (!mat) return;
    mat.onBeforeCompile = (shader) => {
      shader.fragmentShader = shader.fragmentShader.replace(
        '#include <opaque_fragment>',
        /* glsl */ `
          float rimEdge = 1.0 - max(dot(normalize(normal), normalize(vViewPosition)), 0.0);
          outgoingLight *= mix(1.0, 0.22, pow(rimEdge, 2.5));
          #include <opaque_fragment>
        `,
      );
    };
    mat.needsUpdate = true;
  }, []);

  // Click-vs-drag discrimination: record pointer position + time on down;
  // only fire a node click if the pointer moved <5px AND elapsed <300ms.
  const pointerDownRef = useRef<{
    x: number;
    y: number;
    time: number;
    instanceId: number | undefined;
  } | null>(null);

  const handlePointerDown = (event: ThreeEvent<PointerEvent>) => {
    pointerDownRef.current = {
      x: event.clientX,
      y: event.clientY,
      time: performance.now(),
      instanceId: event.instanceId,
    };
  };

  const handleClick = (event: ThreeEvent<MouseEvent>) => {
    const down = pointerDownRef.current;
    if (!down) return;
    const dx = event.clientX - down.x;
    const dy = event.clientY - down.y;
    const dist = Math.hypot(dx, dy);
    const elapsed = performance.now() - down.time;
    // Treat as drag if pointer moved ≥5px or held >300ms — do NOT select node.
    if (dist >= 5 || elapsed > 300) return;
    // Genuine click — stop propagation so OrbitControls doesn't also react.
    event.stopPropagation();
    const id = event.instanceId;
    if (id !== undefined && nodes[id] !== undefined && onNodeClick) {
      onNodeClick(nodes[id]!);
    }
  };

  const handlePointerMove = (event: ThreeEvent<PointerEvent>) => {
    event.stopPropagation();
    const id = event.instanceId;
    if (id !== undefined) {
      // Only update state and fire onNodeHover when the hovered instance actually
      // changes — prevents redundant per-move re-renders on every mouse-move pixel.
      if (hoveredRefLegacy.current !== id) {
        hoveredRefLegacy.current = id;
        setHovered(id);
        if (onNodeHover) {
          const node = nodes[id];
          if (node) onNodeHover(node, event.clientX, event.clientY);
        }
      }
      if (onNodeClick) document.body.style.cursor = 'pointer';
    }
  };

  const handlePointerOut = () => {
    if (hoveredRefLegacy.current !== null) {
      hoveredRefLegacy.current = null;
      setHovered(null);
      if (onNodeHover) onNodeHover(null, 0, 0);
    }
    document.body.style.cursor = 'auto';
  };

  if (nodes.length === 0) return null;

  const interaction = {
    onPointerDown: handlePointerDown,
    onPointerMove: handlePointerMove,
    onPointerOut: handlePointerOut,
    ...(onNodeClick ? { onClick: handleClick } : {}),
  };

  const hoverIndex = hovered !== null && hovered !== selectedIndex ? hovered : null;

  return (
    <>
      <instancedMesh
        ref={ref}
        args={[undefined, undefined, nodes.length]}
        frustumCulled
        raycast={raycastGate}
        {...interaction}
      >
        <sphereGeometry args={[NODE_BASE_RADIUS, 16, 16]} />
        <meshStandardMaterial ref={nodeMatRef} roughness={0.45} metalness={0.15} transparent />
      </instancedMesh>

      <NodeHalo
        index={selectedIndex}
        nodes={nodes}
        positions={positions}
        effectiveWeights={effectiveWeights}
        minW={minW}
        maxW={maxW}
        color={selectedHaloColor}
        opacity={0.55}
        scaleFactor={1.15}
      />
      <NodeHalo
        index={hoverIndex}
        nodes={nodes}
        positions={positions}
        effectiveWeights={effectiveWeights}
        minW={minW}
        maxW={maxW}
        color={hoverHaloColor}
        opacity={0.28}
        scaleFactor={1.3}
      />
    </>
  );
}

// ---------------------------------------------------------------------------
// Public GraphNodes — legacy mode (unchanged API for existing callers)
// ---------------------------------------------------------------------------

export function GraphNodes({
  nodes,
  positions,
  colorBy = 'project',
  onNodeClick,
  onNodeHover,
  selectedId,
  matchedIds,
  draggingRef,
}: LegacyProps) {
  // Stable opacity ref for legacy mode (always 1 — no cross-fade in legacy mode)
  const opacityRef = useRef(1);

  if (nodes.length === 0) return null;

  return (
    <LegacyNodeMesh
      nodes={nodes}
      positions={positions}
      colorBy={colorBy}
      onNodeClick={onNodeClick}
      onNodeHover={onNodeHover}
      selectedId={selectedId}
      matchedIds={matchedIds}
      opacityRef={opacityRef}
      animateEntrance={false}
      draggingRef={draggingRef}
    />
  );
}

// ---------------------------------------------------------------------------
// NodeHalo helpers
// ---------------------------------------------------------------------------

// Opacity multiplier for the x-ray (occluded) halo layer relative to the front one.
const HALO_XRAY_FACTOR = 0.15;

// Selection/hover halo: a 3D wireframe shell around the node (the look the user
// likes). The previous UV-sphere wireframe bunched its lines tightly at the poles,
// so the outline looked uneven and visibly "jumped" as the camera orbited. An
// icosphere (geodesic) has near-uniform edges, so it reads consistently from any
// angle; a slow idle spin gives it life and turns the per-orbit change into steady
// motion.
//
// Two layers keep the selected node findable when an orbit pushes it behind other
// nodes (x-ray):
//   - Front layer: default depthFunc → drawn only where the halo is NOT occluded.
//   - X-ray layer: depthFunc = GreaterDepth → drawn ONLY where the halo sits behind
//     other geometry, at a fainter opacity, so a hidden selection still shows through.
// Both use depthWrite:false and renderOrder > nodes so the node depth is already in
// the buffer when the GreaterDepth test runs.
function HaloMesh({
  x,
  y,
  z,
  radius,
  color,
  opacity,
}: {
  x: number;
  y: number;
  z: number;
  radius: number;
  color: string;
  opacity: number;
}) {
  const ref = useRef<THREE.Group>(null);
  useFrame((_, delta) => {
    const g = ref.current;
    if (!g) return;
    g.rotation.y += delta * 0.35;
    g.rotation.x += delta * 0.12;
  });
  return (
    <group ref={ref} position={[x, y, z]}>
      {/* Front (un-occluded) layer */}
      <mesh raycast={() => null} renderOrder={10}>
        <icosahedronGeometry args={[radius, 2]} />
        <meshBasicMaterial
          color={color}
          wireframe
          transparent
          opacity={opacity}
          depthWrite={false}
        />
      </mesh>
      {/* X-ray (occluded) layer — only where the halo is behind other nodes */}
      <mesh raycast={() => null} renderOrder={11}>
        <icosahedronGeometry args={[radius, 2]} />
        <meshBasicMaterial
          color={color}
          wireframe
          transparent
          opacity={opacity * HALO_XRAY_FACTOR}
          depthWrite={false}
          depthFunc={THREE.GreaterDepth}
        />
      </mesh>
    </group>
  );
}

function NodeHalo({
  index,
  nodes,
  positions,
  effectiveWeights,
  minW,
  maxW,
  color,
  opacity,
  scaleFactor,
}: {
  index: number | null;
  nodes: GraphNode[];
  positions: Float32Array | null;
  effectiveWeights: number[];
  minW: number;
  maxW: number;
  color: string;
  opacity: number;
  scaleFactor: number;
}) {
  if (index === null || !positions) return null;
  const node = nodes[index];
  if (!node) return null;

  const ew = effectiveWeights[index] ?? 1;
  const radius = NODE_BASE_RADIUS * weightScale(ew, minW, maxW) * scaleFactor;
  const x = positions[index * 3] ?? 0;
  const y = positions[index * 3 + 1] ?? 0;
  const z = positions[index * 3 + 2] ?? 0;

  return <HaloMesh x={x} y={y} z={z} radius={radius} color={color} opacity={opacity} />;
}
