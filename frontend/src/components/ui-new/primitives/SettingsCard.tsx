import * as React from 'react';

import { cn } from '@/lib/utils';

export interface SettingsCardProps {
  title: string;
  description?: string;
  children: React.ReactNode;
  className?: string;
}

const SettingsCard = React.forwardRef<HTMLDivElement, SettingsCardProps>(
  ({ title, description, children, className }, ref) => (
    <div
      ref={ref}
      className={cn('bg-panel rounded-lg border p-double', className)}
    >
      <div className="mb-base">
        <h3 className="text-high text-lg font-medium">{title}</h3>
        {description && (
          <p className="text-low text-sm mt-1">{description}</p>
        )}
      </div>
      <div>{children}</div>
    </div>
  )
);
SettingsCard.displayName = 'SettingsCard';

export { SettingsCard };
