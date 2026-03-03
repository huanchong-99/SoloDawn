import { useParams, Link } from 'react-router-dom';
import {
  useWorkflow,
  useStartWorkflow,
  usePauseWorkflow,
  useStopWorkflow,
} from '@/hooks/useWorkflows';
import { TerminalDebugView } from '@/components/terminal/TerminalDebugView';
import { PipelineView } from '@/components/workflow/PipelineView';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Button } from '@/components/ui/button';
import {
  ArrowLeft,
  Play,
  Pause,
  Square,
  Activity,
  GitBranch,
  Terminal as TerminalIcon,
} from 'lucide-react';
import type { WorkflowTask } from '@/components/workflow/PipelineView';
import type { Terminal } from '@/components/workflow/TerminalCard';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';

function mapWorkflowStatus(
  status: string
): 'idle' | 'running' | 'paused' | 'completed' | 'failed' {
  switch (status) {
    case 'created':
    case 'starting':
    case 'ready':
      return 'idle';
    case 'running':
    case 'merging':
      return 'running';
    case 'paused':
      return 'paused';
    case 'completed':
      return 'completed';
    case 'failed':
      return 'failed';
    default:
      return 'idle';
  }
}

function mapTerminalStatus(status: string): Terminal['status'] {
  switch (status) {
    case 'idle':
    case 'not_started':
      return 'not_started';
    case 'starting':
      return 'starting';
    case 'waiting':
      return 'waiting';
    case 'running':
    case 'working':
      return 'working';
    case 'completed':
      return 'completed';
    case 'failed':
      return 'failed';
    default:
      return 'not_started';
  }
}

export function buildWorkflowDebugWsUrl(
  locationLike: Pick<Location, 'protocol' | 'host'>
): string {
  const wsProtocol = locationLike.protocol === 'https:' ? 'wss' : 'ws';
  return `${wsProtocol}://${locationLike.host}/api`;
}

/**
 * Status indicator with color coding
 */
function StatusBadge({ status }: Readonly<{ status: string }>) {
  const { t } = useTranslation('workflow');

  const statusColors: Record<string, string> = {
    ready: 'bg-blue-500/20 text-blue-600 border-blue-500/30',
    running: 'bg-green-500/20 text-green-600 border-green-500/30',
    merging: 'bg-green-500/20 text-green-600 border-green-500/30',
    paused: 'bg-yellow-500/20 text-yellow-600 border-yellow-500/30',
    completed: 'bg-gray-500/20 text-gray-600 border-gray-500/30',
    failed: 'bg-red-500/20 text-red-600 border-red-500/30',
  };

  return (
    <span
      className={cn(
        'px-2 py-0.5 rounded-full text-xs font-medium border',
        statusColors[status] ??
          'bg-gray-500/20 text-gray-600 border-gray-500/30'
      )}
    >
      {t(`status.${status}`, { defaultValue: status })}
    </span>
  );
}

export function WorkflowDebugPage() {
  const { t } = useTranslation('workflow');
  const { workflowId } = useParams<{ workflowId: string }>();
  const { data, isLoading, error } = useWorkflow(workflowId ?? '', {
    staleTime: 0,
    refetchInterval: 2000,
    retry: false,
  });

  // Workflow control hooks
  const startMutation = useStartWorkflow();
  const pauseMutation = usePauseWorkflow();
  const stopMutation = useStopWorkflow();

  const handleStart = () => {
    if (workflowId) {
      startMutation.mutate({ workflow_id: workflowId });
    }
  };

  const handlePause = () => {
    if (workflowId) {
      pauseMutation.mutate({ workflow_id: workflowId });
    }
  };

  const handleStop = () => {
    if (workflowId && confirm(t('workflowDebug.confirmStop'))) {
      stopMutation.mutate({ workflow_id: workflowId });
    }
  };

  if (error || (!isLoading && !data)) {
    return (
      <div className="p-8 text-center text-red-500">
        {error instanceof Error ? error.message : t('workflowDebug.error')}
      </div>
    );
  }

  if (isLoading || !data) {
    return <div className="p-8 text-center">{t('workflowDebug.loading')}</div>;
  }

  const wsUrl = buildWorkflowDebugWsUrl(globalThis.location);
  const defaultRoleLabel = t('terminalCard.defaultRole');

  // Map DTO tasks to internal WorkflowTask format
  // NOTE: terminals are embedded within each task in the DTO (not at root level)
  const tasks: WorkflowTask[] = data.tasks.map((taskDto) => ({
    id: taskDto.id,
    name: taskDto.name,
    branch: taskDto.branch ?? null,
    terminals: taskDto.terminals.map(
      (termDto): Terminal => ({
        id: termDto.id,
        cliTypeId: termDto.cliTypeId,
        modelConfigId: termDto.modelConfigId,
        role: termDto.role?.trim()
          ? termDto.role
          : `${defaultRoleLabel} ${termDto.orderIndex + 1}`,
        orderIndex: termDto.orderIndex,
        status: mapTerminalStatus(termDto.status),
      })
    ),
  }));

  const pipelineStatus = mapWorkflowStatus(data.status);

  // Calculate statistics for status bar
  const totalTasks = tasks.length;
  const totalTerminals = tasks.reduce(
    (sum, task) => sum + task.terminals.length,
    0
  );
  const runningTerminals = tasks.reduce(
    (sum, task) =>
      sum + task.terminals.filter((t) => t.status === 'working').length,
    0
  );
  const completedTerminals = tasks.reduce(
    (sum, task) =>
      sum + task.terminals.filter((t) => t.status === 'completed').length,
    0
  );

  const isReadyToStart = data.status === 'ready' || data.status === 'created';

  return (
    <div className="h-screen flex flex-col">
      <header className="border-b p-4 flex items-center justify-between">
        <div className="flex items-center gap-4">
          <Link to="/workflows">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="w-4 h-4 mr-2" /> {t('workflowDebug.back')}
            </Button>
          </Link>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="font-semibold">{data.name}</h1>
              <StatusBadge status={data.status} />
            </div>
            <p className="text-sm text-muted-foreground flex items-center gap-2 mt-1">
              <GitBranch className="w-3 h-3" />
              {data.targetBranch}
            </p>
          </div>
        </div>
        <div className="flex gap-2">
          {isReadyToStart && (
            <Button
              size="sm"
              onClick={handleStart}
              disabled={startMutation.isPending}
              className="bg-green-600 hover:bg-green-700"
            >
              <Play className="w-4 h-4 mr-2" /> {t('workflowDebug.start')}
            </Button>
          )}
          {data.status === 'running' && (
            <>
              <Button
                variant="outline"
                size="sm"
                onClick={handlePause}
                disabled={pauseMutation.isPending}
              >
                <Pause className="w-4 h-4 mr-2" /> {t('workflowDebug.pause')}
              </Button>
              <Button
                variant="destructive"
                size="sm"
                onClick={handleStop}
                disabled={stopMutation.isPending}
              >
                <Square className="w-4 h-4 mr-2" /> {t('workflowDebug.stop')}
              </Button>
            </>
          )}
        </div>
      </header>

      {/* Quick start banner for ready workflows */}
      {isReadyToStart && (
        <div className="bg-blue-500/10 border-b border-blue-500/20 px-4 py-3 flex items-center justify-between">
          <div className="flex items-center gap-2 text-sm">
            <Activity className="w-4 h-4 text-blue-500" />
            <span>
              {t('workflowDebug.readyToStart', {
                defaultValue:
                  'Workflow is ready to start. Click the button to begin execution.',
              })}
            </span>
          </div>
          <Button
            size="sm"
            onClick={handleStart}
            disabled={startMutation.isPending}
          >
            <Play className="w-4 h-4 mr-2" />{' '}
            {t('workflowDebug.startNow', { defaultValue: 'Start Now' })}
          </Button>
        </div>
      )}

      <div className="flex-1 overflow-hidden">
        <Tabs defaultValue="pipeline" className="h-full flex flex-col">
          <TabsList className="mx-4 mt-4">
            <TabsTrigger value="pipeline">
              {t('workflowDebug.tabs.pipeline')}
            </TabsTrigger>
            <TabsTrigger value="terminals">
              {t('workflowDebug.tabs.terminals')}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="pipeline" className="flex-1 p-4 overflow-auto">
            <PipelineView
              name={data.name}
              status={pipelineStatus}
              tasks={tasks}
              mergeTerminal={{
                cliTypeId: data.mergeTerminalCliId ?? '',
                modelConfigId: data.mergeTerminalModelId ?? '',
                status: 'not_started' as const,
              }}
            />
          </TabsContent>

          <TabsContent value="terminals" className="flex-1 overflow-hidden">
            <TerminalDebugView tasks={tasks} wsUrl={wsUrl} />
          </TabsContent>
        </Tabs>
      </div>

      {/* Bottom status bar */}
      <footer className="border-t bg-panel px-4 py-2 flex items-center justify-between text-xs">
        <div className="flex items-center gap-4">
          <span className="flex items-center gap-1">
            <Activity className="w-3 h-3" />
            {t('workflowDebug.statusBar.tasks', {
              count: totalTasks,
              defaultValue: '{{count}} tasks',
            })}
          </span>
          <span className="flex items-center gap-1">
            <TerminalIcon className="w-3 h-3" />
            {t('workflowDebug.statusBar.terminals', {
              running: runningTerminals,
              total: totalTerminals,
              defaultValue: '{{running}}/{{total}} terminals running',
            })}
          </span>
          <span className="flex items-center gap-1 text-green-600">
            {t('workflowDebug.statusBar.completed', {
              count: completedTerminals,
              defaultValue: '{{count}} completed',
            })}
          </span>
        </div>
        <div className="text-muted-foreground">
          {data.orchestratorEnabled
            ? t('workflowDebug.statusBar.orchestratorActive', {
                defaultValue: 'Orchestrator: Active',
              })
            : t('workflowDebug.statusBar.orchestratorInactive', {
                defaultValue: 'Orchestrator: Inactive',
              })}
        </div>
      </footer>
    </div>
  );
}
