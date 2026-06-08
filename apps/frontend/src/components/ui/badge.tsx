import { cva, type VariantProps } from 'class-variance-authority';
import type { HTMLAttributes, JSX } from 'react';
import { cn } from '../../lib/cn.ts';

const badgeStyles = cva(
  'inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium border',
  {
    variants: {
      tone: {
        neutral: 'border-border bg-surface-2 text-fg-muted',
        accent: 'border-accent/40 bg-accent/15 text-accent',
        ok: 'border-ok/40 bg-ok/15 text-ok',
        warn: 'border-warn/40 bg-warn/15 text-warn',
        fail: 'border-fail/40 bg-fail/15 text-fail',
      },
    },
    defaultVariants: { tone: 'neutral' },
  },
);

interface BadgeProps extends HTMLAttributes<HTMLSpanElement>, VariantProps<typeof badgeStyles> {}

export function Badge({ tone, className, ...props }: BadgeProps): JSX.Element {
  return <span className={cn(badgeStyles({ tone }), className)} {...props} />;
}
