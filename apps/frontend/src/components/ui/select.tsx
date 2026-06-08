import { forwardRef, type SelectHTMLAttributes } from 'react';
import { cn } from '../../lib/cn.ts';

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(
  function Select({ className, children, ...props }, ref) {
    return (
      <select
        ref={ref}
        className={cn(
          'h-9 rounded-md border border-border bg-surface-2 px-2 text-sm text-fg outline-none focus:border-accent',
          className,
        )}
        {...props}
      >
        {children}
      </select>
    );
  },
);
