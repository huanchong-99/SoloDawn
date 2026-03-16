import * as React from 'react';
import { FloppyDiskIcon } from '@phosphor-icons/react';

import { cn } from '@/lib/utils';

export interface SettingsSaveBarProps {
  visible: boolean;
  onSave: () => void;
  onDiscard: () => void;
  saving?: boolean;
  className?: string;
}

const SettingsSaveBar = React.forwardRef<HTMLDivElement, SettingsSaveBarProps>(
  ({ visible, onSave, onDiscard, saving = false, className }, ref) => {
    if (!visible) return null;

    return (
      <div
        ref={ref}
        className={cn(
          'fixed bottom-0 left-0 right-0 z-50 flex items-center justify-end gap-base bg-panel border-t border-border px-double py-base',
          className
        )}
      >
        <button
          type="button"
          onClick={onDiscard}
          disabled={saving}
          className="rounded border border-border bg-secondary px-base py-1 text-sm text-normal hover:bg-surface-2 transition-colors duration-200 disabled:opacity-60 disabled:cursor-not-allowed"
        >
          Discard
        </button>
        <button
          type="button"
          onClick={onSave}
          disabled={saving}
          className="inline-flex items-center gap-1.5 rounded bg-brand px-base py-1 text-sm text-white hover:bg-brand-hover transition-colors duration-200 disabled:opacity-60 disabled:cursor-not-allowed"
        >
          <FloppyDiskIcon className="size-icon-xs" weight="bold" />
          {saving ? 'Saving...' : 'Save changes'}
        </button>
      </div>
    );
  }
);
SettingsSaveBar.displayName = 'SettingsSaveBar';

export { SettingsSaveBar };
