import { useEffect, useState } from 'react';

const STORAGE_KEY = 'engram-explorer:sidebar-collapsed';

export function readStoredCollapsed(): boolean {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === 'true';
  } catch {
    // ignore (private mode / SSR)
    return false;
  }
}

export function useSidebarCollapsed(): readonly [boolean, () => void] {
  const [collapsed, setCollapsed] = useState<boolean>(() => readStoredCollapsed());

  useEffect(() => {
    try {
      window.localStorage.setItem(STORAGE_KEY, String(collapsed));
    } catch {
      // ignore
    }
  }, [collapsed]);

  const toggle = (): void => setCollapsed((prev) => !prev);

  return [collapsed, toggle] as const;
}
