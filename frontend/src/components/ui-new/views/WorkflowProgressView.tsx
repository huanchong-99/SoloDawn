import {
  CaretRightIcon,
  GitBranchIcon,
  CheckCircleIcon,
  CircleNotchIcon,
  TerminalIcon,
  XCircleIcon,
  ClockIcon,
  GitCommitIcon,
} from '@phosphor-icons/react';
import type { WorkflowTaskDto } from 'shared/types';
import type { LiveEvent } from '@/hooks/useWorkflowLiveStatus';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface WorkflowProgressViewProps {
  readonly workflowStatus: string | null;
  readonly tasks: ReadonlyArray<WorkflowTaskDto>;
  readonly events: ReadonlyArray<LiveEvent>;
  readonly isEventsExpanded: boolean;
  readonly onToggleEvents: () => void;
  readonly onOpenBoard: () => void;
  readonly connectionStatus: string;
  readonly isLoading: boolean;
  readonly t: (key: string, options?: Record<string, string>) => string;
}

// ---------------------------------------------------------------------------
// Helpers (pure functions, no hooks)
// ---------------------------------------------------------------------------

function statusColor(status: string): string {
  switch (status) {
    case 'running':
    case 'working':
      return 'text-success';
    case 'completed':
    case 'review_pass':
      return 'text-success';
    case 'failed':
    case 'review_reject':
      return 'text-error';
    case 'pending':
    case 'not_started':
    case 'created':
      return 'text-brand';
    default:
      return 'text-low';
  }
}

function statusDot(status: string): string {
  const color = statusColor(status);
  const isAnimated = status === 'running' || status === 'working';
  return `inline-block size-2 rounded-full ${color.replace('text-', 'bg-')}${isAnimated ? ' animate-pulse' : ''}`;
}

function StatusIcon({ status }: Readonly<{ status: string }>) {
  const cls = `size-icon-sm ${statusColor(status)}`;
  if (status === 'running' || status === 'working' || status === 'starting') {
    return <CircleNotchIcon className={`${cls} animate-spin`} />;
  }
  if (status === 'completed' || status === 'review_pass') {
    return <CheckCircleIcon className={cls} />;
  }
  if (status === 'failed' || status === 'review_reject') {
    return <XCircleIcon className={cls} />;
  }
  return <ClockIcon className={cls} />;
}

function eventIcon(type: LiveEvent['type']): React.ReactNode {
  const cls = 'size-icon-xs text-low shrink-0';
  if (type === 'git_commit') return <GitCommitIcon className={cls} />;
  if (type === 'task_status') return <CheckCircleIcon className={cls} />;
  if (type === 'terminal_status') return <TerminalIcon className={cls} />;
  return <CircleNotchIcon className={cls} />;
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  } catch {
    return '';
  }
}

// ---------------------------------------------------------------------------
// Component (stateless view)
// ---------------------------------------------------------------------------

export function WorkflowProgressView({
  workflowStatus,
  tasks,
  events,
  isEventsExpanded,
  onToggleEvents,
  onOpenBoard,
  connectionStatus,
  isLoading,
  t,
}: WorkflowProgressViewProps) {
  if (isLoading) {
    return (
      <div className="rounded bg-secondary p-base text-sm text-low animate-pulse">
        {t('conversation.planning.progress.noTasks')}
      </div>
    );
  }

  const isConnected = connectionStatus === 'connected';

  return (
    <div className="rounded border bg-secondary overflow-hidden">
      {/* Header: status + open board */}
      <div className="flex items-center gap-half px-base py-half border-b">
        <span className="text-sm font-medium text-high">
          {t('conversation.planning.progress.title')}
        </span>

        {workflowStatus && (
          <span className="flex items-center gap-1">
            <StatusIcon status={workflowStatus} />
            <span className={`text-xs ${statusColor(workflowStatus)}`}>
              {workflowStatus}
            </span>
          </span>
        )}

        <span className="ml-auto flex items-center gap-half">
          <span
            className={`size-1.5 rounded-full ${isConnected ? 'bg-success' : 'bg-error animate-pulse'}`}
            title={isConnected
              ? t('conversation.planning.progress.connected')
              : t('conversation.planning.progress.reconnecting')}
          />
          <button
            type="button"
            onClick={onOpenBoard}
            className="text-xs px-base py-half rounded bg-brand text-white hover:bg-brand/90"
          >
            {t('conversation.planning.progress.openBoard')}
          </button>
        </span>
      </div>

      {/* Task list */}
      <div className="px-base py-half space-y-1">
        {tasks.length === 0 && (
          <p className="text-xs text-low py-half">
            {t('conversation.planning.progress.noTasks')}
          </p>
        )}
        {tasks.map((task) => (
          <div
            key={task.id}
            className="flex items-center gap-half text-xs"
          >
            <span className={statusDot(task.status)} />
            <span className="text-normal truncate flex-1" title={task.name}>
              {task.name}
            </span>
            <span className="flex items-center gap-px text-low shrink-0">
              <GitBranchIcon className="size-icon-xs" />
              <span className="truncate max-w-[120px]" title={task.branch}>
                {task.branch}
              </span>
            </span>
            <span className="flex items-center gap-px text-low shrink-0">
              <TerminalIcon className="size-icon-xs" />
              <span>{(task.terminals ?? []).length}</span>
            </span>
          </div>
        ))}
      </div>

      {/* Events (collapsed by default) */}
      <div className="border-t">
        <button
          type="button"
          onClick={onToggleEvents}
          className="w-full flex items-center gap-half px-base py-half text-xs text-low hover:text-normal"
        >
          <CaretRightIcon
            className={`size-icon-xs transition-transform ${isEventsExpanded ? 'rotate-90' : ''}`}
          />
          <span>{t('conversation.planning.progress.events')}</span>
          {events.length > 0 && (
            <span className="ml-1 text-low">({events.length})</span>
          )}
        </button>

        {isEventsExpanded && (
          <div className="px-base pb-half space-y-px max-h-[160px] overflow-y-auto">
            {events.length === 0 && (
              <p className="text-xs text-low py-half">--</p>
            )}
            {events.map((evt) => (
              <div
                key={evt.id}
                className="flex items-start gap-half text-xs"
              >
                {eventIcon(evt.type)}
                <span className="text-low shrink-0 tabular-nums">
                  {formatTime(evt.timestamp)}
                </span>
                <span className="text-normal truncate">{evt.summary}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
