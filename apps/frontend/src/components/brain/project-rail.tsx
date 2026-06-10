// Side rail of project containers for the Brain graph. Each project is its own
// container, stacked vertically. Clicking one isolates that project (highlights
// its nodes, dims the rest); clicking the active one again clears the isolation.
// The graph itself colors nodes by type — the swatch here is a per-project
// identifier (buildHexPalette) so each rail entry stays visually distinct.
import { type JSX } from 'react';
import { useTranslation } from 'react-i18next';

export interface ProjectRailItem {
  /** Project name (a real value, never null). */
  project: string;
  /** Number of nodes in this project. */
  count: number;
  /** Identifier color (CSS hsl) from the project palette. */
  color: string;
}

interface Props {
  items: ProjectRailItem[];
  /** Currently isolated project, or null when nothing is isolated. */
  isolated: string | null;
  onToggle: (project: string) => void;
}

export function ProjectRail({ items, isolated, onToggle }: Props): JSX.Element | null {
  const { t } = useTranslation();

  if (items.length === 0) return null;

  // Req 3 — barely-there floating rail: the container background is almost
  // imperceptible (you can hardly tell it exists); only the item cards carry a
  // faint tint to keep their text legible over the 3D scene.
  return (
    <div className="flex h-full w-44 flex-col gap-1.5 overflow-y-auto py-2 px-2 bg-surface/5 backdrop-blur-[2px]">
      {items.map((it) => {
        const active = isolated === it.project;
        const dimmed = isolated !== null && !active;
        return (
          <button
            key={it.project}
            type="button"
            onClick={() => onToggle(it.project)}
            aria-pressed={active}
            aria-label={t('brain.projectRail.isolate', { project: it.project })}
            title={it.project}
            className={`flex w-full items-center gap-2 rounded-lg border p-2 text-left transition-all hover:border-accent bg-bg/35 backdrop-blur-sm ${
              active ? 'border-accent ring-1 ring-accent' : 'border-border/40'
            } ${dimmed ? 'opacity-40' : ''}`}
          >
            <span
              className="h-2.5 w-2.5 shrink-0 rounded-full"
              style={{ backgroundColor: it.color }}
              aria-hidden
            />
            <span className="min-w-0 flex-1 truncate text-xs font-medium text-fg">
              {it.project}
            </span>
            <span className="shrink-0 font-mono text-[10px] text-fg-muted">{it.count}</span>
          </button>
        );
      })}
    </div>
  );
}
