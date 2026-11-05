// Off-main-thread 3D force-directed layout.
// Receives { nodes: {id: number; cluster: number; subCluster: number}[], edges: {source: number; target: number}[], iterations? }
// Returns transferable Float32Array of positions (x,y,z interleaved).
// The caller maps numeric IDs → indices and derives cluster/subCluster from project/topicKey before posting.
// The simulation itself lives in force-layout.ts (pure, unit-tested).
import { computeLayout, type LayoutEdge, type LayoutNode } from './force-layout.ts';

type WorkerMsg = { nodes: LayoutNode[]; edges: LayoutEdge[]; iterations?: number };

self.onmessage = (e: MessageEvent<WorkerMsg>) => {
  const { nodes, edges, iterations } = e.data;
  const t0 = performance.now();
  const pos = computeLayout(nodes, edges, iterations);
  if (import.meta.env.DEV) {
    console.log(
      `[force-layout] n=${nodes.length} edges=${edges.length} done in ${(performance.now() - t0).toFixed(0)}ms`,
    );
  }
  // Transfer the buffer zero-copy back to the main thread.
  (self as unknown as Worker).postMessage({ positions: pos, count: nodes.length }, [pos.buffer]);
};
