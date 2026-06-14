import { useTranslation } from 'react-i18next';
import { useWorkflow } from '@/hooks/useWorkflows';
import { useWsStore } from '@/stores/wsStore';

interface StatusBarProps {
  readonly workflowId: string | null;
}

export function StatusBar({ workflowId }: Readonly<StatusBarProps>) {
  const { t } = useTranslation('common');
  const { data: workflow } = useWorkflow(workflowId ?? '');
  const connectionStatus = useWsStore((state) =>
    workflowId
      ? state.getWorkflowConnectionStatus(workflowId)
      : state.connectionStatus
  );

  // Count active terminals across all tasks (backend uses 'working' status)
  const runningTerminalsCount =
    (workflow?.tasks ?? []).reduce((count, task) => {
      return count + task.terminals.filter(
        (terminal) => terminal.status === 'working'
      ).length;
    }, 0) ?? 0;

  // Map connection status to display text
  const gitStatusText: Record<typeof connectionStatus, string> = {
    connected: t('statusBar.gitListening'),
    connecting: t('statusBar.gitConnecting'),
    reconnecting: t('statusBar.gitReconnecting'),
    disconnected: t('statusBar.gitDisconnected'),
  };

  // Determine orchestrator status based on workflow
  const orchestratorStatus = workflow?.orchestratorEnabled
    ? t('statusBar.active')
    : t('statusBar.inactive');

  return (
    <div className="h-8 bg-panel border-t border-border px-4 flex items-center text-xs">
      <span className={workflow?.orchestratorEnabled ? 'text-brand' : 'text-low'}>
        {t('statusBar.orchestrator')} {orchestratorStatus}
      </span>
      <span className="ml-4">{t('statusBar.terminalsRunning', { count: runningTerminalsCount })}</span>
      <span className="ml-4">{t('statusBar.git')} {gitStatusText[connectionStatus]}</span>
    </div>
  );
}
