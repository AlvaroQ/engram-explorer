import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api, type GraphNode, type ObservationSearchItem } from './api.ts';
import { useDebouncedValue } from './use-debounced-value.ts';

const LOCAL_RESULTS_CAP = 8;
const CONTENT_RESULTS_LIMIT = 8;

export interface SpotlightSearchResult {
  localResults: GraphNode[];
  contentResults: ObservationSearchItem[];
  isSearching: boolean;
}

export function useSpotlightSearch(
  query: string,
  nodes: GraphNode[],
  searchableIndex: Map<number, string>,
  open: boolean,
): SpotlightSearchResult {
  const debouncedQuery = useDebouncedValue(query);

  // Local search: synchronous substring match over the precomputed searchableIndex.
  // No debounce — instant. Cap at LOCAL_RESULTS_CAP.
  // Memoized so it only recomputes when the query or node set changes — NOT on
  // every render (e.g. ArrowUp/Down only change activeIndex upstream).
  const localResults = useMemo<GraphNode[]>(() => {
    const q = query.trim().toLowerCase();
    if (!q) return [];
    const results: GraphNode[] = [];
    for (const node of nodes) {
      if (results.length >= LOCAL_RESULTS_CAP) break;
      const text = searchableIndex.get(node.id) ?? '';
      if (text.includes(q)) {
        results.push(node);
      }
    }
    return results;
  }, [query, nodes, searchableIndex]);

  // FTS5 content search — gated on open + debounced query length
  const enabled = open && debouncedQuery.trim().length >= 2;

  const { data: contentData, isFetching } = useQuery({
    queryKey: ['spotlight', 'content', debouncedQuery],
    queryFn: () => api.searchObservations(debouncedQuery, CONTENT_RESULTS_LIMIT),
    enabled,
  });

  const contentResults: ObservationSearchItem[] = contentData?.items ?? [];

  return {
    localResults,
    contentResults,
    isSearching: isFetching,
  };
}
