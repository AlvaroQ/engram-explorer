import { Outlet, useRouterState } from '@tanstack/react-router';
import { Sidebar } from './sidebar.tsx';
import { useSidebarCollapsed } from '../../lib/sidebar.ts';

import type { JSX } from "react";

export function AppShell(): JSX.Element {
  const [collapsed, toggleSidebar] = useSidebarCollapsed();
  // The Brain page is a full-bleed 3D canvas: it manages its own height and must
  // sit edge-to-edge, so we drop the standard page padding/scroll for that route.
  const isFullBleed = useRouterState({ select: (s) => s.location.pathname === '/brain' });
  return (
    <div className="flex h-screen w-full">
      <Sidebar collapsed={collapsed} onToggle={toggleSidebar} />
      <div className="flex flex-1 flex-col overflow-hidden">
        <main
          className={isFullBleed ? 'flex-1 overflow-hidden' : 'flex-1 overflow-y-auto px-8 py-6'}
          role="main"
        >
          <Outlet />
        </main>
      </div>
    </div>
  );
}
