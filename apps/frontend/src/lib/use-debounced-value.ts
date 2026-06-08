import { useEffect, useState } from 'react';

/**
 * Returns a debounced copy of `value` that only updates after `ms` milliseconds
 * of inactivity. Defaults to 250ms — matches the debounce delays used across
 * FilterBar (observations-page), PromptsPage, and useSpotlightSearch.
 */
export function useDebouncedValue<T>(value: T, ms = 250): T {
  const [debounced, setDebounced] = useState<T>(value);

  useEffect(() => {
    const id = window.setTimeout(() => setDebounced(value), ms);
    return () => window.clearTimeout(id);
  }, [value, ms]);

  return debounced;
}
