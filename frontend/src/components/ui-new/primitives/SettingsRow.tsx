import * as React from 'react';

import { cn } from '@/lib/utils';

export interface SettingsRowProps {
  label: string;
  description?: string;
  children: React.ReactNode;
  className?: string;
  error?: string;
}

const SettingsRow = React.forwardRef<HTMLDivElement, SettingsRowProps>(
  ({ label, description, children, className, error }, ref) => (
    <div
      ref={ref}
      className={cn('flex items-start justify-between gap-double', className)}
    >
      <div className="flex-1 min-w-0">
        <span className="text-normal text-base">{label}</span>
        {description && (
          <p className="text-low text-sm mt-0.5">{description}</p>
        )}
        {error && <p className="text-error text-sm mt-0.5">{error}</p>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  )
);
SettingsRow.displayName = 'SettingsRow';

export { SettingsRow };
