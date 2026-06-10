/**
 * tfidf.ts — Single source of truth for lexical / TF-IDF primitives used by
 * the brain graph (tunnel-flow.tsx, views/organico.ts) and the keyword chips
 * shown in NodeHoverTooltip and NodeDetailPanel.
 *
 * All functions are pure and free of side-effects so they can run safely inside
 * useMemo without causing re-render churn.
 */

import type { GraphNode } from './api.ts';

// ---------------------------------------------------------------------------
// Lexical constants
// ---------------------------------------------------------------------------

export const MIN_TOKEN_LEN = 3;

export const TOPIC_TOKEN_WEIGHT = 2; // curated path → stronger signal than free-text title
export const TITLE_TOKEN_WEIGHT = 1;

// Low-value tokens (stopwords EN+ES + generic engineering verbs) that would
// otherwise inflate similarity without carrying topic meaning.
export const STOPWORDS = new Set([
  'the',
  'and',
  'for',
  'with',
  'from',
  'this',
  'that',
  'are',
  'was',
  'use',
  'using',
  'add',
  'set',
  'new',
  'fix',
  'fixed',
  'via',
  'into',
  'per',
  'los',
  'las',
  'una',
  'con',
  'para',
  'por',
  'del',
  'que',
  'como',
  'sus',
  'este',
  'esta',
]);

// Types that carry no topical content: session summaries are template-titled
// ("Session summary: <project>") with no topicKey, so similarity would wire
// them into a dense hairball on the shared template words alone.
export const NON_TOPICAL_TYPES = new Set(['session_summary']);

// ---------------------------------------------------------------------------
// Tokenization
// ---------------------------------------------------------------------------

/**
 * Split on any non-alphanumeric run (covers '/', '-', '_', spaces,
 * punctuation), lowercase, and drop stopwords / very short tokens.
 * Keeps Spanish accents (á é í ó ú ñ ü).
 */
export function tokenize(text: string | null | undefined): string[] {
  if (!text) return [];
  const out: string[] = [];
  for (const raw of text.toLowerCase().split(/[^a-z0-9áéíóúñü]+/i)) {
    if (raw.length >= MIN_TOKEN_LEN && !STOPWORDS.has(raw)) out.push(raw);
  }
  return out;
}

// ---------------------------------------------------------------------------
// Term frequency
// ---------------------------------------------------------------------------

/**
 * Tokenize `text` and count raw occurrences per token.
 * Each occurrence increments the count by 1 (no weighting here — callers
 * apply TOPIC_TOKEN_WEIGHT / TITLE_TOKEN_WEIGHT before calling this, or use
 * this for flat content where all tokens are equal-weight).
 */
export function termFrequencies(text: string | null | undefined): Map<string, number> {
  const freq = new Map<string, number>();
  for (const tok of tokenize(text)) {
    freq.set(tok, (freq.get(tok) ?? 0) + 1);
  }
  return freq;
}

// ---------------------------------------------------------------------------
// Top-k selection
// ---------------------------------------------------------------------------

/**
 * Given a weight vector, return the top-k tokens sorted by weight desc.
 * Ties are broken alphabetically (determinism across re-renders).
 * Tokens whose weight is below `minRatio * maxWeight` are excluded.
 * Returns [] for an empty map or k ≤ 0.
 */
export function topKeywords(vec: Map<string, number>, k: number, minRatio = 0): string[] {
  if (vec.size === 0 || k <= 0) return [];

  let maxWeight = 0;
  for (const w of vec.values()) {
    if (w > maxWeight) maxWeight = w;
  }

  const floor = minRatio * maxWeight;
  const entries: Array<[string, number]> = [];
  for (const [tok, w] of vec) {
    if (w >= floor) entries.push([tok, w]);
  }

  // Sort desc by weight; break ties alphabetically.
  entries.sort(([ta, wa], [tb, wb]) => {
    if (wb !== wa) return wb - wa;
    return ta < tb ? -1 : ta > tb ? 1 : 0;
  });

  return entries.slice(0, k).map(([tok]) => tok);
}

// ---------------------------------------------------------------------------
// Per-node keyword computation (intra-project TF-IDF)
// ---------------------------------------------------------------------------

/**
 * Replicate the WITHIN-PROJECT TF-IDF that deriveSimilarityEdges uses to
 * compute meaningful per-node keyword sets.
 *
 * Algorithm (mirrors deriveSimilarityEdges step-by-step):
 *   1. Filter NON_TOPICAL_TYPES; group remaining nodes by project.
 *   2. Skip projects with N < 2 (IDF is undefined for a singleton group).
 *   3. Per node: weighted TF (topicKey × TOPIC_TOKEN_WEIGHT, label × TITLE_TOKEN_WEIGHT).
 *   4. Per token: IDF = log(N / df); skip if IDF ≤ 0 (token in every node → zero discriminative value).
 *   5. TF-IDF weight vector → topKeywords(vec, k, 0.15).
 *
 * Keys the returned Map by GraphNode.id (number).
 */
export function computeNodeKeywords(nodes: GraphNode[], k = 5): Map<number, string[]> {
  const result = new Map<number, string[]>();

  // Group by project, skipping non-topical types.
  const byProject = new Map<string, GraphNode[]>();
  for (const n of nodes) {
    if (n.type != null && NON_TOPICAL_TYPES.has(n.type)) continue;
    const p = n.project ?? '∅';
    const bucket = byProject.get(p);
    if (bucket) bucket.push(n);
    else byProject.set(p, [n]);
  }

  for (const [, members] of byProject) {
    const N = members.length;
    if (N < 2) continue;

    // Step 1: weighted term frequency per node + document frequency per token.
    const tfMaps: Map<string, number>[] = [];
    const df = new Map<string, number>();

    for (const m of members) {
      const tf = new Map<string, number>();
      for (const tok of tokenize(m.topicKey)) {
        tf.set(tok, (tf.get(tok) ?? 0) + TOPIC_TOKEN_WEIGHT);
      }
      for (const tok of tokenize(m.label)) {
        tf.set(tok, (tf.get(tok) ?? 0) + TITLE_TOKEN_WEIGHT);
      }
      tfMaps.push(tf);
      for (const tok of tf.keys()) df.set(tok, (df.get(tok) ?? 0) + 1);
    }

    // Step 2: TF-IDF vector per node; skip tokens with IDF ≤ 0.
    for (let i = 0; i < N; i++) {
      const vec = new Map<string, number>();
      for (const [tok, freq] of tfMaps[i]!) {
        const idf = Math.log(N / (df.get(tok) ?? 1));
        if (idf <= 0) continue; // token in every node → zero discriminative value
        vec.set(tok, freq * idf);
      }
      const keywords = topKeywords(vec, k, 0.15);
      // Only insert when there is at least one keyword — callers can rely on
      // `result.has(id)` meaning "this node has discriminative keywords".
      if (keywords.length > 0) result.set(members[i]!.id, keywords);
    }
  }

  return result;
}
