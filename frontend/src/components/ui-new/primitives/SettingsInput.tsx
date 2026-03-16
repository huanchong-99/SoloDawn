import * as React from 'react';

import { cn } from '@/lib/utils';

export interface SettingsInputProps {
  label: string;
  description?: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  type?: 'text' | 'password' | 'url';
  error?: string;
  disabled?: boolean;
  className?: string;
}

const SettingsInput = React.forwardRef<HTMLDivElement, SettingsInputProps>(
  (
    {
      label,
      description,
      value,
      onChange,
      placeholder,
      type = 'text',
      error,
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
        {error && <p className="text-error text-sm mt-0.5">{error}</p>}
      </div>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        disabled={disabled}
        className={cn(
          'shrink-0 w-56 rounded border border-border bg-secondary px-base py-1 text-base text-normal placeholder:text-low',
          'focus:outline-none focus:ring-1 focus:ring-brand',
          error && 'border-error',
          disabled && 'opacity-60 cursor-not-allowed'
        )}
      />
    </div>
  )
);
SettingsInput.displayName = 'SettingsInput';

export { SettingsInput };
