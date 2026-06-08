import { X } from 'lucide-react';
import { useEffect, useRef, useState, type JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '../ui/badge.tsx';
import { Input } from '../ui/input.tsx';

interface MultiSelectChipsProps {
  label: string;
  values: string[];
  options?: string[];
  onChange: (values: string[]) => void;
  placeholder?: string;
  compact?: boolean;
}

export function MultiSelectChips({
  label,
  values,
  options = [],
  onChange,
  placeholder,
  compact = false,
}: MultiSelectChipsProps): JSX.Element {
  const { t } = useTranslation();
  const [draft, setDraft] = useState('');
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const filteredOptions = options
    .filter((o) => !values.includes(o))
    .filter((o) => o.toLowerCase().includes(draft.toLowerCase()))
    .slice(0, 8);

  useEffect(() => {
    if (!compact) return;
    function handleClickOutside(e: MouseEvent): void {
      if (!containerRef.current) return;
      if (!containerRef.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [compact]);

  function add(value: string): void {
    if (value === '' || values.includes(value)) return;
    onChange([...values, value]);
    setDraft('');
  }

  function remove(value: string): void {
    onChange(values.filter((v) => v !== value));
  }

  if (compact) {
    const showDropdown = open && filteredOptions.length > 0;
    return (
      <div ref={containerRef} className="relative flex flex-col gap-1">
        <div
          className="flex h-9 items-center gap-1.5 overflow-x-auto rounded-md border border-border bg-surface-2 px-2"
          aria-label={label}
        >
          {values.map((v) => (
            <Badge key={v} tone="accent" className="shrink-0 gap-1">
              {v}
              <button
                type="button"
                aria-label={t('multiSelect.removeAria', { value: v })}
                className="text-fg-muted hover:text-fg"
                onClick={() => remove(v)}
              >
                <X size={12} />
              </button>
            </Badge>
          ))}
          <input
            className="h-7 min-w-[80px] flex-1 border-0 bg-transparent text-sm text-fg outline-none placeholder:text-fg-muted"
            aria-label={label}
            placeholder={values.length === 0 ? (placeholder ?? label) : ''}
            value={draft}
            onChange={(e) => {
              setDraft(e.target.value);
              setOpen(true);
            }}
            onFocus={() => setOpen(true)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                add(draft.trim());
              } else if (e.key === 'Backspace' && draft === '' && values.length > 0) {
                const last = values[values.length - 1];
                if (last) remove(last);
              } else if (e.key === 'Escape') {
                setOpen(false);
              }
            }}
          />
        </div>
        {showDropdown ? (
          <div className="absolute left-0 right-0 top-full z-20 mt-1 max-h-56 overflow-auto rounded-md border border-border bg-surface shadow-lg">
            <ul className="py-1">
              {filteredOptions.map((o) => (
                <li key={o}>
                  <button
                    type="button"
                    className="block w-full px-3 py-1.5 text-left text-sm text-fg hover:bg-surface-2"
                    onClick={() => {
                      add(o);
                      setOpen(false);
                    }}
                  >
                    {o}
                  </button>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-1">
      <label className="text-xs uppercase tracking-wide text-fg-muted">{label}</label>
      <div className="flex flex-wrap items-center gap-1.5 rounded-md border border-border bg-surface-2 p-1.5">
        {values.map((v) => (
          <Badge key={v} tone="accent" className="gap-1">
            {v}
            <button
              type="button"
              aria-label={t('multiSelect.removeAria', { value: v })}
              className="text-fg-muted hover:text-fg"
              onClick={() => remove(v)}
            >
              <X size={12} />
            </button>
          </Badge>
        ))}
        <Input
          className="min-w-[120px] flex-1 border-0 bg-transparent p-0 focus:ring-0"
          placeholder={values.length === 0 ? (placeholder ?? t('multiSelect.addPlaceholder', { label })) : ''}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              add(draft.trim());
            } else if (e.key === 'Backspace' && draft === '' && values.length > 0) {
              const last = values[values.length - 1];
              if (last) remove(last);
            }
          }}
        />
      </div>
      {filteredOptions.length > 0 && draft.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {filteredOptions.map((o) => (
            <button
              key={o}
              type="button"
              className="rounded-md border border-border bg-surface px-2 py-0.5 text-xs text-fg-muted hover:text-fg"
              onClick={() => add(o)}
            >
              + {o}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
