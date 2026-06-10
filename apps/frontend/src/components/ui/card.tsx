import type { HTMLAttributes, ReactNode, JSX } from 'react';
import { cn } from '../../lib/cn.ts';

export function Card({ className, ...props }: HTMLAttributes<HTMLDivElement>): JSX.Element {
  return (
    <div
      className={cn(
        'rounded-lg border border-border bg-surface shadow-sm shadow-black/10',
        className,
      )}
      {...props}
    />
  );
}

interface CardHeaderProps extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  actionsClassName?: string;
}

export function CardHeader({
  title,
  description,
  actions,
  actionsClassName,
  className,
  ...props
}: CardHeaderProps): JSX.Element {
  return (
    <div
      className={cn(
        'flex items-start justify-between gap-3 border-b border-border px-5 py-4',
        className,
      )}
      {...props}
    >
      <div className="min-w-0 shrink-0">
        <h2 className="text-sm font-semibold text-fg">{title}</h2>
        {description ? <p className="mt-1 text-xs text-fg-muted">{description}</p> : null}
      </div>
      {actions ? <div className={cn('shrink-0', actionsClassName)}>{actions}</div> : null}
    </div>
  );
}

export function CardBody({ className, ...props }: HTMLAttributes<HTMLDivElement>): JSX.Element {
  return <div className={cn('px-5 py-4', className)} {...props} />;
}
