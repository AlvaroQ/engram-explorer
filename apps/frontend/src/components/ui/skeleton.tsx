import type { HTMLAttributes, JSX } from 'react';
import { cn } from '../../lib/cn.ts';

export function Skeleton({ className, ...props }: HTMLAttributes<HTMLDivElement>): JSX.Element {
  return (
    <div
      className={cn('animate-pulse rounded-md bg-surface-2', className)}
      aria-hidden="true"
      {...props}
    />
  );
}
