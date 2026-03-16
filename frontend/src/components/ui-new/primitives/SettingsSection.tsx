import * as React from 'react';

import { cn } from '@/lib/utils';

export interface SettingsSectionProps {
  title?: string;
  children: React.ReactNode;
  className?: string;
}

const SettingsSection = React.forwardRef<HTMLDivElement, SettingsSectionProps>(
  ({ title, children, className }, ref) => (
    <div ref={ref} className={cn('space-y-base', className)}>
      {title && (
        <h4 className="text-normal text-sm font-medium uppercase tracking-wider">
          {title}
        </h4>
      )}
      {children}
    </div>
  )
);
SettingsSection.displayName = 'SettingsSection';

export { SettingsSection };
