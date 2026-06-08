// Segmented control for the Brain graph: switches the node color dimension
// between project and type.
//
// Variants:
//   - default (legacy): absolutely positioned at top-center; used by the old layout.
//   - compact (secondary): inline control suitable for placement alongside other
//     controls (e.g. right of the view-switcher or in the page header). D3.
import { type JSX } from 'react'
import { useTranslation } from 'react-i18next'
import type { ColorBy } from './node-colors.ts'

const OPTIONS: ColorBy[] = ['project', 'type']

interface Props {
  colorBy: ColorBy
  onChange: (colorBy: ColorBy) => void
  /**
   * When true, renders as a compact secondary control (inline, no absolute positioning).
   * When false (default), renders as the legacy floating top-center overlay.
   */
  compact?: boolean
}

export function ColorByToggle({ colorBy, onChange, compact = false }: Props): JSX.Element {
  const { t } = useTranslation()

  const containerClass = compact
    ? 'flex items-center gap-0.5 rounded border border-border bg-surface px-1 py-0.5'
    : 'pointer-events-auto absolute left-1/2 top-3 z-10 flex -translate-x-1/2 items-center gap-1 rounded-lg border border-border bg-surface px-2 py-1 shadow-lg'

  return (
    <div className={containerClass}>
      {OPTIONS.map((opt) => (
        <button
          key={opt}
          type="button"
          onClick={() => onChange(opt)}
          aria-pressed={colorBy === opt}
          className={`rounded border px-1.5 py-0.5 text-[11px] transition-colors ${
            colorBy === opt
              ? 'border-accent font-medium text-fg ring-1 ring-accent'
              : 'border-transparent text-fg-muted hover:bg-surface-2 hover:text-fg'
          }`}
        >
          {t(`brain.colorBy.${opt}`)}
        </button>
      ))}
    </div>
  )
}
