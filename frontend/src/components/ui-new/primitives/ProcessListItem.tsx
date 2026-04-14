import {
  TerminalIcon,
  GearIcon,
  CodeIcon,
  GlobeIcon,
} from '@phosphor-icons/react';
import { cn } from '@/lib/utils';
import { formatRelativeTime } from '@/utils/date';
import { RunningDots } from './RunningDots';
import type {
  ExecutionProcessStatus,
  ExecutionProcessRunReason,
} from 'shared/types';

interface ProcessListItemProps {
  readonly runReason: ExecutionProcessRunReason;
  readonly status: ExecutionProcessStatus;
  readonly startedAt: string;
  readonly selected?: boolean;
  readonly onClick?: () => void;
  readonly className?: string;
}

const RUN_REASON_LABELS: Record<ExecutionProcessRunReason, string> = {
  codingagent: 'Coding Agent',
  setupscript: 'Setup Script',
  cleanupscript: 'Cleanup Script',
  devserver: 'Dev Server',
  qualityscan: 'Quality Scan',
};

const RUN_REASON_ICONS: Record<ExecutionProcessRunReason, typeof TerminalIcon> =
  {
    codingagent: CodeIcon,
    setupscript: GearIcon,
    cleanupscript: GearIcon,
    devserver: GlobeIcon,
    qualityscan: GearIcon,
  };

const STATUS_COLORS: Record<ExecutionProcessStatus, string> = {
  running: 'bg-info',
  completed: 'bg-success',
  failed: 'bg-destructive',
  killed: 'bg-low',
};

export function ProcessListItem({
  runReason,
  status,
  startedAt,
  selected,
  onClick,
  className,
}: Readonly<ProcessListItemProps>) {
  const IconComponent = RUN_REASON_ICONS[runReason];
  const label = RUN_REASON_LABELS[runReason];
  const statusColor = STATUS_COLORS[status];

  const isRunning = status === 'running';

  // TODO (P3): Native <button> already handles Enter/Space activation, so no explicit
  // onKeyDown is required (E06-03).
  // TODO (P2): `selected` prop only drives text color on the inner <span>. Consider
  // adding a container-level selected style (e.g., data-[state=selected]:ring-2 or
  // bg-secondary) for clearer affordance (E06-10).
  return (
    <button
      type="button"
      onClick={onClick}
      data-state={selected ? 'selected' : undefined}
      aria-pressed={selected}
      className={cn(
        'w-full h-[26px] flex items-center gap-half px-half rounded-sm text-left transition-colors',
        selected && 'bg-surface-2',
        className
      )}
    >
      <IconComponent
        className="size-icon-sm flex-shrink-0 text-low"
        weight="regular"
      />
      {isRunning ? (
        <RunningDots />
      ) : (
        <span
          className={cn('size-dot rounded-full flex-shrink-0', statusColor)}
          title={status}
        />
      )}
      <span
        className={cn(
          'text-sm truncate flex-1',
          selected ? 'text-high' : 'text-normal'
        )}
      >
        {label}
      </span>
      <span className="text-xs text-low flex-shrink-0">
        {formatRelativeTime(startedAt)}
      </span>
    </button>
  );
}
