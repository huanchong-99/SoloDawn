import * as React from 'react';

import { cn } from '@/lib/utils';

export interface SettingsToggleProps {
  label: string;
  description?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  className?: string;
}

const SettingsToggle = React.forwardRef<HTMLDivElement, SettingsToggleProps>(
  ({ label, description, checked, onChange, disabled = false, className }, ref) => (
    <div
      ref={ref}
      className={cn('flex items-start justify-between gap-double', className)}
    >
      <div className="flex-1 min-w-0">
        <span className="text-normal text-base">{label}</span>
        {description && (
          <p className="text-low text-sm mt-0.5">{description}</p>
        )}
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={cn(
          'relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border transition-colors duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/40 focus-visible:ring-offset-2 focus-visible:ring-offset-background',
          checked ? 'bg-brand border-brand' : 'bg-secondary border-border',
          disabled && 'opacity-60 cursor-not-allowed'
        )}
      >
        <span
          className={cn(
            'pointer-events-none block h-3.5 w-3.5 rounded-full bg-white shadow-sm transition-transform duration-200',
            checked ? 'translate-x-[18px]' : 'translate-x-[3px]'
          )}
        />
      </button>
    </div>
  )
);
SettingsToggle.displayName = 'SettingsToggle';

export { SettingsToggle };
