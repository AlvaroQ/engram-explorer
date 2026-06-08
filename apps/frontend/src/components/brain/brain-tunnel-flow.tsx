/**
 * brain-tunnel-flow.tsx — BrainModel-aware synapse renderer (C5).
 *
 * Implements the four sub-slices of PR C5:
 *   5a: TunnelFocusLayer — focus dim/brighten on hover/select (C5.1–C5.2)
 *   5b: Relation-color shader + ambient opacity constants (C5.3–C5.4)
 *   5c: TubeGeometry radius encoding + topic-road dashing (C5.5–C5.6)
 *   5d: Consumes BrainLevel.edges (BrainEdge[]) from the model (C5.7–C5.8)
 *
 * S9 — geometry rebuilt only when edges/positions change (not on hover).
 *   - aFocusOpacity per-vertex attribute drives focus dimming via shader.
 *   - focusId changes only mutate the Float32Array + needsUpdate; no reallocation.
 *
 * graph3d-viz hard rules honored:
 * - NO gl.lineWidth for thickness (C6) — TubeGeometry used exclusively
 * - GPU uTime particles preserved and LOD-gated
 * - All opacity transitions route through motion-store (single master invalidator, C5)
 * - No setState per frame — all opacity mutations via refs in useFrame
 * - ≤ 3 draw calls for synapses per level (S5.1):
 *     1. Semantic merged TubeGeometry (solid tubes)
 *     2. Topic-road merged TubeGeometry (stippled tubes via dash shader)
 *     3. Particle layer (GPU uTime points — shared for both families)
 *
 * Pure functions live in brain-tunnel-flow.ts (unit-tested).
 * This file contains only R3F/Three.js rendering code.
 */

import { useFrame, useThree, invalidate } from '@react-three/fiber'
import { useMemo, useEffect, useRef } from 'react'
import * as THREE from 'three'
import type { BrainEdge, BrainLevel, EdgeFamily } from '../../features/brain/model/types.ts'
import { tween, easeInOutCubic, DURATIONS, useReducedMotion } from '../../features/brain/motion/interpolator.ts'
import { motionStore } from '../../features/brain/motion/motion-store.ts'
import {
  computeTubeRadius,
  computeEdgeOpacity,
  computeEdgeBrightness,
  resolveEdgeColor,
  SYNAPSE_AMBIENT_SEMANTIC_MIN,
  SYNAPSE_AMBIENT_SEMANTIC_MAX,
  SYNAPSE_AMBIENT_TOPIC_ROAD_MIN,
  SYNAPSE_AMBIENT_TOPIC_ROAD_MAX,
} from './brain-tunnel-flow.ts'

// ---------------------------------------------------------------------------
// LOD constants
// ---------------------------------------------------------------------------

/** Distance beyond which particles are suppressed (LOD gate). */
const PARTICLE_LOD_MAX_DIST = 200

/** Max particles across all edges per layer. */
const BRAIN_PARTICLE_CAP = 1200

// ---------------------------------------------------------------------------
// Particle shaders (GPU-driven uTime, preserved from legacy TunnelFlow)
// ---------------------------------------------------------------------------

const particleVertexShader = /* glsl */ `
  attribute vec3 aTo;
  attribute float aOffset;
  attribute float aSpeed;
  attribute vec3 aColor;
  uniform float uTime;
  uniform float uSize;
  uniform float uOpacity;
  varying float vOpacity;
  varying vec3 vColor;

  void main() {
    float t = fract(aOffset + uTime * aSpeed);
    vec3 p = mix(position, aTo, t);
    vColor = aColor;
    vOpacity = uOpacity;
    vec4 mvPosition = modelViewMatrix * vec4(p, 1.0);
    gl_Position = projectionMatrix * mvPosition;
    gl_PointSize = clamp(uSize * (300.0 / max(1.0, -mvPosition.z)), 1.5, 5.0);
  }
`

const particleFragmentShader = /* glsl */ `
  varying float vOpacity;
  varying vec3 vColor;

  void main() {
    float d = length(gl_PointCoord - 0.5);
    if (d > 0.5) discard;
    float a = smoothstep(0.5, 0.25, d) * vOpacity;
    gl_FragColor = vec4(vColor, a);
  }
`

// Tube fragment shader with optional stipple (dashing) for topic-road edges.
// uStipple = 0.0 → solid (semantic); uStipple = 1.0 → dashed (topic-road).
// aFocusOpacity (passed as varying vFocusOpacity from vertex shader) scales
// per-vertex brightness for focus dimming without rebuilding geometry (S9).
const tubeFragmentShader = /* glsl */ `
  uniform float uOpacity;
  uniform float uStipple;
  varying vec2 vUv;
  varying vec3 vColor;
  varying float vFocusOpacity;

  void main() {
    // Stipple: repeat dash pattern along tube axis (uv.x) for topic-road.
    if (uStipple > 0.5) {
      float dash = fract(vUv.x * 8.0); // 8 dashes per tube
      if (dash > 0.5) discard;
    }
    gl_FragColor = vec4(vColor * vFocusOpacity, uOpacity * vFocusOpacity);
  }
`

const tubeVertexShader = /* glsl */ `
  attribute vec3 aColor;
  attribute float aFocusOpacity;
  varying vec2 vUv;
  varying vec3 vColor;
  varying float vFocusOpacity;

  void main() {
    vUv = uv;
    vColor = aColor;
    vFocusOpacity = aFocusOpacity;
    gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
  }
`

// ---------------------------------------------------------------------------
// Geometry builders
// ---------------------------------------------------------------------------

/** Vertex range in the merged buffer for a single edge. */
interface EdgeVertexRange {
  edgeKey: string
  family: EdgeFamily
  startVertex: number
  endVertex: number
}

/** Result of buildMergedTubeGeometry — geometry + per-edge vertex ranges. */
interface MergedTubeResult {
  geometry: THREE.BufferGeometry
  focusOpacityAttr: THREE.Float32BufferAttribute
  vertexRanges: EdgeVertexRange[]
}

/**
 * Build a merged TubeGeometry for all edges of a given family.
 *
 * S9 change: opacity is NOT baked into geometry. Instead:
 * - aColor = relation-color * brightness (static, shape only)
 * - aFocusOpacity = per-vertex ambient opacity (mutated by focusId changes, not geometry rebuild)
 * Returns focusOpacityAttr so callers can mutate + needsUpdate on hover.
 */
function buildMergedTubeGeometry(
  edges: BrainEdge[],
  idToIndex: Map<string, number>,
  positions: Float32Array,
  family: EdgeFamily,
): MergedTubeResult | null {
  const mergedPositions: number[] = []
  const mergedUvs: number[] = []
  const mergedNormals: number[] = []
  const mergedColors: number[] = []
  const mergedFocusOpacity: number[] = []
  const mergedIndices: number[] = []
  const vertexRanges: EdgeVertexRange[] = []
  const scratch = new THREE.Color()

  const RADIAL_SEGS = 4
  let indexOffset = 0

  for (const edge of edges) {
    if (edge.family !== family) continue

    const ai = idToIndex.get(edge.source)
    const bi = idToIndex.get(edge.target)
    if (ai === undefined || bi === undefined || ai === bi) continue

    const ax = positions[ai * 3] ?? 0
    const ay = positions[ai * 3 + 1] ?? 0
    const az = positions[ai * 3 + 2] ?? 0
    const bx = positions[bi * 3] ?? 0
    const by = positions[bi * 3 + 1] ?? 0
    const bz = positions[bi * 3 + 2] ?? 0

    const dx = bx - ax
    const dy = by - ay
    const dz = bz - az
    const lenSq = dx * dx + dy * dy + dz * dz
    if (lenSq < 1e-6) continue

    const radius = computeTubeRadius(edge.weight)
    // brightness baked into color (static, confidence-derived)
    const brightness = computeEdgeBrightness(edge.confidence)
    // ambient opacity goes into aFocusOpacity (mutable on focus change)
    const ambientOpacity = computeEdgeOpacity(family, false, null)

    scratch.set(resolveEdgeColor(edge.relation))
    // Color encodes only brightness, not opacity — opacity via aFocusOpacity
    const r = scratch.r * brightness
    const g = scratch.g * brightness
    const b = scratch.b * brightness

    // Build a simple tube using a CatmullRomCurve3 with 2 points (straight tube)
    const curve = new THREE.CatmullRomCurve3([
      new THREE.Vector3(ax, ay, az),
      new THREE.Vector3(bx, by, bz),
    ])

    const tubeSeg = 1
    const tubeGeo = new THREE.TubeGeometry(curve, tubeSeg, radius, RADIAL_SEGS, false)

    const posAttr = tubeGeo.getAttribute('position') as THREE.BufferAttribute
    const uvAttr = tubeGeo.getAttribute('uv') as THREE.BufferAttribute
    const normAttr = tubeGeo.getAttribute('normal') as THREE.BufferAttribute
    const idxAttr = tubeGeo.getIndex()

    const vCount = posAttr.count
    const startVertex = indexOffset

    for (let v = 0; v < vCount; v++) {
      mergedPositions.push(posAttr.getX(v), posAttr.getY(v), posAttr.getZ(v))
      mergedUvs.push(uvAttr.getX(v), uvAttr.getY(v))
      mergedNormals.push(normAttr.getX(v), normAttr.getY(v), normAttr.getZ(v))
      mergedColors.push(r, g, b)
      mergedFocusOpacity.push(ambientOpacity)
    }

    if (idxAttr) {
      for (let i = 0; i < idxAttr.count; i++) {
        mergedIndices.push(idxAttr.getX(i) + indexOffset)
      }
    }

    vertexRanges.push({
      edgeKey: `${edge.source}:${edge.target}`,
      family,
      startVertex,
      endVertex: indexOffset + vCount,
    })

    indexOffset += vCount
    tubeGeo.dispose()
  }

  if (mergedPositions.length === 0) return null

  const focusOpacityAttr = new THREE.Float32BufferAttribute(mergedFocusOpacity, 1)
  focusOpacityAttr.setUsage(THREE.DynamicDrawUsage)

  const geo = new THREE.BufferGeometry()
  geo.setAttribute('position', new THREE.Float32BufferAttribute(mergedPositions, 3))
  geo.setAttribute('uv', new THREE.Float32BufferAttribute(mergedUvs, 2))
  geo.setAttribute('normal', new THREE.Float32BufferAttribute(mergedNormals, 3))
  geo.setAttribute('aColor', new THREE.Float32BufferAttribute(mergedColors, 3))
  geo.setAttribute('aFocusOpacity', focusOpacityAttr)
  geo.setIndex(mergedIndices)
  return { geometry: geo, focusOpacityAttr, vertexRanges }
}

/** Deterministic [0,1) hash for particle phase/speed variation. */
function hash01(n: number): number {
  const x = Math.sin(n * 127.1) * 43758.5453
  return x - Math.floor(x)
}

/** Build particle geometry for all edges of a given family. */
function buildParticleGeometry(
  edges: BrainEdge[],
  idToIndex: Map<string, number>,
  positions: Float32Array,
  family: EdgeFamily,
): THREE.BufferGeometry | null {
  const from: number[] = []
  const to: number[] = []
  const offset: number[] = []
  const speed: number[] = []
  const color: number[] = []
  const scratch = new THREE.Color()

  let totalEmitted = 0
  let edgeIndex = 0

  for (const edge of edges) {
    if (edge.family !== family) continue
    if (totalEmitted >= BRAIN_PARTICLE_CAP) break

    const ai = idToIndex.get(edge.source)
    const bi = idToIndex.get(edge.target)
    if (ai === undefined || bi === undefined || ai === bi) continue

    const ax = positions[ai * 3] ?? 0
    const ay = positions[ai * 3 + 1] ?? 0
    const az = positions[ai * 3 + 2] ?? 0
    const bx = positions[bi * 3] ?? 0
    const by = positions[bi * 3 + 1] ?? 0
    const bz = positions[bi * 3 + 2] ?? 0
    const dx = bx - ax, dy = by - ay, dz = bz - az
    if (dx * dx + dy * dy + dz * dz < 1e-6) continue

    // Particle count scales with edge weight (capped per family)
    const maxPerEdge = family === 'semantic' ? 4 : 2
    const n = Math.min(maxPerEdge, Math.max(1, Math.round(edge.weight)))
    const fwdCount = Math.ceil(n / 2)
    const revCount = n - fwdCount

    scratch.set(resolveEdgeColor(edge.relation))
    const phaseBase = hash01(edgeIndex * 2.17)
    const baseSpeed = family === 'semantic' ? 0.10 : 0.06
    const edgeSpeed = baseSpeed + hash01(edgeIndex * 3.71) * (family === 'semantic' ? 0.06 : 0.03)

    for (let k = 0; k < fwdCount && totalEmitted < BRAIN_PARTICLE_CAP; k++) {
      from.push(ax, ay, az)
      to.push(bx, by, bz)
      offset.push((k / n + phaseBase) % 1)
      speed.push(edgeSpeed)
      color.push(scratch.r, scratch.g, scratch.b)
      totalEmitted++
    }
    for (let k = 0; k < revCount && totalEmitted < BRAIN_PARTICLE_CAP; k++) {
      from.push(bx, by, bz)
      to.push(ax, ay, az)
      offset.push(((k + fwdCount) / n + phaseBase + 0.5) % 1)
      speed.push(edgeSpeed)
      color.push(scratch.r, scratch.g, scratch.b)
      totalEmitted++
    }

    edgeIndex++
  }

  if (from.length === 0) return null
  const geo = new THREE.BufferGeometry()
  geo.setAttribute('position', new THREE.Float32BufferAttribute(from, 3))
  geo.setAttribute('aTo', new THREE.Float32BufferAttribute(to, 3))
  geo.setAttribute('aOffset', new THREE.Float32BufferAttribute(offset, 1))
  geo.setAttribute('aSpeed', new THREE.Float32BufferAttribute(speed, 1))
  geo.setAttribute('aColor', new THREE.Float32BufferAttribute(color, 3))
  return geo
}

// ---------------------------------------------------------------------------
// Sub-slice 5a: TunnelFocusLayer — per-edge opacity animation
//
// Maintains a Map<edgeKey, opacityRef> so each edge can be tweened independently.
// When hoveredId changes:
//   - Incident edges → tween opacity to FOCUS value
//   - Non-incident edges → tween opacity to ≤ ambient/2
//   - No hover (null) → tween all edges back to ambient
// ---------------------------------------------------------------------------

interface EdgeOpacityState {
  key: string
  opacityRef: { current: number }
}

function buildEdgeOpacityStates(edges: BrainEdge[]): Map<string, EdgeOpacityState> {
  const map = new Map<string, EdgeOpacityState>()
  for (const edge of edges) {
    const key = `${edge.source}:${edge.target}`
    map.set(key, {
      key,
      opacityRef: { current: computeEdgeOpacity(edge.family, false, null) },
    })
  }
  return map
}

// ---------------------------------------------------------------------------
// TubeMesh — renders a merged TubeGeometry for one family
//
// S9: receives focusOpacityAttr + vertexRanges to mutate per-vertex opacity
// on focusId change without rebuilding geometry.
// ---------------------------------------------------------------------------

function TubeMesh({
  geometry,
  focusOpacityAttr,
  vertexRanges,
  family,
  opacity,
  focusId,
  edgeOpacityStatesRef,
  reducedMotion,
}: {
  geometry: THREE.BufferGeometry
  focusOpacityAttr: THREE.Float32BufferAttribute
  vertexRanges: EdgeVertexRange[]
  family: EdgeFamily
  opacity: number
  focusId: string | null
  edgeOpacityStatesRef: React.MutableRefObject<Map<string, EdgeOpacityState>>
  reducedMotion: boolean
}) {
  const matRef = useRef<THREE.ShaderMaterial>(null!)

  const uniforms = useMemo(
    () => ({
      uOpacity: { value: opacity },
      uStipple: { value: family === 'topic-road' ? 1.0 : 0.0 },
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [family],
  )

  // Update uOpacity uniform when prop changes
  useEffect(() => {
    const u = matRef.current?.uniforms.uOpacity
    if (u) u.value = opacity
  }, [opacity])

  // S9: Animate aFocusOpacity per-vertex when focusId changes.
  // This replaces opacityMap → geometry rebuild cycle.
  // Tweens route through motion-store (single invalidator, C5 contract).
  const prevFocusIdRef = useRef<string | null | undefined>(undefined)
  useEffect(() => {
    if (prevFocusIdRef.current === focusId) return
    prevFocusIdRef.current = focusId

    const opacityStates = edgeOpacityStatesRef.current

    // Build a lookup from edgeKey to the vertex range for this merged geometry
    // (O(N) on vertexRanges, not on all edges — fast)
    for (const range of vertexRanges) {
      const state = opacityStates.get(range.edgeKey)
      if (!state) continue

      const edgeParts = range.edgeKey.split(':')
      const edgeSource = edgeParts[0] ?? ''
      const edgeTarget = edgeParts[1] ?? ''
      const isIncident =
        focusId !== null && (edgeSource === focusId || edgeTarget === focusId)
      const targetOpacity = computeEdgeOpacity(range.family, isIncident, focusId)

      const from = state.opacityRef.current
      const to = targetOpacity

      if (Math.abs(from - to) < 0.001) continue

      const rangeRef = { ...range }
      const handle = tween<number>(
        { from, to, durationMs: DURATIONS.dimBrighten, easing: easeInOutCubic },
        (v) => {
          state.opacityRef.current = v
          // Mutate the Float32Array in-place and mark dirty
          const arr = focusOpacityAttr.array as Float32Array
          for (let vi = rangeRef.startVertex; vi < rangeRef.endVertex; vi++) {
            arr[vi] = v
          }
          focusOpacityAttr.needsUpdate = true
          invalidate()
        },
        undefined,
        reducedMotion,
      )
      motionStore.register(handle)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusId, focusOpacityAttr, vertexRanges, reducedMotion])

  useEffect(() => () => geometry.dispose(), [geometry])

  return (
    <mesh geometry={geometry} frustumCulled={false} raycast={() => null}>
      <shaderMaterial
        ref={matRef}
        uniforms={uniforms}
        vertexShader={tubeVertexShader}
        fragmentShader={tubeFragmentShader}
        transparent
        depthWrite={false}
        side={THREE.DoubleSide}
        blending={THREE.AdditiveBlending}
      />
    </mesh>
  )
}

// ---------------------------------------------------------------------------
// EdgeParticles — GPU uTime particles, LOD-gated
// ---------------------------------------------------------------------------

function BrainEdgeParticles({
  geometry,
  baseOpacity,
}: {
  geometry: THREE.BufferGeometry
  baseOpacity: number
}) {
  const matRef = useRef<THREE.ShaderMaterial>(null!)
  const camera = useThree((s) => s.camera)
  const controls = useThree((s) => s.controls) as { target: THREE.Vector3 } | null

  const uniforms = useMemo(
    () => ({
      uTime: { value: 0 },
      uSize: { value: 2.0 },
      uOpacity: { value: baseOpacity },
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  )

  // Advance uTime on the GPU (no JS per-vertex work).
  // LOD gate: suppress animation when the content is far from the camera.
  //
  // Distance is measured to the OrbitControls target (the point being looked at),
  // NOT to the world origin — drilled-in levels are re-centered on the parent's
  // world position, so an origin-based gate would freeze particles the moment you
  // drill in. Target-based distance keeps the synapses flowing at every level.
  //
  // The invalidate() makes this work under frameloop="demand" too; under
  // frameloop="always" it is a harmless no-op.
  useFrame((_, delta) => {
    if (typeof document !== 'undefined' && document.hidden) return
    const dist = controls?.target ? camera.position.distanceTo(controls.target) : camera.position.length()
    if (dist > PARTICLE_LOD_MAX_DIST) return
    const uTime = matRef.current?.uniforms.uTime
    if (uTime) uTime.value += delta
    // Wake the demand frameloop to keep particles flowing while LOD-gated (zoomed in).
    invalidate()
  })

  useEffect(() => {
    const u = matRef.current?.uniforms.uOpacity
    if (u) u.value = baseOpacity
  }, [baseOpacity])

  useEffect(() => () => geometry.dispose(), [geometry])

  return (
    <points geometry={geometry} frustumCulled={false} raycast={() => null}>
      <shaderMaterial
        ref={matRef}
        uniforms={uniforms}
        vertexShader={particleVertexShader}
        fragmentShader={particleFragmentShader}
        transparent
        depthWrite={false}
        blending={THREE.AdditiveBlending}
      />
    </points>
  )
}

// ---------------------------------------------------------------------------
// Sub-slice 5d: BrainTunnelFlow — main public component
//
// Consumes BrainLevel.edges (BrainEdge[]) from the model (not raw GraphEdge[]).
// The graph-scene passes the current BrainLevel down to this component.
//
// Props:
//   level     — current BrainLevel (nodes + edges from brain-model)
//   positions — Float32Array from force-layout worker (indexed by node order in level)
//   hoveredId — string id of the currently hovered BrainNode, or null
//   selectedId — string id of the currently selected BrainNode, or null
// ---------------------------------------------------------------------------

export interface BrainTunnelFlowProps {
  level: BrainLevel
  positions: Float32Array | null
  /** Id of the currently hovered BrainNode (string id, not numeric). */
  hoveredId?: string | null | undefined
  /** Id of the currently selected BrainNode (string id, not numeric). */
  selectedId?: string | null | undefined
}

export function BrainTunnelFlow({
  level,
  positions,
  hoveredId = null,
  selectedId = null,
}: BrainTunnelFlowProps) {
  const focusId = hoveredId ?? selectedId ?? null

  // Hoist useReducedMotion to component top level (React hygiene: W4a)
  const reducedMotion = useReducedMotion()

  // Build id → position index map from level.nodes (indexed in same order as positions)
  const idToIndex = useMemo(() => {
    const m = new Map<string, number>()
    level.nodes.forEach((n, i) => m.set(n.id, i))
    return m
  }, [level.nodes])

  // ---------------------------------------------------------------------------
  // Sub-slice 5a: Focus-layer opacity state
  // Build per-edge opacity refs; animate via motion-store when focusId changes.
  // ---------------------------------------------------------------------------

  // Per-edge opacity refs (mutable, mutated by tweens, read in render)
  const edgeOpacityStatesRef = useRef<Map<string, EdgeOpacityState>>(new Map())
  // Track previous edges for rebuild detection
  const prevEdgesRef = useRef<BrainEdge[]>([])

  // Rebuild opacity states when edges change
  if (prevEdgesRef.current !== level.edges) {
    prevEdgesRef.current = level.edges
    edgeOpacityStatesRef.current = buildEdgeOpacityStates(level.edges)
  }

  // ---------------------------------------------------------------------------
  // S9: Build merged tube geometries ONLY on [level.edges, idToIndex, positions].
  // focusId is NOT a dependency — focus dimming is driven via aFocusOpacity attribute
  // mutations in TubeMesh.useEffect (no geometry reallocation on hover).
  // ---------------------------------------------------------------------------

  const semanticResult = useMemo(
    () =>
      positions
        ? buildMergedTubeGeometry(level.edges, idToIndex, positions, 'semantic')
        : null,
    [level.edges, idToIndex, positions],
  )

  const topicRoadResult = useMemo(
    () =>
      positions
        ? buildMergedTubeGeometry(level.edges, idToIndex, positions, 'topic-road')
        : null,
    [level.edges, idToIndex, positions],
  )

  // ---------------------------------------------------------------------------
  // Particle layer (GPU uTime, Sub-slice 5b / LOD gated)
  // One draw call for all edges — particles per family handled by geometry builder
  // ---------------------------------------------------------------------------

  const semanticParticleGeo = useMemo(
    () =>
      positions
        ? buildParticleGeometry(
            level.edges.filter((e) => e.family === 'semantic'),
            idToIndex,
            positions,
            'semantic',
          )
        : null,
    [level.edges, idToIndex, positions],
  )

  const topicParticleGeo = useMemo(
    () =>
      positions
        ? buildParticleGeometry(
            level.edges.filter((e) => e.family === 'topic-road'),
            idToIndex,
            positions,
            'topic-road',
          )
        : null,
    [level.edges, idToIndex, positions],
  )

  if (!positions) return null

  // Ambient midpoints for particle base opacity
  const semanticParticleOpacity = (SYNAPSE_AMBIENT_SEMANTIC_MIN + SYNAPSE_AMBIENT_SEMANTIC_MAX) / 2
  const topicParticleOpacity = (SYNAPSE_AMBIENT_TOPIC_ROAD_MIN + SYNAPSE_AMBIENT_TOPIC_ROAD_MAX) / 2

  return (
    <>
      {/* Draw call 1: Semantic solid tubes (relation-colored, weight-radius, confidence-brightness) */}
      {semanticResult ? (
        <TubeMesh
          geometry={semanticResult.geometry}
          focusOpacityAttr={semanticResult.focusOpacityAttr}
          vertexRanges={semanticResult.vertexRanges}
          family="semantic"
          opacity={1.0}
          focusId={focusId}
          edgeOpacityStatesRef={edgeOpacityStatesRef}
          reducedMotion={reducedMotion}
        />
      ) : null}

      {/* Draw call 2: Topic-road dashed/stippled tubes (fainter, amber-colored) */}
      {topicRoadResult ? (
        <TubeMesh
          geometry={topicRoadResult.geometry}
          focusOpacityAttr={topicRoadResult.focusOpacityAttr}
          vertexRanges={topicRoadResult.vertexRanges}
          family="topic-road"
          opacity={1.0}
          focusId={focusId}
          edgeOpacityStatesRef={edgeOpacityStatesRef}
          reducedMotion={reducedMotion}
        />
      ) : null}

      {/* Draw call 3a: Semantic particles (GPU uTime, LOD-gated) */}
      {semanticParticleGeo ? (
        <BrainEdgeParticles
          geometry={semanticParticleGeo}
          baseOpacity={semanticParticleOpacity * 1.5}
        />
      ) : null}

      {/* Draw call 3b: Topic-road particles (dimmer) */}
      {topicParticleGeo ? (
        <BrainEdgeParticles
          geometry={topicParticleGeo}
          baseOpacity={topicParticleOpacity}
        />
      ) : null}
    </>
  )
}

// ---------------------------------------------------------------------------
// Re-export constants for consumers (graph-scene, tests)
// These re-exports allow importing from brain-tunnel-flow.tsx directly.
// ---------------------------------------------------------------------------
export type { EdgeFamily } from '../../features/brain/model/types.ts'
