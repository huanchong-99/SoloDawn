import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { CaretDown, CaretUp } from '@phosphor-icons/react';
import { useTranslation } from 'react-i18next';
import { useWorkflow } from '@/hooks/useWorkflows';
import { useRecentTerminalOutput } from '@/stores/terminalStore';
import { cn } from '@/lib/utils';

interface TerminalActivityPanelProps {
  readonly workflowId: string | null;
}

interface ActivityItem {
  readonly id: string;
  readonly taskId: string;
  readonly label: string;
  readonly status: string;
  readonly orderIndex: number;
  readonly lastActivity?: Date | null;
}

/** Status values that indicate active terminals */
const ACTIVE_STATUSES = new Set(['working', 'waiting', 'running', 'starting']);

/**
 * Format relative time for last activity
 */
function useFormatRelativeTime() {
  const { t } = useTranslation('workflow');

  return (date: Date | null | undefined): string => {
    if (!date) return '';
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffSec = Math.floor(diffMs / 1000);

    if (diffSec < 60) return t('terminalActivity.timeAgo.seconds', { count: diffSec });
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return t('terminalActivity.timeAgo.minutes', { count: diffMin });
    const diffHour = Math.floor(diffMin / 60);
    return t('terminalActivity.timeAgo.hours', { count: diffHour });
  };
}

/**
 * Single terminal activity item with recent output preview
 */
function TerminalActivityItem({
  item,
  workflowId
}: Readonly<{
  item: ActivityItem;
  workflowId: string;
}>) {
  const recentOutput = useRecentTerminalOutput(item.id, 3);
  const formatRelativeTime = useFormatRelativeTime();

  return (
    <Link
      to={`/debug/${workflowId}`}
      className="block rounded border border-border px-2 py-1 text-left transition-colors hover:bg-secondary"
    >
      <div className="flex items-center gap-2 text-xs">
        <StatusIndicator status={item.status} />
        <span className="font-medium text-foreground">[T{item.orderIndex + 1}]</span>
        <span className="min-w-0 flex-1 truncate">{item.label}</span>
        {item.lastActivity && (
          <span className="text-low text-[10px]">{formatRelativeTime(item.lastActivity)}</span>
        )}
      </div>
      {recentOutput.length > 0 && (
        <div className="mt-1 pl-4 text-[10px] font-mono text-low max-h-12 overflow-hidden">
          {recentOutput.slice(-3).map((line) => (
            <div key={`${item.id}-output-${line}`} className="truncate">{line || '\u00A0'}</div>
          ))}
        </div>
      )}
    </Link>
  );
}

/**
 * Status indicator dot with animation for active states
 */
function StatusIndicator({ status }: Readonly<{ status: string }>) {
  const { t } = useTranslation('workflow');
  const statusKey = `terminalDebug.status.${status}` as const;

  return (
    <span
      className={cn(
        'inline-block w-2 h-2 rounded-full',
        (() => {
          if (status === 'working' || status === 'running') return 'bg-green-500 animate-pulse';
          if (status === 'waiting') return 'bg-blue-500';
          if (status === 'starting') return 'bg-yellow-500';
          return 'bg-gray-400';
        })()
      )}
      title={t(statusKey, { defaultValue: status })}
    />
  );
}

export function TerminalActivityPanel({ workflowId }: Readonly<TerminalActivityPanelProps>) {
  const { t } = useTranslation('workflow');
  const [isCollapsed, setIsCollapsed] = useState(false);
  const { data: workflow, isLoading } = useWorkflow(workflowId ?? '');
  const workflowTasks = useMemo(() => workflow?.tasks ?? [], [workflow?.tasks]);
  const totalTerminalCount = workflowTasks.reduce(
    (count, task) => count + task.terminals.length,
    0
  );

  // Filter to only show active terminals (working/waiting)
  const activityItems = useMemo<ActivityItem[]>(() => {
    if (!workflow) return [];

    return workflowTasks.flatMap((task) =>
      task.terminals
        .filter((terminal) => ACTIVE_STATUSES.has(terminal.status))
        .map((terminal) => ({
          id: terminal.id,
          taskId: task.id,
          label: terminal.role?.trim() || task.name || t('terminalActivity.defaultLabel'),
          status: terminal.status,
          orderIndex: terminal.orderIndex,
          lastActivity: null, // Will be populated from terminalStore
        }))
    );
  }, [workflow, workflowTasks, t]);

  const toggleCollapse = () => setIsCollapsed(!isCollapsed);

  return (
    <div className={cn(
      'bg-panel border-t border-border transition-all',
      isCollapsed ? 'h-10' : 'h-auto min-h-[8rem]'
    )}>
      {/* Header with collapse toggle */}
      <button
        onClick={toggleCollapse}
        className="w-full px-4 py-2 flex items-center justify-between text-sm font-semibold hover:bg-secondary/50 transition-colors"
      >
        <span>
          {t('terminalActivity.title')}
          {activityItems.length > 0 && (
            <span className="ml-2 text-xs font-normal text-low">
              ({t('terminalActivity.active', { count: activityItems.length })})
            </span>
          )}
        </span>
        {isCollapsed ? (
          <CaretDown className="w-4 h-4 text-low" />
        ) : (
          <CaretUp className="w-4 h-4 text-low" />
        )}
      </button>

      {/* Content */}
      {!isCollapsed && (
        <div className="px-4 pb-3">
          {!workflowId && (
            <div className="text-xs text-low">
              {t('terminalActivity.selectWorkflow')}
            </div>
          )}
          {workflowId && isLoading && (
            <div className="text-xs text-low">{t('terminalActivity.loading')}</div>
          )}
          {workflowId && !isLoading && activityItems.length === 0 && (
            <div className="text-xs text-low">
              {totalTerminalCount === 0
                ? t('terminalActivity.noTerminalsYet')
                : t('terminalActivity.noActive')}
            </div>
          )}
          {workflowId && !isLoading && activityItems.length > 0 && (
            <div className="space-y-1">
              {activityItems.map((item) => (
                <TerminalActivityItem
                  key={item.id}
                  item={item}
                  workflowId={workflowId}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
