// Graph edges rendered as two families of layers:
//  1. BaseLines (semantic)   — faint static segments for explicit memory_relations.
//  2. BaseLines (topic)      — even fainter segments for synthetic similarity roads
//     (intra-project TF-IDF/k-NN over content; see deriveSimilarityEdges).
//  3. EdgeParticles (semantic) — bright sprites travelling BOTH directions along
//     semantic edges, weight-driven count and speed. One draw call.
//  4. EdgeParticles (topic)  — dim sprites, same bidirectional trick, for the
//     similarity roads. One draw call.
//
// ALL animation is GPU-driven: positions are interpolated in the vertex shader by a
// uTime uniform. JS never moves a vertex per frame. Two draw calls total for particles
// (one per layer), two for base lines = four draw calls total regardless of edge count.
import { useFrame } from '@react-three/fiber'
import { useMemo, useEffect, useRef } from 'react'
import * as THREE from 'three'
import type { GraphEdge, GraphEdgeRelation, GraphNode } from '../../lib/api.ts'
import {
  tokenize,
  TOPIC_TOKEN_WEIGHT,
  TITLE_TOKEN_WEIGHT,
  NON_TOPICAL_TYPES,
} from '../../lib/tfidf.ts'

// ---------------------------------------------------------------------------
// Colors
// ---------------------------------------------------------------------------

// Bright per-relation colors for explicit semantic edges.
const EDGE_COLORS: Record<GraphEdgeRelation, string> = {
  related: '#60a5fa',    // blue-400
  scoped: '#fbbf24',     // amber-400
  compatible: '#34d399', // emerald-400
}

// Dim neutral color for implicit shared-topic proximity roads.
const TOPIC_ROAD_COLOR = '#6b80a0' // desaturated slate-blue

// ---------------------------------------------------------------------------
// Particle budget
// ---------------------------------------------------------------------------

// Semantic edges: particle count per road scales with endpoint weight.
const SEMANTIC_MIN_PARTICLES = 1
const SEMANTIC_MAX_PARTICLES = 6
// Topic roads: always dim and sparse — weight influence is softer.
const TOPIC_MIN_PARTICLES = 1
const TOPIC_MAX_PARTICLES = 3
// Hard cap on total particles across all roads (performance ceiling at 800 nodes).
const TOTAL_PARTICLE_CAP = 4000

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// Deterministic [0,1) pseudo-random so rebuilds don't reshuffle phases/speeds.
function hash01(n: number): number {
  const x = Math.sin(n * 127.1) * 43758.5453
  return x - Math.floor(x)
}

// Weight → particle count for semantic edges.
// Uses log2(1 + avgWeight) so heavy nodes get more light but the growth tapers off.
function semanticParticleCount(weightA: number, weightB: number): number {
  const avg = (weightA + weightB) / 2
  return Math.max(SEMANTIC_MIN_PARTICLES, Math.min(SEMANTIC_MAX_PARTICLES, Math.round(1 + Math.log2(1 + avg))))
}

// Weight → particle count for topic roads (softer scaling, lower ceiling).
function topicParticleCount(weightA: number, weightB: number): number {
  const avg = (weightA + weightB) / 2
  return Math.max(TOPIC_MIN_PARTICLES, Math.min(TOPIC_MAX_PARTICLES, Math.round(1 + Math.log2(1 + avg * 0.5))))
}

interface DerivedEdge {
  source: number // node id
  target: number
  weightSource: number
  weightTarget: number
}

// ---------------------------------------------------------------------------
// Synthetic similarity edges (intra-project, content-aware)
// ---------------------------------------------------------------------------
//
// WHY synthetic: the dataset has almost no real edges. Judged memory_relations are
// sparse (most projects have zero) and exact-topicKey groups collapse to a single
// node (re-saved revisions share a normalized_hash), so connecting on exact topicKey
// yields ~nothing. To give each project legible internal structure we SYNTHESIZE
// roads from content affinity.
//
// HOW it stays meaningful: a TF-IDF cosine over each node's topicKey + title,
// computed PER PROJECT. Weighting by inverse document frequency means shared
// DISTINCTIVE tokens (e.g. "ticker-historical") link siblings while generic ones
// ("arch", "fix") don't — so it's not "everything under arch joined". Each node
// keeps only its top-K most similar neighbours above a threshold → bounded degree,
// no hairballs.
//
// SIGNAL note: observations.embedding is empty in the local DB today, so we use the
// lexical signals present in the graph payload (topicKey, title). When embeddings
// get populated this should move to a backend cosine over the real vectors; the
// render layer below does not change.

// SIM_NEIGHBOURS / SIM_THRESHOLD are local to the edge-derivation algorithm;
// they are NOT part of the shared tfidf.ts surface because they control the
// graph structure rather than the lexical representation.
const SIM_NEIGHBOURS = 3 // top-K most-similar neighbours kept per node (bounds degree)
const SIM_THRESHOLD = 0.18 // min cosine to draw a road (drops weak, coincidental links)

// Build synthetic roads by TF-IDF cosine similarity within each project.
// Each node links to its SIM_NEIGHBOURS strongest matches above SIM_THRESHOLD; the
// union of those per-node picks (deduped, undirected) is the final road set.
function deriveSimilarityEdges(nodes: GraphNode[]): DerivedEdge[] {
  // Similarity is computed WITHIN a project only, so synthetic roads never bridge
  // galaxies — matching the intra-project intent.
  const byProject = new Map<string, GraphNode[]>()
  for (const n of nodes) {
    if (n.type != null && NON_TOPICAL_TYPES.has(n.type)) continue
    const p = n.project ?? '∅'
    const bucket = byProject.get(p)
    if (bucket) bucket.push(n)
    else byProject.set(p, [n])
  }

  const edges: DerivedEdge[] = []

  for (const [, members] of byProject) {
    const N = members.length
    if (N < 2) continue

    // 1. Weighted term frequency per node + document frequency per token.
    const tfMaps: Map<string, number>[] = []
    const df = new Map<string, number>()
    for (const m of members) {
      const tf = new Map<string, number>()
      for (const tok of tokenize(m.topicKey)) tf.set(tok, (tf.get(tok) ?? 0) + TOPIC_TOKEN_WEIGHT)
      for (const tok of tokenize(m.label)) tf.set(tok, (tf.get(tok) ?? 0) + TITLE_TOKEN_WEIGHT)
      tfMaps.push(tf)
      for (const tok of tf.keys()) df.set(tok, (df.get(tok) ?? 0) + 1)
    }

    // 2. L2-normalized TF-IDF vectors + an inverted index token→node indices so we
    //    only score pairs that actually share a token (sparse, not full O(n²) dot).
    const vecs: Map<string, number>[] = []
    const norms: number[] = []
    const inverted = new Map<string, number[]>()
    for (let i = 0; i < N; i++) {
      const v = new Map<string, number>()
      let sumSq = 0
      for (const [tok, freq] of tfMaps[i]!) {
        const idf = Math.log(N / (df.get(tok) ?? 1))
        if (idf <= 0) continue // token in every node → zero discriminative value
        const w = freq * idf
        v.set(tok, w)
        sumSq += w * w
        const inv = inverted.get(tok)
        if (inv) inv.push(i)
        else inverted.set(tok, [i])
      }
      vecs.push(v)
      norms.push(Math.sqrt(sumSq) || 1)
    }

    // 3. For each node, accumulate dot products against every token-sharing peer,
    //    keep its top-K above threshold, and add the (deduped, undirected) road.
    const seen = new Set<string>()
    for (let i = 0; i < N; i++) {
      const dots = new Map<number, number>()
      for (const [tok, wi] of vecs[i]!) {
        const inv = inverted.get(tok)
        if (!inv) continue
        for (const j of inv) {
          if (j === i) continue
          dots.set(j, (dots.get(j) ?? 0) + wi * (vecs[j]!.get(tok) ?? 0))
        }
      }

      const scored: { j: number; cos: number }[] = []
      for (const [j, dot] of dots) {
        const cos = dot / (norms[i]! * norms[j]!)
        if (cos >= SIM_THRESHOLD) scored.push({ j, cos })
      }
      scored.sort((a, b) => b.cos - a.cos)

      for (let k = 0; k < Math.min(SIM_NEIGHBOURS, scored.length); k++) {
        const j = scored[k]!.j
        const a = i < j ? i : j
        const b = i < j ? j : i
        const key = `${a}:${b}`
        if (seen.has(key)) continue
        seen.add(key)
        edges.push({
          source: members[a]!.id,
          target: members[b]!.id,
          weightSource: members[a]!.weight,
          weightTarget: members[b]!.weight,
        })
      }
    }
  }

  return edges
}

// ---------------------------------------------------------------------------
// Geometry builders
// ---------------------------------------------------------------------------

interface Props {
  nodes: GraphNode[]
  edges: GraphEdge[]
  positions: Float32Array | null
  /**
   * Ids of the isolated project's nodes. When set, the base edge layers fade and
   * a brighter "focused" layer — edges with BOTH endpoints inside the isolated
   * project — is drawn on top. This is what makes the intra-galaxy roads legible:
   * with 32 projects normalized into one fixed cube, a project's internal edges are
   * short + faint and vanish in the global view; isolating one and zooming in must
   * resurface them rather than dim them. null = global view (base opacities).
   */
  matchedIds?: Set<number> | null | undefined
}

// Faint always-on segments for explicit semantic edges — one draw call.
function buildBaseLineGeometry(
  edges: GraphEdge[],
  idToIndex: Map<number, number>,
  positions: Float32Array,
): THREE.BufferGeometry | null {
  const pts: number[] = []
  const cols: number[] = []
  const scratch = new THREE.Color()

  for (const edge of edges) {
    const ai = idToIndex.get(edge.source)
    const bi = idToIndex.get(edge.target)
    if (ai === undefined || bi === undefined || ai === bi) continue

    pts.push(
      positions[ai * 3] ?? 0, positions[ai * 3 + 1] ?? 0, positions[ai * 3 + 2] ?? 0,
      positions[bi * 3] ?? 0, positions[bi * 3 + 1] ?? 0, positions[bi * 3 + 2] ?? 0,
    )
    // Per-vertex color (both endpoints share the relation color). This is the
    // legitimate use of vertexColors: the geometry actually has a `color`
    // attribute — unlike the node spheres, which use instanceColor instead.
    scratch.set(EDGE_COLORS[edge.relation])
    cols.push(scratch.r, scratch.g, scratch.b, scratch.r, scratch.g, scratch.b)
  }

  if (pts.length === 0) return null
  const geo = new THREE.BufferGeometry()
  geo.setAttribute('position', new THREE.Float32BufferAttribute(pts, 3))
  geo.setAttribute('color', new THREE.Float32BufferAttribute(cols, 3))
  return geo
}

// Even fainter always-on segments for derived shared-topicKey roads — one draw call.
function buildTopicLineGeometry(
  topicEdges: DerivedEdge[],
  idToIndex: Map<number, number>,
  positions: Float32Array,
): THREE.BufferGeometry | null {
  const pts: number[] = []
  const cols: number[] = []
  const scratch = new THREE.Color(TOPIC_ROAD_COLOR)

  for (const edge of topicEdges) {
    const ai = idToIndex.get(edge.source)
    const bi = idToIndex.get(edge.target)
    if (ai === undefined || bi === undefined || ai === bi) continue

    pts.push(
      positions[ai * 3] ?? 0, positions[ai * 3 + 1] ?? 0, positions[ai * 3 + 2] ?? 0,
      positions[bi * 3] ?? 0, positions[bi * 3 + 1] ?? 0, positions[bi * 3 + 2] ?? 0,
    )
    cols.push(scratch.r, scratch.g, scratch.b, scratch.r, scratch.g, scratch.b)
  }

  if (pts.length === 0) return null
  const geo = new THREE.BufferGeometry()
  geo.setAttribute('position', new THREE.Float32BufferAttribute(pts, 3))
  geo.setAttribute('color', new THREE.Float32BufferAttribute(cols, 3))
  return geo
}

// Build the travelling-particle cloud for ONE layer of edges.
//
// Bidirectional trick: for each road we spawn N forward sprites AND N reverse
// sprites. Forward sprites have `aDir = 0.0` (t runs from `position` to `aTo`).
// Reverse sprites have `aDir = 1.0` (from/to are swapped, same math, opposite
// direction). This keeps the motion purely GPU-driven — JS never swaps anything
// per frame; the attribute is static data baked at geometry build time.
//
// A per-edge random split (using hash01) keeps the forward/reverse counts roughly
// equal but not perfectly synchronized, which reads as organic two-way traffic.
//
// Returns null when there are no valid edges (so the caller can skip rendering).
function buildParticleGeometry(
  edgeList: { source: number; target: number; weightSource: number; weightTarget: number; colorHex: string }[],
  idToIndex: Map<number, number>,
  positions: Float32Array,
  countFn: (wa: number, wb: number) => number,
  baseSpeed: number,
  speedVariance: number,
  totalCap: number,
): THREE.BufferGeometry | null {
  const from: number[] = []
  const to: number[] = []
  const offset: number[] = []
  const speed: number[] = []
  const color: number[] = []
  // aDir: 0 = forward (position→aTo), 1 = reverse (position→aTo with start at far end).
  // The shader handles both cases identically — swapping from/to at build time means
  // the attribute is purely a build-time concern, not a runtime shader branch.
  // (We bake the swap into the position/aTo attributes directly, so aDir is
  //  not even needed in the shader. See note below.)
  //
  // Implementation note: instead of a shader branch, we build reverse sprites by
  // simply swapping which endpoint goes into `position` (aFrom) vs `aTo`. The
  // shader's `mix(position, aTo, t)` then runs the sprite from the "target" side
  // back to the "source" side. Zero shader changes required.

  const scratch = new THREE.Color()
  let totalEmitted = 0
  let edgeIndex = 0

  for (const edge of edgeList) {
    if (totalEmitted >= totalCap) break

    const ai = idToIndex.get(edge.source)
    const bi = idToIndex.get(edge.target)
    if (ai === undefined || bi === undefined || ai === bi) continue

    const ax = positions[ai * 3] ?? 0, ay = positions[ai * 3 + 1] ?? 0, az = positions[ai * 3 + 2] ?? 0
    const bx = positions[bi * 3] ?? 0, by = positions[bi * 3 + 1] ?? 0, bz = positions[bi * 3 + 2] ?? 0
    const dx = bx - ax, dy = by - ay, dz = bz - az
    if (dx * dx + dy * dy + dz * dz < 1e-6) continue

    scratch.set(edge.colorHex)
    const n = Math.min(countFn(edge.weightSource, edge.weightTarget), totalCap - totalEmitted)
    // Forward count = half (rounded up), reverse = remainder.
    const fwdCount = Math.ceil(n / 2)
    const revCount = n - fwdCount
    const phaseBase = hash01(edgeIndex * 2.17)
    const edgeSpeed = baseSpeed + hash01(edgeIndex * 3.71) * speedVariance
    // Scale speed slightly with weight: heavier nodes → slightly faster light.
    const avgWeight = (edge.weightSource + edge.weightTarget) / 2
    const weightSpeedBoost = Math.min(0.04, avgWeight * 0.002) // max +0.04 boost
    const finalSpeed = edgeSpeed + weightSpeedBoost

    // Emit forward sprites (source → target).
    for (let k = 0; k < fwdCount; k++) {
      from.push(ax, ay, az)
      to.push(bx, by, bz)
      // Evenly spaced along edge + per-edge phase so the graph isn't synchronized.
      offset.push((k / n + phaseBase) % 1)
      speed.push(finalSpeed)
      color.push(scratch.r, scratch.g, scratch.b)
    }

    // Emit reverse sprites (target → source): swap position and aTo.
    for (let k = 0; k < revCount; k++) {
      from.push(bx, by, bz)  // sprite starts at target
      to.push(ax, ay, az)     // and travels to source
      // Offset reverse sprites by 0.5 so they're staggered against the forward ones.
      offset.push(((k + fwdCount) / n + phaseBase + 0.5) % 1)
      speed.push(finalSpeed)
      color.push(scratch.r, scratch.g, scratch.b)
    }

    totalEmitted += n
    edgeIndex += 1
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
// Shaders (shared between both particle layers, parameterised by uniforms)
// ---------------------------------------------------------------------------

const particleVertexShader = /* glsl */ `
  attribute vec3 aTo;
  attribute float aOffset;
  attribute float aSpeed;
  attribute vec3 aColor;
  uniform float uTime;
  uniform float uSize;
  varying vec3 vColor;

  void main() {
    // Loop 0→1 along the edge; fract makes the sprite jump back to the start.
    // Bidirectional motion is handled at geometry build time: reverse sprites have
    // their position/aTo endpoints swapped, so this shader is direction-agnostic.
    float t = fract(aOffset + uTime * aSpeed);
    vec3 p = mix(position, aTo, t);
    vColor = aColor;
    vec4 mvPosition = modelViewMatrix * vec4(p, 1.0);
    gl_Position = projectionMatrix * mvPosition;
    // Perspective size attenuation, clamped so near sprites don't balloon into
    // giant blobs and far ones never vanish — keeps them reading as small "cars".
    gl_PointSize = clamp(uSize * (300.0 / max(1.0, -mvPosition.z)), 1.5, 6.0);
  }
`

const particleFragmentShader = /* glsl */ `
  uniform float uOpacity;
  varying vec3 vColor;

  void main() {
    // Turn the square point into a soft circle.
    float d = length(gl_PointCoord - 0.5);
    if (d > 0.5) discard;
    float a = smoothstep(0.5, 0.25, d) * uOpacity;
    gl_FragColor = vec4(vColor, a);
  }
`

// ---------------------------------------------------------------------------
// React components
// ---------------------------------------------------------------------------

// Animated travelling sprites — one <points> draw call, advanced on the GPU.
function EdgeParticles({
  geometry,
  dimmed,
  baseOpacity,
  pointSize,
  renderOrder = 0,
}: {
  geometry: THREE.BufferGeometry
  dimmed: boolean
  /** Full opacity when not dimmed. Topic roads pass a lower value (0.5) to stay dim. */
  baseOpacity: number
  /** Base point size. Topic roads use a smaller value (1.5) to read as secondary. */
  pointSize: number
  /** Draw order — focused overlay layers use a higher value to sit above the base. */
  renderOrder?: number
}) {
  const matRef = useRef<THREE.ShaderMaterial>(null!)
  const uniforms = useMemo(
    () => ({
      uTime: { value: 0 },
      uSize: { value: pointSize },
      uOpacity: { value: baseOpacity },
    }),
    // pointSize and baseOpacity are static per layer — safe to capture once.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  )

  // Advance time on the actual material's uniforms — mutating a ref, no re-render.
  useFrame((_, delta) => {
    if (typeof document !== 'undefined' && document.hidden) return
    const uTime = matRef.current?.uniforms.uTime
    if (uTime) uTime.value += delta
  })

  // Fade the sprites along with the base lines when filtering.
  useEffect(() => {
    const uOpacity = matRef.current?.uniforms.uOpacity
    if (uOpacity) uOpacity.value = dimmed ? 0.2 : baseOpacity
  }, [dimmed, baseOpacity])

  useEffect(() => () => geometry.dispose(), [geometry])

  return (
    <points geometry={geometry} renderOrder={renderOrder} frustumCulled={false} raycast={() => null}>
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

// Renders static base lines and disposes geometry on unmount/change.
function BaseLines({
  geometry,
  opacity,
  renderOrder = 0,
}: {
  geometry: THREE.BufferGeometry
  opacity: number
  /** Draw order — focused overlay layers use a higher value to sit above the base. */
  renderOrder?: number
}) {
  useEffect(() => () => geometry.dispose(), [geometry])
  return (
    <lineSegments geometry={geometry} renderOrder={renderOrder} frustumCulled={false} raycast={() => null}>
      <lineBasicMaterial vertexColors transparent opacity={opacity} depthWrite={false} />
    </lineSegments>
  )
}

// ---------------------------------------------------------------------------
// Public component
// ---------------------------------------------------------------------------

export function TunnelFlow({ nodes, edges, positions, matchedIds = null }: Props) {
  // A project is isolated when matchedIds is a non-null set. In that mode the base
  // layers fade (dimmed) and a brighter "focused" overlay is drawn on top.
  const dimmed = matchedIds != null

  // Predicate: an edge is "focused" when BOTH endpoints belong to the isolated
  // project. null when nothing is isolated (global view) — no overlay is built.
  const isFocused = useMemo(
    () => (matchedIds ? (s: number, t: number) => matchedIds.has(s) && matchedIds.has(t) : null),
    [matchedIds],
  )

  // Build id→index map once per node list (ids are now numbers).
  const idToIndex = useMemo(() => {
    const m = new Map<number, number>()
    nodes.forEach((n, i) => m.set(n.id, i))
    return m
  }, [nodes])

  // Derive synthetic similarity roads from node content (topicKey + title).
  // Render-only layer — no backend contract change. See deriveSimilarityEdges for
  // the TF-IDF / k-NN rationale. Recomputed only when the node set changes.
  const topicEdges = useMemo(() => deriveSimilarityEdges(nodes), [nodes])

  // Build a weight-map for nodes so the particle builder can look up weights for
  // derived topic edges (semantic edges already carry their node data via idToIndex).
  const weightById = useMemo(() => {
    const m = new Map<number, number>()
    for (const n of nodes) m.set(n.id, n.weight)
    return m
  }, [nodes])

  // ---------------------------------------------------------------------------
  // Semantic layer geometries
  // ---------------------------------------------------------------------------

  // Static base lines for every semantic edge — always visible (one draw call).
  const semanticLineGeo = useMemo(
    () => (positions ? buildBaseLineGeometry(edges, idToIndex, positions) : null),
    [edges, idToIndex, positions],
  )

  // Bright bidirectional particles for semantic edges (weight-driven count + speed).
  // Annotate each edge with source/target weights and relation color for the builder.
  const semanticEdgesAnnotated = useMemo(
    () =>
      edges.map((e) => ({
        source: e.source,
        target: e.target,
        weightSource: weightById.get(e.source) ?? 1,
        weightTarget: weightById.get(e.target) ?? 1,
        colorHex: EDGE_COLORS[e.relation],
      })),
    [edges, weightById],
  )

  // Perf: while a project is isolated the base particle layers are NOT rendered —
  // they would draw thousands of near-invisible additive sprites on top of the
  // bright focused overlay, roughly doubling fill-rate exactly when the user is
  // zoomed in on the active galaxy. The faint base LINES below still render for
  // context. Returning null here (vs only skipping the JSX) lets EdgeParticles
  // dispose the geometry on unmount and rebuild a fresh one when isolation clears.
  const semanticParticleGeo = useMemo(
    () =>
      positions && !dimmed
        ? buildParticleGeometry(
            semanticEdgesAnnotated,
            idToIndex,
            positions,
            semanticParticleCount,
            /* baseSpeed */ 0.10,
            /* speedVariance */ 0.06,
            /* totalCap */ TOTAL_PARTICLE_CAP,
          )
        : null,
    [semanticEdgesAnnotated, idToIndex, positions, dimmed],
  )

  // ---------------------------------------------------------------------------
  // Topic layer geometries
  // ---------------------------------------------------------------------------

  // Even fainter base lines for derived shared-topicKey roads (one draw call).
  const topicLineGeo = useMemo(
    () => (positions ? buildTopicLineGeometry(topicEdges, idToIndex, positions) : null),
    [topicEdges, idToIndex, positions],
  )

  // Annotate topic edges with their color for the shared builder.
  const topicEdgesAnnotated = useMemo(
    () =>
      topicEdges.map((e) => ({
        ...e,
        colorHex: TOPIC_ROAD_COLOR,
      })),
    [topicEdges],
  )

  // Dim, sparse particles for topic proximity roads (weight-driven but lower ceiling).
  // Same isolation gate as the semantic particles: skip the base topic-road
  // particle cloud while a project is focused (see note above).
  const topicParticleGeo = useMemo(
    () =>
      positions && !dimmed
        ? buildParticleGeometry(
            topicEdgesAnnotated,
            idToIndex,
            positions,
            topicParticleCount,
            /* baseSpeed */ 0.06,
            /* speedVariance */ 0.03,
            /* totalCap */ Math.floor(TOTAL_PARTICLE_CAP * 0.4), // topic roads stay within 40% budget
          )
        : null,
    [topicEdgesAnnotated, idToIndex, positions, dimmed],
  )

  // ---------------------------------------------------------------------------
  // Focused overlay geometries (only when a project is isolated)
  // ---------------------------------------------------------------------------
  //
  // These rebuild only when matchedIds changes — a click on the project rail, not
  // a per-keystroke input — so the extra geometry passes are cheap in practice.
  // Each is the subset of its layer's edges whose endpoints both sit inside the
  // isolated project (its intra-galaxy roads), rendered bright on top of the
  // faded base layers below.

  const focusedSemanticLineGeo = useMemo(
    () =>
      positions && isFocused
        ? buildBaseLineGeometry(edges.filter((e) => isFocused(e.source, e.target)), idToIndex, positions)
        : null,
    [edges, idToIndex, positions, isFocused],
  )

  const focusedSemanticParticleGeo = useMemo(
    () =>
      positions && isFocused
        ? buildParticleGeometry(
            semanticEdgesAnnotated.filter((e) => isFocused(e.source, e.target)),
            idToIndex,
            positions,
            semanticParticleCount,
            0.10,
            0.06,
            TOTAL_PARTICLE_CAP,
          )
        : null,
    [semanticEdgesAnnotated, idToIndex, positions, isFocused],
  )

  const focusedTopicLineGeo = useMemo(
    () =>
      positions && isFocused
        ? buildTopicLineGeometry(topicEdges.filter((e) => isFocused(e.source, e.target)), idToIndex, positions)
        : null,
    [topicEdges, idToIndex, positions, isFocused],
  )

  const focusedTopicParticleGeo = useMemo(
    () =>
      positions && isFocused
        ? buildParticleGeometry(
            topicEdgesAnnotated.filter((e) => isFocused(e.source, e.target)),
            idToIndex,
            positions,
            topicParticleCount,
            0.06,
            0.03,
            Math.floor(TOTAL_PARTICLE_CAP * 0.4),
          )
        : null,
    [topicEdgesAnnotated, idToIndex, positions, isFocused],
  )

  if (!positions) return null

  return (
    <>
      {/* Semantic layer: bright roads + bright bidirectional particles */}
      {semanticLineGeo ? (
        <BaseLines geometry={semanticLineGeo} opacity={dimmed ? 0.04 : 0.16} />
      ) : null}
      {semanticParticleGeo ? (
        <EdgeParticles
          geometry={semanticParticleGeo}
          dimmed={dimmed}
          baseOpacity={1.0}
          pointSize={2.2}
        />
      ) : null}

      {/* Topic proximity layer: dim roads + dim sparse particles.
          Base floor raised (0.07→0.10 lines, 0.45→0.50 particles) so shared-topic
          roads read a little better in the global view without flooding it. */}
      {topicLineGeo ? (
        <BaseLines geometry={topicLineGeo} opacity={dimmed ? 0.02 : 0.10} />
      ) : null}
      {topicParticleGeo ? (
        <EdgeParticles
          geometry={topicParticleGeo}
          dimmed={dimmed}
          baseOpacity={0.5}
          pointSize={1.5}
        />
      ) : null}

      {/* Focused overlay: the isolated project's own intra-galaxy roads, drawn
          bright on top of the faded base. renderOrder keeps them above the base
          transparents so they aren't occluded by the dimmed layers. */}
      {focusedSemanticLineGeo ? (
        <BaseLines geometry={focusedSemanticLineGeo} opacity={0.5} renderOrder={2} />
      ) : null}
      {focusedSemanticParticleGeo ? (
        <EdgeParticles
          geometry={focusedSemanticParticleGeo}
          dimmed={false}
          baseOpacity={1.0}
          pointSize={2.4}
          renderOrder={3}
        />
      ) : null}
      {focusedTopicLineGeo ? (
        <BaseLines geometry={focusedTopicLineGeo} opacity={0.3} renderOrder={2} />
      ) : null}
      {focusedTopicParticleGeo ? (
        <EdgeParticles
          geometry={focusedTopicParticleGeo}
          dimmed={false}
          baseOpacity={0.75}
          pointSize={1.8}
          renderOrder={3}
        />
      ) : null}
    </>
  )
}
