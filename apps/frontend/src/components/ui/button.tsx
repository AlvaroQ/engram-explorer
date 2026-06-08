import { cva, type VariantProps } from 'class-variance-authority';
import { forwardRef, type ButtonHTMLAttributes } from 'react';
import { cn } from '../../lib/cn.ts';

const buttonStyles = cva(
  'inline-flex items-center justify-center gap-1.5 rounded-md font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50 focus:outline-none focus:ring-1 focus:ring-accent/60',
  {
    variants: {
      variant: {
        primary: 'bg-accent text-bg hover:opacity-90',
        secondary: 'bg-surface-2 text-fg border border-border hover:bg-surface',
        ghost: 'text-fg-muted hover:text-fg hover:bg-surface-2',
        danger: 'bg-fail/15 text-fail border border-fail/40 hover:bg-fail/25',
      },
      size: {
        sm: 'h-7 px-2 text-xs',
        md: 'h-9 px-3 text-sm',
        lg: 'h-10 px-4 text-sm',
      },
    },
    defaultVariants: { variant: 'secondary', size: 'md' },
  },
);

interface ButtonProps
  extends ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonStyles> {}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant, size, className, ...props },
  ref,
) {
  return (
    <button
      ref={ref}
      type={props.type ?? 'button'}
      className={cn(buttonStyles({ variant, size }), className)}
      {...props}
    />
  );
});
