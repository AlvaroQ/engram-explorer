import { forwardRef, type InputHTMLAttributes } from 'react';
import { cn } from '../../lib/cn.ts';

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  function Input({ className, ...props }, ref) {
    return (
      <input
        ref={ref}
        className={cn(
          'h-9 w-full rounded-md border border-border bg-surface-2 px-3 text-sm text-fg outline-none',
          'placeholder:text-fg-muted focus:border-accent focus:ring-1 focus:ring-accent/40',
          className,
        )}
        {...props}
      />
    );
  },
);
