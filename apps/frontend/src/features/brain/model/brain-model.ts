/**
 * brain-model.ts — Seam file for the BrainModel interface.
 *
 * Re-exports the BrainModel interface from types.ts. This file is the
 * canonical import target for consumers. When the backend implementation
 * lands, only this file and the factory wiring change — all UI imports stay.
 */

export type {
  BrainModel,
  BrainLevel,
  BrainNode,
  BrainNodeRef,
  BrainEdge,
  LevelStats,
  ActiveFacets,
  FacetGroupId,
  ViewMode,
  NodeKind,
  EdgeFamily,
  GraphEdgeRelation,
} from './types.ts';
