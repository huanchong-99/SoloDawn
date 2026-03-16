import * as React from 'react';
import { CaretDownIcon } from '@phosphor-icons/react';

import { cn } from '@/lib/utils';

export interface SettingsSelectProps {
  label: string;
  description?: string;
  value: string;
  onChange: (value: string) => void;
  options: Array<{ value: string; label: string }>;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}

const SettingsSelect = React.forwardRef<HTMLDivElement, SettingsSelectProps>(
  (
    {
      label,
      description,
      value,
      onChange,
      options,
      placeholder,
      disabled = false,
      className,
    },
    ref
  ) => (
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
      <div className="relative shrink-0">
        <select
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
          className={cn(
            'appearance-none rounded border border-border bg-secondary px-base py-1 pr-7 text-base text-normal',
            'focus:outline-none focus:ring-1 focus:ring-brand',
            disabled && 'opacity-60 cursor-not-allowed'
          )}
        >
          {placeholder && (
            <option value="" disabled>
              {placeholder}
            </option>
          )}
          {options.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
        <CaretDownIcon
          className="size-icon-xs absolute right-1.5 top-1/2 -translate-y-1/2 text-low pointer-events-none"
          weight="bold"
        />
      </div>
    </div>
  )
);
SettingsSelect.displayName = 'SettingsSelect';

export { SettingsSelect };
