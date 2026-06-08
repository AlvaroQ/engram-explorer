// ONE InstancedMesh per level — single draw call regardless of node count.
// Supports two modes:
//   1. Legacy mode:   nodes: GraphNode[] + positions: Float32Array | null
//   2. BrainLevel mode: level: BrainLevel (C4.1); positions provided separately.
//
// Cross-fade (C4.2 / C7): level transitions render TWO InstancedMesh instances:
//   - Outgoing: freezes at old positions, fades 1→0 (easeOutCubic, DURATIONS.nodeEnter)
//   - Incoming: renders at new positions, fades 0→1 (easeOutCubic, DURATIONS.nodeEnter)
//   Outgoing disposes geometry + material on completion.
//
// Drill-in burst ("node opens"): when an incoming BrainLevel has a burstOrigin,
// each instance starts collapsed at that world point and tweens position+scale to
// its target, staggered by distance — the parent node opening into its children.
// Plain level changes (pop / view switch) just cross-fade. All transform/opacity
// changes mutate refs in useFrame — no setState per frame.
import { useMemo, useRef, useEffect, useState, useCallback } from 'react'
import * as THREE from 'three'
import { invalidate, useFrame, type ThreeEvent } from '@react-three/fiber'
import type { GraphNode } from '../../lib/api.ts'
import { NULL_HEX, buildHexPalette, dynamicKey } from './node-colors.ts'
import type { ColorBy } from './node-colors.ts'
import type { BrainLevel, BrainNode } from '../../features/brain/model/types.ts'
import { tween, easeOutCubic, DURATIONS, useReducedMotion } from '../../features/brain/motion/interpolator.ts'
import { motionStore } from '../../features/brain/motion/motion-store.ts'

// Reused for lightening node colors into halo colors.
const WHITE = new THREE.Color('#ffffff')

// World radius of one node = base sphere radius × per-instance weight scale.
const NODE_BASE_RADIUS = 1.5

// Minimum instance scale used as the "collapsed" start of the drill-in burst.
// Non-zero to keep the instance matrix non-degenerate (avoids zero-determinant warnings).
const BURST_MIN_SCALE = 0.001

// Map a node's effective weight to its instance scale, normalized into [0.4 .. 1.2].
// duplicateCount amplifies the weight so collapsed duplicate groups appear larger.
function effectiveWeight(node: GraphNode): number {
  return node.weight + Math.max(0, node.duplicateCount - 1) * 2
}

function brainNodeWeight(node: BrainNode): number {
  return Math.max(0.5, node.weight)
}

function weightScale(w: number, minW: number, maxW: number): number {
  const range = maxW - minW || 1
  return 0.4 + ((w - minW) / range) * 0.8
}

const NULL_COLOR = new THREE.Color(NULL_HEX)

// ---------------------------------------------------------------------------
// Stagger helper — computes a per-instance entrance delay (fraction of total).
// Entrance delay index [0, count) maps to a fraction in [0, 0.4] so the last
// instance starts its tween at 40% of the total duration, creating an overlap.
// ---------------------------------------------------------------------------
const STAGGER_FRACTION = 0.4

function staggerDelay(index: number, count: number): number {
  if (count <= 1) return 0
  return (index / (count - 1)) * STAGGER_FRACTION
}

interface LegacyProps {
  nodes: GraphNode[]
  positions: Float32Array | null
  colorBy?: ColorBy | undefined
  onNodeClick?: ((node: GraphNode) => void) | undefined
  /** id of the currently selected node — rendered with a persistent halo. */
  selectedId?: number | null | undefined
  /**
   * Set of node ids that pass the active filters/search. Nodes not in the set
   * are dimmed. `null` means no filter is active (all nodes at full color).
   */
  matchedIds?: Set<number> | null | undefined
  /**
   * Called when the hovered node identity changes. Receives the resolved
   * GraphNode (or null on pointer-out) and the DOM cursor coordinates.
   * Only emitted when the hovered node IDENTITY changes — not every mouse-move
   * pixel — so callers can safely call setState without per-frame churn.
   */
  onNodeHover?: ((node: GraphNode | null, clientX: number, clientY: number) => void) | undefined
  /**
   * Ref flag set true while the user is orbiting/panning the camera (OrbitControls
   * start→end). While true, node raycasting (hover) is skipped so dragging stays
   * fluid — raycasting the whole InstancedMesh on every pointermove is the main
   * source of drag jank. null/undefined → hover always active (e.g. preview cards).
   */
  draggingRef?: React.MutableRefObject<boolean> | undefined
  /** BrainLevel is NOT provided in legacy mode. */
  level?: undefined
}


// Factor applied to a node's color when it is filtered out (dimmed, not hidden).
const DIM_FACTOR = 0.1

// ---------------------------------------------------------------------------
// Internal InstancedMesh renderer for legacy (GraphNode[]) mode
// Used by the public GraphNodes export.
// ---------------------------------------------------------------------------

interface LegacyMeshProps {
  nodes: GraphNode[]
  positions: Float32Array | null
  colorBy: ColorBy
  onNodeClick?: ((node: GraphNode) => void) | undefined
  selectedId?: number | null | undefined
  matchedIds?: Set<number> | null | undefined
  /** Called when the hovered node identity changes; null on pointer-out. */
  onNodeHover?: ((node: GraphNode | null, clientX: number, clientY: number) => void) | undefined
  /** Opacity multiplier applied via MeshStandardMaterial.opacity (cross-fade). */
  opacityRef?: React.MutableRefObject<number>
  /** If true, animate entrance (opacity 0→1 staggered). If false, show at full opacity immediately. */
  animateEntrance?: boolean
  /** While true (camera orbit/pan in progress), hover raycasting is skipped. */
  draggingRef?: React.MutableRefObject<boolean> | undefined
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
  const ref = useRef<THREE.InstancedMesh>(null!)
  const nodeMatRef = useRef<THREE.MeshStandardMaterial>(null!)

  // Skip hover raycasting while the camera is being orbited/panned. R3F raycasts
  // the whole InstancedMesh on every pointermove; doing that during a drag is the
  // main jank source. Returning early (no intersects pushed) also makes R3F fire
  // pointerout once, which clears any stuck hover halo. When draggingRef is absent
  // (e.g. preview cards) it falls back to the native InstancedMesh raycast.
  const raycastGate = useCallback(
    (raycaster: THREE.Raycaster, intersects: THREE.Intersection[]) => {
      if (draggingRef?.current) return
      const mesh = ref.current
      // Default nearest-surface pick: R3F sorts intersections by distance, so the
      // node whose surface is closest to the camera (the one visually in front,
      // occluding the rest) wins — that is the node the user sees under the cursor.
      if (mesh) THREE.InstancedMesh.prototype.raycast.call(mesh, raycaster, intersects)
    },
    [draggingRef],
  )
  const dummy = useMemo(() => new THREE.Object3D(), [])
  const colorScratch = useMemo(() => new THREE.Color(), [])
  // Per-instance opacity values for staggered entrance (C4.1)
  // In legacy mode without entrance animation these stay at 1.
  const instanceOpacitiesRef = useRef<Float32Array>(new Float32Array(nodes.length).fill(1))

  const [hovered, setHovered] = useState<number | null>(null)
  // Tracks the last reported hover instanceId — prevents redundant onNodeHover
  // calls on every mouse-move pixel (same ref-guard pattern as BrainLevelNodeMesh).
  const hoveredRefLegacy = useRef<number | null>(null)

  const effectiveWeights = useMemo(() => nodes.map(effectiveWeight), [nodes])
  const [minW, maxW] = useMemo(() => {
    let min = Infinity
    let max = -Infinity
    for (const w of effectiveWeights) {
      if (w < min) min = w
      if (w > max) max = w
    }
    return [min, max] as const
  }, [effectiveWeights])

  // Hoist useReducedMotion to component top level (React hygiene: W4a)
  const reducedMotion = useReducedMotion()

  const dynamicPalette = useMemo(() => {
    const hex = buildHexPalette(nodes.map((n) => dynamicKey(n, colorBy)))
    const colors = new Map<string, THREE.Color>()
    for (const [key, value] of hex) colors.set(key, new THREE.Color(value))
    return colors
  }, [nodes, colorBy])

  const colorForNode = useCallback(
    (node: GraphNode): THREE.Color => {
      const key = dynamicKey(node, colorBy)
      return (key !== null && dynamicPalette?.get(key)) || NULL_COLOR
    },
    [colorBy, dynamicPalette],
  )

  const selectedIndex = useMemo(() => {
    if (selectedId === null || selectedId === undefined) return null
    const i = nodes.findIndex((n) => n.id === selectedId)
    return i >= 0 ? i : null
  }, [nodes, selectedId])

  const selectedHaloColor = useMemo(() => {
    const node = selectedIndex !== null ? nodes[selectedIndex] : undefined
    return node ? colorForNode(node).clone().lerp(WHITE, 0.15).getStyle() : '#ffffff'
  }, [selectedIndex, nodes, colorForNode])

  const hoverHaloColor = useMemo(() => {
    const node = hovered !== null ? nodes[hovered] : undefined
    return node ? colorForNode(node).clone().lerp(WHITE, 0.6).getStyle() : '#e0e7ff'
  }, [hovered, nodes, colorForNode])

  // Staggered entrance animation (C4.1): animate each instance opacity 0→1.
  useEffect(() => {
    if (!animateEntrance || nodes.length === 0) {
      instanceOpacitiesRef.current = new Float32Array(nodes.length).fill(1)
      return
    }
    const opacities = new Float32Array(nodes.length).fill(0)
    instanceOpacitiesRef.current = opacities
    const count = nodes.length

    for (let i = 0; i < count; i++) {
      const delay = staggerDelay(i, count)
      const idx = i
      // Build tween per instance: opacity 0 → 1.
      // Effective duration accounts for stagger: total visible animation = nodeEnter.
      // Items staggered in the first STAGGER_FRACTION of the duration, so
      // each item's own tween runs for (1 - delay) fraction of total.
      const itemDuration = DURATIONS.nodeEnter * (1 - delay * 0.5)
      setTimeout(() => {
        if ((opacities[idx] ?? 0) >= 1) return // already done (e.g. unmounted)
        const handle = tween<number>(
          { from: 0, to: 1, durationMs: itemDuration, easing: easeOutCubic },
          (v) => { opacities[idx] = v },
          undefined,
          reducedMotion,
        )
        motionStore.register(handle)
      }, delay * DURATIONS.nodeEnter)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [animateEntrance, nodes.length])

  // Positions + colors effect
  useEffect(() => {
    if (!positions || !ref.current) return
    const count = nodes.length

    for (let i = 0; i < count; i++) {
      const node = nodes[i]
      if (!node) continue

      dummy.position.set(
        positions[i * 3] ?? 0,
        positions[i * 3 + 1] ?? 0,
        positions[i * 3 + 2] ?? 0,
      )
      const ew = effectiveWeights[i] ?? 1
      const scale = weightScale(ew, minW, maxW)
      dummy.scale.setScalar(scale)
      dummy.updateMatrix()
      ref.current.setMatrixAt(i, dummy.matrix)

      colorScratch.copy(colorForNode(node))
      if (matchedIds && !matchedIds.has(node.id)) {
        colorScratch.multiplyScalar(DIM_FACTOR)
      }
      ref.current.setColorAt(i, colorScratch)
    }

    ref.current.instanceMatrix.needsUpdate = true
    if (ref.current.instanceColor) {
      ref.current.instanceColor.needsUpdate = true
    }
    ref.current.computeBoundingSphere()
    invalidate()
  }, [nodes, positions, dummy, colorScratch, effectiveWeights, minW, maxW, colorForNode, matchedIds])

  // W2: Dispose geometry + material on unmount to prevent GPU leaks on level transitions.
  useEffect(() => {
    return () => {
      ref.current?.geometry?.dispose()
      nodeMatRef.current?.dispose()
    }
  }, [])

  useEffect(() => {
    return () => {
      document.body.style.cursor = 'auto'
    }
  }, [])

  useEffect(() => {
    invalidate()
  }, [hovered, selectedIndex])

  // Apply overall fade opacity each frame (driven by externalOpacityRef from cross-fade).
  // Never calls setState per frame — only mutates material ref (graph3d-viz rule).
  useFrame(() => {
    const mat = nodeMatRef.current
    if (!mat) return
    // Sync external fade opacity (from cross-fade swap)
    const externalOp = externalOpacityRef?.current ?? 1
    mat.opacity = Math.max(0, externalOp)
  })

  // Fresnel rim shader
  useEffect(() => {
    const mat = nodeMatRef.current
    if (!mat) return
    mat.onBeforeCompile = (shader) => {
      shader.fragmentShader = shader.fragmentShader.replace(
        '#include <opaque_fragment>',
        /* glsl */ `
          float rimEdge = 1.0 - max(dot(normalize(normal), normalize(vViewPosition)), 0.0);
          outgoingLight *= mix(1.0, 0.22, pow(rimEdge, 2.5));
          #include <opaque_fragment>
        `,
      )
    }
    mat.needsUpdate = true
  }, [])

  // Click-vs-drag discrimination: record pointer position + time on down;
  // only fire a node click if the pointer moved <5px AND elapsed <300ms.
  const pointerDownRef = useRef<{ x: number; y: number; time: number; instanceId: number | undefined } | null>(null)

  const handlePointerDown = (event: ThreeEvent<PointerEvent>) => {
    pointerDownRef.current = {
      x: event.clientX,
      y: event.clientY,
      time: performance.now(),
      instanceId: event.instanceId,
    }
  }

  const handleClick = (event: ThreeEvent<MouseEvent>) => {
    const down = pointerDownRef.current
    if (!down) return
    const dx = event.clientX - down.x
    const dy = event.clientY - down.y
    const dist = Math.hypot(dx, dy)
    const elapsed = performance.now() - down.time
    // Treat as drag if pointer moved ≥5px or held >300ms — do NOT select node.
    if (dist >= 5 || elapsed > 300) return
    // Genuine click — stop propagation so OrbitControls doesn't also react.
    event.stopPropagation()
    const id = event.instanceId
    if (id !== undefined && nodes[id] !== undefined && onNodeClick) {
      onNodeClick(nodes[id]!)
    }
  }

  const handlePointerMove = (event: ThreeEvent<PointerEvent>) => {
    event.stopPropagation()
    const id = event.instanceId
    if (id !== undefined) {
      // Only update state and fire onNodeHover when the hovered instance actually
      // changes — prevents redundant per-move re-renders on every mouse-move pixel.
      if (hoveredRefLegacy.current !== id) {
        hoveredRefLegacy.current = id
        setHovered(id)
        if (onNodeHover) {
          const node = nodes[id]
          if (node) onNodeHover(node, event.clientX, event.clientY)
        }
      }
      if (onNodeClick) document.body.style.cursor = 'pointer'
    }
  }

  const handlePointerOut = () => {
    if (hoveredRefLegacy.current !== null) {
      hoveredRefLegacy.current = null
      setHovered(null)
      if (onNodeHover) onNodeHover(null, 0, 0)
    }
    document.body.style.cursor = 'auto'
  }

  if (nodes.length === 0) return null

  const interaction = {
    onPointerDown: handlePointerDown,
    onPointerMove: handlePointerMove,
    onPointerOut: handlePointerOut,
    ...(onNodeClick ? { onClick: handleClick } : {}),
  }

  const hoverIndex = hovered !== null && hovered !== selectedIndex ? hovered : null

  return (
    <>
      <instancedMesh ref={ref} args={[undefined, undefined, nodes.length]} frustumCulled raycast={raycastGate} {...interaction}>
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
  )
}

// ---------------------------------------------------------------------------
// BrainLevel InstancedMesh renderer (C4.1 / C4.2)
// ---------------------------------------------------------------------------

interface BrainLevelMeshProps {
  level: BrainLevel
  positions: Float32Array | null
  colorBy?: ColorBy | undefined
  onNodeClick?: ((node: BrainNode) => void) | undefined
  /** Called when the hovered instance changes; null when pointer leaves the mesh. */
  onNodeHover?: ((node: BrainNode | null, x: number, y: number) => void) | undefined
  selectedId?: string | null | undefined
  matchedIds?: Set<string> | null | undefined
  /** Overall opacity ref driven by cross-fade controller. */
  opacityRef: React.MutableRefObject<number>
  /** If true, animate per-instance entrance. */
  animateEntrance?: boolean
  /**
   * Drill-in "node opens" focal point (world coords). When set together with
   * animateEntrance, each instance bursts out from this point (scale+position
   * 0→target, staggered by distance) — the parent node visually opening into
   * its children. When null, entrance is a plain cross-fade (no burst).
   */
  burstOrigin?: THREE.Vector3 | null
}

function BrainLevelNodeMesh({
  level,
  positions,
  colorBy = 'project',
  onNodeClick,
  onNodeHover,
  selectedId,
  matchedIds,
  opacityRef,
  animateEntrance = false,
  burstOrigin = null,
}: BrainLevelMeshProps) {
  const nodes = level.nodes
  const ref = useRef<THREE.InstancedMesh>(null!)
  const nodeMatRef = useRef<THREE.MeshStandardMaterial>(null!)
  const dummy = useMemo(() => new THREE.Object3D(), [])
  const colorScratch = useMemo(() => new THREE.Color(), [])

  // Hoist useReducedMotion to component top level (React hygiene: W4a)
  const reducedMotion = useReducedMotion()

  const [hovered, setHovered] = useState<number | null>(null)
  // Tracks the last reported hover instanceId — prevents redundant onNodeHover calls on each mouse move.
  const hoveredRef = useRef<number | null>(null)

  // Weights derived from BrainNode.weight
  const effectiveWeights = useMemo(() => nodes.map(brainNodeWeight), [nodes])
  const [minW, maxW] = useMemo(() => {
    let min = Infinity
    let max = -Infinity
    for (const w of effectiveWeights) {
      if (w < min) min = w
      if (w > max) max = w
    }
    return [min, max] as const
  }, [effectiveWeights])

  // Dynamic palette mirroring LegacyNodeMesh: keys on node.meta['project'] or node.meta['type'].
  // Aggregate nodes (lobes/neurons/clusters) that lack the meta field fall back to node.color.
  const dynamicBrainPalette = useMemo(() => {
    const keyFn = (n: BrainNode): string | null => {
      if (colorBy === 'project') {
        const v = n.meta['project']
        return typeof v === 'string' && v ? v : null
      }
      if (colorBy === 'type') {
        const v = n.meta['type']
        return typeof v === 'string' && v ? v : null
      }
      return null
    }
    const hex = buildHexPalette(nodes.map(keyFn))
    const colors = new Map<string, THREE.Color>()
    for (const [key, value] of hex) colors.set(key, new THREE.Color(value))
    return { colors, keyFn }
  }, [nodes, colorBy])

  const colorForBrainNode = useCallback(
    (node: BrainNode): THREE.Color => {
      const key = dynamicBrainPalette.keyFn(node)
      if (key !== null) {
        const c = dynamicBrainPalette.colors.get(key)
        if (c) return c
      }
      // Fallback: use baked node.color for aggregate nodes that lack the meta field
      return colorScratch.set(node.color || NULL_HEX)
    },
    [dynamicBrainPalette, colorScratch],
  )

  const selectedIndex = useMemo(() => {
    if (!selectedId) return null
    const i = nodes.findIndex((n) => n.id === selectedId)
    return i >= 0 ? i : null
  }, [nodes, selectedId])

  // Drill-in burst state (C4.1 / "node opens"). Per-instance target transform
  // buffers + a single 0→1 progress ref driven by one tween; per-instance stagger
  // (by distance to the burst origin) and easing are applied in useFrame.
  const targetPosRef = useRef<Float32Array>(new Float32Array(0))
  const targetScaleRef = useRef<Float32Array>(new Float32Array(0))
  const staggerRef = useRef<Float32Array>(new Float32Array(0))
  const burstProgressRef = useRef(1) // 1 = settled (no burst in flight)
  const burstActiveRef = useRef(false)
  const burstHandleRef = useRef<ReturnType<typeof tween> | null>(null)
  // Identity of the positions buffer the last burst fired for — so color-only
  // re-renders (e.g. matchedIds changes) don't re-trigger the burst.
  const burstedPositionsRef = useRef<Float32Array | null>(null)

  // Positions + colors + drill-in burst.
  useEffect(() => {
    const mesh = ref.current
    if (!positions || !mesh) return
    const count = nodes.length

    // Build per-instance target transform buffers + colors.
    const targetPos = new Float32Array(count * 3)
    const targetScale = new Float32Array(count)
    for (let i = 0; i < count; i++) {
      const node = nodes[i]
      if (!node) continue
      const x = positions[i * 3] ?? 0
      const y = positions[i * 3 + 1] ?? 0
      const z = positions[i * 3 + 2] ?? 0
      targetPos[i * 3] = x
      targetPos[i * 3 + 1] = y
      targetPos[i * 3 + 2] = z
      const ew = effectiveWeights[i] ?? 1
      targetScale[i] = weightScale(ew, minW, maxW)

      colorScratch.copy(colorForBrainNode(node))
      if (matchedIds && !matchedIds.has(node.id)) {
        colorScratch.multiplyScalar(DIM_FACTOR)
      }
      mesh.setColorAt(i, colorScratch)
    }
    targetPosRef.current = targetPos
    targetScaleRef.current = targetScale

    const origin = burstOrigin
    const isNewLayout = burstedPositionsRef.current !== positions

    if (isNewLayout) {
      burstedPositionsRef.current = positions
      // Place instances at their targets first so the bounding sphere covers the
      // full extent (prevents frustum-culling the mesh mid-burst).
      for (let i = 0; i < count; i++) {
        dummy.position.set(targetPos[i * 3] ?? 0, targetPos[i * 3 + 1] ?? 0, targetPos[i * 3 + 2] ?? 0)
        dummy.scale.setScalar(targetScale[i] ?? 1)
        dummy.updateMatrix()
        mesh.setMatrixAt(i, dummy.matrix)
      }
      mesh.computeBoundingSphere()

      if (animateEntrance && origin && count > 0) {
        // Per-instance stagger by distance to the origin → ripple outward.
        const stagger = new Float32Array(count)
        let maxDist = 0
        for (let i = 0; i < count; i++) {
          const dx = (targetPos[i * 3] ?? 0) - origin.x
          const dy = (targetPos[i * 3 + 1] ?? 0) - origin.y
          const dz = (targetPos[i * 3 + 2] ?? 0) - origin.z
          const d = Math.hypot(dx, dy, dz)
          stagger[i] = d
          if (d > maxDist) maxDist = d
        }
        for (let i = 0; i < count; i++) {
          stagger[i] = maxDist > 0 ? ((stagger[i] ?? 0) / maxDist) * STAGGER_FRACTION : 0
        }
        staggerRef.current = stagger

        // Collapse every instance at the origin (scale ≈ 0) — the closed "node".
        for (let i = 0; i < count; i++) {
          dummy.position.copy(origin)
          dummy.scale.setScalar(BURST_MIN_SCALE)
          dummy.updateMatrix()
          mesh.setMatrixAt(i, dummy.matrix)
        }
        burstProgressRef.current = 0
        burstActiveRef.current = true
        burstHandleRef.current?.cancel()
        const handle = tween<number>(
          { from: 0, to: 1, durationMs: DURATIONS.nodeBurst },
          (v) => { burstProgressRef.current = v },
          () => {
            // Settle exactly on targets when the burst completes. Read the count
            // from the current target buffers (not the captured `count`) so a
            // late completion can never index past a resized buffer.
            const m = ref.current
            if (m) {
              const tp = targetPosRef.current
              const ts = targetScaleRef.current
              const c = ts.length
              for (let i = 0; i < c; i++) {
                dummy.position.set(tp[i * 3] ?? 0, tp[i * 3 + 1] ?? 0, tp[i * 3 + 2] ?? 0)
                dummy.scale.setScalar(ts[i] ?? 1)
                dummy.updateMatrix()
                m.setMatrixAt(i, dummy.matrix)
              }
              m.instanceMatrix.needsUpdate = true
            }
            burstActiveRef.current = false
            burstProgressRef.current = 1
            invalidate()
          },
          reducedMotion,
        )
        burstHandleRef.current = handle
        motionStore.register(handle)
      } else {
        burstActiveRef.current = false
        burstProgressRef.current = 1
      }
    } else if (!burstActiveRef.current) {
      // Color-only re-render while settled: keep instances on their targets.
      for (let i = 0; i < count; i++) {
        dummy.position.set(targetPos[i * 3] ?? 0, targetPos[i * 3 + 1] ?? 0, targetPos[i * 3 + 2] ?? 0)
        dummy.scale.setScalar(targetScale[i] ?? 1)
        dummy.updateMatrix()
        mesh.setMatrixAt(i, dummy.matrix)
      }
    }
    // Matrices were (re)written everywhere except the color-only-while-bursting
    // case, where useFrame owns them — avoid a needless buffer re-upload there.
    if (isNewLayout || !burstActiveRef.current) {
      mesh.instanceMatrix.needsUpdate = true
    }
    if (mesh.instanceColor) {
      mesh.instanceColor.needsUpdate = true
    }
    invalidate()
  // colorForBrainNode/matchedIds drive color-only updates; burstOrigin/animateEntrance drive the burst.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nodes, positions, dummy, colorScratch, effectiveWeights, minW, maxW, colorForBrainNode, matchedIds, burstOrigin, animateEntrance])

  // W2: Dispose geometry + material on unmount to prevent GPU leaks on level transitions.
  useEffect(() => {
    return () => {
      burstHandleRef.current?.cancel()
      ref.current?.geometry?.dispose()
      nodeMatRef.current?.dispose()
    }
  }, [])

  useEffect(() => {
    return () => {
      document.body.style.cursor = 'auto'
    }
  }, [])

  useEffect(() => {
    invalidate()
  }, [hovered, selectedIndex])

  // Apply overall fade opacity + drill-in burst each frame.
  // Never calls setState per frame — only mutates refs (graph3d-viz rule).
  useFrame(() => {
    const mat = nodeMatRef.current
    if (mat) mat.opacity = Math.max(0, opacityRef.current)

    if (!burstActiveRef.current) return
    const mesh = ref.current
    const origin = burstOrigin
    if (!mesh || !origin) return
    const targetPos = targetPosRef.current
    const targetScale = targetScaleRef.current
    const stagger = staggerRef.current
    const progress = burstProgressRef.current
    const count = nodes.length
    for (let i = 0; i < count; i++) {
      const delay = stagger[i] ?? 0
      const denom = 1 - delay
      const rawLocal = denom > 0 ? (progress - delay) / denom : 1
      const lt = easeOutCubic(Math.min(1, Math.max(0, rawLocal)))
      const tx = targetPos[i * 3] ?? 0
      const ty = targetPos[i * 3 + 1] ?? 0
      const tz = targetPos[i * 3 + 2] ?? 0
      dummy.position.set(
        origin.x + (tx - origin.x) * lt,
        origin.y + (ty - origin.y) * lt,
        origin.z + (tz - origin.z) * lt,
      )
      const s = (targetScale[i] ?? 1) * lt
      dummy.scale.setScalar(s < BURST_MIN_SCALE ? BURST_MIN_SCALE : s)
      dummy.updateMatrix()
      mesh.setMatrixAt(i, dummy.matrix)
    }
    mesh.instanceMatrix.needsUpdate = true
  })

  // Fresnel rim shader
  useEffect(() => {
    const mat = nodeMatRef.current
    if (!mat) return
    mat.onBeforeCompile = (shader) => {
      shader.fragmentShader = shader.fragmentShader.replace(
        '#include <opaque_fragment>',
        /* glsl */ `
          float rimEdge = 1.0 - max(dot(normalize(normal), normalize(vViewPosition)), 0.0);
          outgoingLight *= mix(1.0, 0.22, pow(rimEdge, 2.5));
          #include <opaque_fragment>
        `,
      )
    }
    mat.needsUpdate = true
  }, [])

  // Click-vs-drag discrimination: record pointer position + time on down;
  // only fire a node click if the pointer moved <5px AND elapsed <300ms.
  const pointerDownRef = useRef<{ x: number; y: number; time: number; instanceId: number | undefined } | null>(null)

  const handlePointerDown = (event: ThreeEvent<PointerEvent>) => {
    pointerDownRef.current = {
      x: event.clientX,
      y: event.clientY,
      time: performance.now(),
      instanceId: event.instanceId,
    }
  }

  const handleClick = (event: ThreeEvent<MouseEvent>) => {
    const down = pointerDownRef.current
    if (!down) return
    const dx = event.clientX - down.x
    const dy = event.clientY - down.y
    const dist = Math.hypot(dx, dy)
    const elapsed = performance.now() - down.time
    // Treat as drag if pointer moved ≥5px or held >300ms — do NOT select node.
    if (dist >= 5 || elapsed > 300) return
    // Genuine click — stop propagation so OrbitControls doesn't also react.
    event.stopPropagation()
    const id = event.instanceId
    if (id !== undefined && nodes[id] !== undefined && onNodeClick) {
      onNodeClick(nodes[id]!)
    }
  }

  const handlePointerMove = (event: ThreeEvent<PointerEvent>) => {
    event.stopPropagation()
    const id = event.instanceId
    if (id !== undefined) {
      // Only update state and fire onNodeHover when the hovered instance actually changes.
      // This prevents redundant per-move re-renders on every mouse-move pixel.
      if (hoveredRef.current !== id) {
        hoveredRef.current = id
        setHovered(id)
        if (onNodeHover) {
          const node = nodes[id]
          if (node) onNodeHover(node, event.clientX, event.clientY)
        }
      }
      if (onNodeClick) document.body.style.cursor = 'pointer'
    }
  }

  const handlePointerOut = () => {
    if (hoveredRef.current !== null) {
      hoveredRef.current = null
      setHovered(null)
      if (onNodeHover) onNodeHover(null, 0, 0)
    }
    document.body.style.cursor = 'auto'
  }

  if (nodes.length === 0) return null

  const interaction = {
    onPointerDown: handlePointerDown,
    onPointerMove: handlePointerMove,
    onPointerOut: handlePointerOut,
    ...(onNodeClick ? { onClick: handleClick } : {}),
  }

  const hoverIndex = hovered !== null && hovered !== selectedIndex ? hovered : null

  const selectedNode = selectedIndex !== null ? nodes[selectedIndex] : undefined
  const selectedHaloColor = selectedNode
    ? colorForBrainNode(selectedNode).clone().lerp(WHITE, 0.15).getStyle()
    : '#ffffff'
  const hoverNode = hoverIndex !== null ? nodes[hoverIndex] : undefined
  const hoverHaloColor = hoverNode
    ? colorForBrainNode(hoverNode).clone().lerp(WHITE, 0.6).getStyle()
    : '#e0e7ff'

  return (
    <>
      <instancedMesh ref={ref} args={[undefined, undefined, nodes.length]} frustumCulled {...interaction}>
        <sphereGeometry args={[NODE_BASE_RADIUS, 16, 16]} />
        <meshStandardMaterial ref={nodeMatRef} roughness={0.45} metalness={0.15} transparent />
      </instancedMesh>

      {selectedIndex !== null && positions && (
        <BrainNodeHalo
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
      )}
      {hoverIndex !== null && positions && (
        <BrainNodeHalo
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
      )}
    </>
  )
}

// ---------------------------------------------------------------------------
// Cross-fade controller (C4.2 / C7)
// Manages outgoing + incoming InstancedMesh instances for level transitions.
// ---------------------------------------------------------------------------

interface CrossFadeState {
  /** Key identifying the current level (to detect transitions). */
  levelKey: string
  /** Snapshot of the BrainLevel at the time of the last render. */
  level: BrainLevel | null
  positions: Float32Array | null
  animateEntrance: boolean
}

interface OutgoingMesh {
  /** Frozen BrainLevel snapshot (OLD level — nodes + colors from before the transition). */
  level: BrainLevel
  /** The positions at which the outgoing mesh was frozen. */
  positions: Float32Array
  /** Opacity driven to 0 by the fade-out tween. */
  opacityRef: React.MutableRefObject<number>
  /** Set to true once the fade is done and the mesh should be removed. */
  done: boolean
}

interface BrainLevelCrossFadeProps extends Omit<BrainLevelMeshProps, 'opacityRef' | 'animateEntrance'> {
  levelKey: string
}

/**
 * Cross-fade controller: renders outgoing + incoming BrainLevel InstancedMesh
 * instances when the level changes. (C4.2 / C7)
 *
 * When the levelKey changes:
 *   1. Current mesh becomes "outgoing" — freezes positions, tweens opacity 1→0.
 *   2. New level becomes "incoming" — tweens opacity 0→1 with staggered entrance.
 *   3. Outgoing mesh unmounts (disposes) when fade-out completes.
 */
export function BrainLevelNodes({
  level,
  positions,
  colorBy = 'project',
  onNodeClick,
  onNodeHover,
  selectedId,
  matchedIds,
  levelKey,
  burstOrigin = null,
}: BrainLevelCrossFadeProps) {
  // Track if this is the first render (no cross-fade needed)
  const isFirstRender = useRef(true)
  const prevStateRef = useRef<CrossFadeState>({ levelKey: '', level: null, positions: null, animateEntrance: false })

  // Outgoing mesh state — null when no transition in progress
  const [outgoing, setOutgoing] = useState<OutgoingMesh | null>(null)
  // Incoming mesh opacity ref — start at 0 to avoid one-frame full-opacity flash (I17)
  const incomingOpacityRef = useRef(0)

  // Hoist useReducedMotion to component top level (React hygiene: W4a)
  const reducedMotion = useReducedMotion()

  // Detect level change
  const prevLevelKey = prevStateRef.current.levelKey
  const levelChanged = prevLevelKey !== '' && prevLevelKey !== levelKey

  useEffect(() => {
    const prev = prevStateRef.current
    if (isFirstRender.current) {
      isFirstRender.current = false
      incomingOpacityRef.current = 1
      prevStateRef.current = { levelKey, level, positions, animateEntrance: false }
      return
    }

    if (prev.levelKey !== levelKey && prev.positions !== null && prev.level !== null) {
      // Start cross-fade: capture OLD level + positions as the outgoing frozen snapshot.
      // The outgoing mesh renders prev.level (OLD nodes) at prev.positions (OLD layout).
      // The incoming mesh renders the NEW level at the new positions.
      const outgoingOpacityRef = { current: 1 }

      // Fade outgoing: 1 → 0
      const fadeOutHandle = tween<number>(
        { from: 1, to: 0, durationMs: DURATIONS.nodeEnter, easing: easeOutCubic },
        (v) => { outgoingOpacityRef.current = v },
        () => {
          // Unmount outgoing mesh when fade completes (W2 disposal happens in BrainLevelNodeMesh unmount)
          setOutgoing((o) => o ? { ...o, done: true } : null)
        },
        reducedMotion,
      )
      motionStore.register(fadeOutHandle)

      // Freeze the OLD level snapshot for the outgoing mesh
      setOutgoing({
        level: prev.level,
        positions: prev.positions,
        opacityRef: outgoingOpacityRef,
        done: false,
      })

      // Fade incoming: 0 → 1
      incomingOpacityRef.current = 0
      const fadeInHandle = tween<number>(
        { from: 0, to: 1, durationMs: DURATIONS.nodeEnter, easing: easeOutCubic },
        (v) => { incomingOpacityRef.current = v },
        undefined,
        reducedMotion,
      )
      motionStore.register(fadeInHandle)
    }

    prevStateRef.current = { levelKey, level, positions, animateEntrance: levelChanged }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [levelKey, positions])

  if (level.nodes.length === 0) return null

  return (
    <>
      {/* Outgoing mesh — frozen at OLD level's nodes + positions, fading opacity 1→0.
          Uses outgoing.level (the snapshot captured at transition start — NOT the new level).
          Unmounted when done=true; BrainLevelNodeMesh disposes geometry+material on unmount (W2). */}
      {outgoing && !outgoing.done && (
        <BrainLevelNodeMesh
          level={outgoing.level}
          positions={outgoing.positions}
          colorBy={colorBy}
          opacityRef={outgoing.opacityRef}
          animateEntrance={false}
        />
      )}
      {/* Incoming mesh — NEW level at new layout positions, fading opacity 0→1.
          When burstOrigin is set (drill-in) the nodes also burst out from the
          parent's position ("the node opens"). */}
      <BrainLevelNodeMesh
        level={level}
        positions={positions}
        colorBy={colorBy}
        onNodeClick={onNodeClick}
        onNodeHover={onNodeHover}
        selectedId={selectedId}
        matchedIds={matchedIds}
        opacityRef={incomingOpacityRef}
        animateEntrance={levelChanged}
        burstOrigin={burstOrigin}
      />
    </>
  )
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
  const opacityRef = useRef(1)

  if (nodes.length === 0) return null

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
  )
}

// ---------------------------------------------------------------------------
// NodeHalo helpers
// ---------------------------------------------------------------------------

// Opacity multiplier for the x-ray (occluded) halo layer relative to the front one.
const HALO_XRAY_FACTOR = 0.15

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
  x: number
  y: number
  z: number
  radius: number
  color: string
  opacity: number
}) {
  const ref = useRef<THREE.Group>(null)
  useFrame((_, delta) => {
    const g = ref.current
    if (!g) return
    g.rotation.y += delta * 0.35
    g.rotation.x += delta * 0.12
  })
  return (
    <group ref={ref} position={[x, y, z]}>
      {/* Front (un-occluded) layer */}
      <mesh raycast={() => null} renderOrder={10}>
        <icosahedronGeometry args={[radius, 2]} />
        <meshBasicMaterial color={color} wireframe transparent opacity={opacity} depthWrite={false} />
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
  )
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
  index: number | null
  nodes: GraphNode[]
  positions: Float32Array | null
  effectiveWeights: number[]
  minW: number
  maxW: number
  color: string
  opacity: number
  scaleFactor: number
}) {
  if (index === null || !positions) return null
  const node = nodes[index]
  if (!node) return null

  const ew = effectiveWeights[index] ?? 1
  const radius = NODE_BASE_RADIUS * weightScale(ew, minW, maxW) * scaleFactor
  const x = positions[index * 3] ?? 0
  const y = positions[index * 3 + 1] ?? 0
  const z = positions[index * 3 + 2] ?? 0

  return (
    <HaloMesh x={x} y={y} z={z} radius={radius} color={color} opacity={opacity} />
  )
}

function BrainNodeHalo({
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
  index: number | null
  nodes: BrainNode[]
  positions: Float32Array | null
  effectiveWeights: number[]
  minW: number
  maxW: number
  color: string
  opacity: number
  scaleFactor: number
}) {
  if (index === null || !positions) return null
  const node = nodes[index]
  if (!node) return null

  const ew = effectiveWeights[index] ?? 1
  const radius = NODE_BASE_RADIUS * weightScale(ew, minW, maxW) * scaleFactor
  const x = positions[index * 3] ?? 0
  const y = positions[index * 3 + 1] ?? 0
  const z = positions[index * 3 + 2] ?? 0

  return (
    <HaloMesh x={x} y={y} z={z} radius={radius} color={color} opacity={opacity} />
  )
}
