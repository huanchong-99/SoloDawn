import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Plus,
  Play,
  Pause,
  Square,
  Trash2,
  Rocket,
  GitMerge,
} from 'lucide-react';
import { Loader } from '@/components/ui/loader';
import {
  useWorkflows,
  useCreateWorkflow,
  usePrepareWorkflow,
  useStartWorkflow,
  usePauseWorkflow,
  useStopWorkflow,
  useMergeWorkflow,
  useDeleteWorkflow,
  useWorkflow,
  workflowKeys,
  getWorkflowActions,
  type WorkflowStatusEnum,
  useOrchestratorMessages,
  useSubmitOrchestratorChat,
  useSubmitWorkflowPromptResponse,
  type OrchestratorChatMessage,
} from '@/hooks/useWorkflows';
import { useProjects } from '@/hooks/useProjects';
import type {
  WorkflowDetailDto,
  WorkflowListItemDto,
  WorkflowTaskDto,
} from 'shared/types';
import { WorkflowWizard } from '@/components/workflow/WorkflowWizard';
import {
  PipelineView,
  type WorkflowStatus,
  type WorkflowTask,
} from '@/components/workflow/PipelineView';
import { WizardConfig, wizardConfigToCreateRequest } from '@/components/workflow/types';
import type { TerminalStatus } from '@/components/workflow/TerminalCard';
import { cn } from '@/lib/utils';
import { ConfirmDialog } from '@/components/ui-new/dialogs/ConfirmDialog';
import { CreateProjectDialog } from '@/components/ui-new/dialogs/CreateProjectDialog';
import { useTranslation } from 'react-i18next';
import { projectsApi } from '@/lib/api';
import {
  type TerminalPromptResponsePayload,
  type TerminalPromptDecisionPayload,
  type TerminalPromptDetectedPayload,
  useWsStore,
  useWorkflowEvents,
} from '@/stores/wsStore';
import {
  ENTER_CONFIRM_RESPONSE_TOKEN,
  WorkflowPromptDialog,
} from '@/components/workflow/WorkflowPromptDialog';
import { useToast } from '@/components/ui/toast';
import { useUserSystem } from '@/components/ConfigProvider';

interface WorkflowPromptQueueItem {
  id: string;
  detected: TerminalPromptDetectedPayload;
  decision: TerminalPromptDecisionPayload | null;
}

const PROMPT_DUPLICATE_WINDOW_MS = 1500;
const PROMPT_HISTORY_TTL_MS = 60_000;

function getPromptContextKey(
  payload:
    | Pick<TerminalPromptDetectedPayload, 'workflowId' | 'terminalId' | 'sessionId'>
    | Pick<TerminalPromptDecisionPayload, 'workflowId' | 'terminalId' | 'sessionId'>
): string {
  return [payload.workflowId, payload.terminalId, payload.sessionId ?? ''].join(':');
}

function getPromptQueueItemId(payload: TerminalPromptDetectedPayload): string {
  const optionsHash = payload.options.join('|');
  return [
    getPromptContextKey(payload),
    payload.promptKind,
    payload.promptText,
    optionsHash,
  ].join('::');
}

function cleanupPromptHistory(history: Map<string, number>, now: number): void {
  for (const [key, timestamp] of history.entries()) {
    if (now - timestamp > PROMPT_HISTORY_TTL_MS) {
      history.delete(key);
    }
  }
}

function isSamePromptContext(
  prompt: TerminalPromptDetectedPayload,
  decision: TerminalPromptDecisionPayload
): boolean {
  if (prompt.workflowId !== decision.workflowId) {
    return false;
  }
  if (prompt.terminalId !== decision.terminalId) {
    return false;
  }
  if (prompt.sessionId && decision.sessionId) {
    return prompt.sessionId === decision.sessionId;
  }
  return true;
}

function getExecutionModeLabel(
  mode: string | undefined,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  if (mode === 'agent_planned') {
    return t('management.mode.agent_planned');
  }

  return t('management.mode.diy');
}

// Helper to resolve project ID from working directory path
async function resolveProjectIdFromPath(
  workingDir: string,
  fallbackProjectId: string | null
): Promise<string | null> {
  try {
    const resolveResponse = await fetch('/api/projects/resolve-by-path', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: workingDir }),
    });

    if (!resolveResponse.ok) {
      throw new Error('Failed to resolve project from path');
    }

    const resolveData = await resolveResponse.json();
    if (!resolveData.success || !resolveData.data?.projectId) {
      throw new Error(resolveData.message || 'Failed to resolve project');
    }

    return resolveData.data.projectId;
  } catch (resolveError) {
    if (!fallbackProjectId) {
      throw resolveError;
    }
    console.warn(
      'Failed to resolve project from path, using selected project fallback:',
      resolveError
    );
    return fallbackProjectId;
  }
}

// Helper component for workflow detail action buttons
function WorkflowDetailActions({
  workflowId,
  actions,
  hasCompletedAllTasks,
  mutations,
  handlers,
}: Readonly<{
  workflowId: string;
  actions: ReturnType<typeof getWorkflowActions>;
  hasCompletedAllTasks: boolean;
  mutations: {
    preparePending: boolean;
    startPending: boolean;
    pausePending: boolean;
    stopPending: boolean;
    mergePending: boolean;
  };
  handlers: {
    onPrepare: (id: string) => void;
    onStart: (id: string) => void;
    onPause: (id: string) => void;
    onStop: (id: string) => void;
    onMerge: (id: string) => void;
    onDelete: (id: string) => void;
  };
}>) {
  const { t } = useTranslation('workflow');

  return (
    <div className="flex gap-2">
      {actions.canPrepare && (
        <Button onClick={() => handlers.onPrepare(workflowId)} disabled={mutations.preparePending}>
          <Rocket className="w-4 h-4 mr-2" />
          {mutations.preparePending
            ? t('management.actions.preparing')
            : t('management.actions.prepare')}
        </Button>
      )}
      {actions.canStart && (
        <Button onClick={() => handlers.onStart(workflowId)} disabled={mutations.startPending}>
          <Play className="w-4 h-4 mr-2" />
          {t('management.actions.start')}
        </Button>
      )}
      {actions.canPause && (
        <Button variant="outline" onClick={() => handlers.onPause(workflowId)} disabled={mutations.pausePending}>
          <Pause className="w-4 h-4 mr-2" />
          {t('management.actions.pause')}
        </Button>
      )}
      {actions.canStop && (
        <Button variant="destructive" onClick={() => handlers.onStop(workflowId)} disabled={mutations.stopPending}>
          <Square className="w-4 h-4 mr-2" />
          {t('management.actions.stop')}
        </Button>
      )}
      {actions.canMerge && (
        <Button onClick={() => handlers.onMerge(workflowId)} disabled={!hasCompletedAllTasks || mutations.mergePending}>
          <GitMerge className="w-4 h-4 mr-2" />
          {mutations.mergePending
            ? t('management.actions.merging')
            : t('management.actions.merge')}
        </Button>
      )}
      {actions.canDelete && (
        <Button variant="outline" onClick={() => handlers.onDelete(workflowId)}>
          <Trash2 className="w-4 h-4 mr-2" />
          {t('management.actions.delete')}
        </Button>
      )}
    </div>
  );
}

function renderBlockingView({
  projectsLoading,
  projectsErrorMessage,
  projectCount,
  workflowsLoading,
  workflowsErrorMessage,
  loadFailedText,
  loadingProjectsText,
  noProjectsTitleText,
  noProjectsDescriptionText,
  loadingWorkflowsText,
}: Readonly<{
  projectsLoading: boolean;
  projectsErrorMessage: string | null;
  projectCount: number;
  workflowsLoading: boolean;
  workflowsErrorMessage: string | null;
  loadFailedText: string;
  loadingProjectsText: string;
  noProjectsTitleText: string;
  noProjectsDescriptionText: string;
  loadingWorkflowsText: string;
}>): JSX.Element | null {
  if (projectsLoading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Loader message={loadingProjectsText} />
      </div>
    );
  }

  if (projectsErrorMessage) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Card className="max-w-md">
          <CardContent className="pt-6">
            <p className="text-error mb-4">{loadFailedText}</p>
            <p className="text-sm text-low">{projectsErrorMessage}</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (projectCount === 0) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Card className="max-w-md">
          <CardContent className="pt-6">
            <h3 className="text-lg font-semibold mb-2">{noProjectsTitleText}</h3>
            <p className="text-sm text-low">{noProjectsDescriptionText}</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (workflowsLoading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Loader message={loadingWorkflowsText} />
      </div>
    );
  }

  if (workflowsErrorMessage) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Card className="max-w-md">
          <CardContent className="pt-6">
            <p className="text-error mb-4">{loadFailedText}</p>
            <p className="text-sm text-low">{workflowsErrorMessage}</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  return null;
}

const WORKFLOW_STATUS_MAP: Record<string, WorkflowStatus> = {
  created: 'idle',
  starting: 'running',
  ready: 'idle',
  running: 'running',
  paused: 'paused',
  merging: 'running',
  completed: 'completed',
  failed: 'failed',
  cancelled: 'idle',
  draft: 'idle',
};

const WORKFLOW_STATUS_BADGE_CLASSES: Record<string, string> = {
  created: 'bg-gray-100 text-gray-800',
  ready: 'bg-gray-100 text-gray-800',
  draft: 'bg-gray-100 text-gray-800',
  starting: 'bg-blue-100 text-blue-800',
  running: 'bg-blue-100 text-blue-800',
  merging: 'bg-blue-100 text-blue-800',
  paused: 'bg-yellow-100 text-yellow-800',
  completed: 'bg-green-100 text-green-800',
  failed: 'bg-red-100 text-red-800',
  cancelled: 'bg-zinc-100 text-zinc-800',
};

function mapWorkflowStatus(status: string): WorkflowStatus {
  return WORKFLOW_STATUS_MAP[status] ?? 'idle';
}

function getWorkflowStatusBadgeClass(status: string): string {
  return WORKFLOW_STATUS_BADGE_CLASSES[status] ?? 'bg-gray-100 text-gray-800';
}

function mapMergeTerminalStatus(status: string): TerminalStatus {
  switch (status) {
    case 'merging':
      return 'working';
    case 'completed':
      return 'completed';
    case 'failed':
      return 'failed';
    case 'cancelled':
      return 'cancelled';
    default:
      return 'not_started';
  }
}

function mapWorkflowTasks(
  dtoTasks: WorkflowTaskDto[] | undefined | null
): WorkflowTask[] {
  return [...(dtoTasks ?? [])]
    .sort((a, b) => a.orderIndex - b.orderIndex)
    .map((task) => ({
      id: task.id,
      name: task.name,
      branch: task.branch,
      terminals: [...(task.terminals ?? [])]
        .sort((a, b) => a.orderIndex - b.orderIndex)
        .map((terminal) => ({
          id: terminal.id,
          workflowTaskId: task.id,
          cliTypeId: terminal.cliTypeId,
          modelConfigId: terminal.modelConfigId,
          role: terminal.role || undefined,
          orderIndex: terminal.orderIndex,
          status: terminal.status as TerminalStatus,
        })),
    }));
}

function mapOrchestratorMessageRole(
  role: OrchestratorChatMessage['role'],
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  if (role === 'assistant') {
    return t('management.orchestratorChat.roles.assistant', {
      defaultValue: 'Agent',
    });
  }

  if (role === 'user') {
    return t('management.orchestratorChat.roles.user', {
      defaultValue: 'You',
    });
  }

  if (role === 'system') {
    return t('management.orchestratorChat.roles.system', {
      defaultValue: 'System',
    });
  }

  if (role === 'tool-summary') {
    return t('management.orchestratorChat.roles.summary', {
      defaultValue: 'Execution Summary',
    });
  }

  return role;
}

function hasConfiguredWorkflowModelLibrary(config: unknown): boolean {
  const rawLibrary =
    (config as {
      workflow_model_library?: unknown;
      workflowModelLibrary?: unknown;
    } | null)?.workflow_model_library ??
    (config as { workflowModelLibrary?: unknown } | null)
      ?.workflowModelLibrary;
  if (!Array.isArray(rawLibrary)) {
    return false;
  }

  return rawLibrary.some((item) => {
    if (typeof item !== 'object' || item === null) {
      return false;
    }
    const candidate = item as Record<string, unknown>;
    return (
      typeof candidate.modelId === 'string' &&
      candidate.modelId.trim().length > 0
    );
  });
}

function OrchestratorChatPanel({
  workflowId,
  workflowStatus,
  orchestratorEnabled,
  executionMode,
}: Readonly<{
  workflowId: string;
  workflowStatus: string;
  orchestratorEnabled: boolean;
  executionMode?: string;
}>) {
  const { t } = useTranslation('workflow');
  const { config: userConfig } = useUserSystem();
  const [messageInput, setMessageInput] = useState('');
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitReceipt, setSubmitReceipt] = useState<string | null>(null);
  const isPrimaryChannel = executionMode === 'agent_planned';

  const isRunning = workflowStatus === 'running';
  const hasConfiguredModels = useMemo(
    () => hasConfiguredWorkflowModelLibrary(userConfig),
    [userConfig]
  );
  const canSendMessage = orchestratorEnabled && isRunning && hasConfiguredModels;

  const {
    data: messages,
    isLoading,
    error,
    refetch,
  } = useOrchestratorMessages(workflowId, {
    enabled: canSendMessage,
    refetchInterval: canSendMessage ? 2000 : false,
    limit: 80,
  });
  const submitOrchestratorChatMutation = useSubmitOrchestratorChat();

  const visibleMessages = useMemo(
    () =>
      (messages ?? [])
        .filter((message) =>
          ['user', 'assistant', 'system', 'tool-summary'].includes(
            message.role.toLowerCase()
          )
        )
        .slice(-24),
    [messages]
  );

  const handleSendMessage = async () => {
    const trimmedMessage = messageInput.trim();
    if (!trimmedMessage || !canSendMessage) {
      return;
    }

    setSubmitError(null);
    setSubmitReceipt(null);
    try {
      const submitResult = await submitOrchestratorChatMutation.mutateAsync({
        workflow_id: workflowId,
        message: trimmedMessage,
        source: 'web',
      });

      if (submitResult.status === 'failed' || submitResult.status === 'cancelled') {
        setSubmitError(
          submitResult.error ??
            t('management.orchestratorChat.sendFailed', {
              defaultValue: 'Failed to send orchestrator message.',
            })
        );
        return;
      }

      setSubmitReceipt(
        t('management.orchestratorChat.commandAccepted', {
          defaultValue: `Command ${submitResult.command_id} accepted (${submitResult.status}).`,
        })
      );
      setMessageInput('');
      await refetch();
    } catch (sendError) {
      setSubmitError(
        sendError instanceof Error
          ? sendError.message
          : t('management.orchestratorChat.sendFailed', {
              defaultValue: 'Failed to send orchestrator message.',
            })
      );
    }
  };

  const hint = !hasConfiguredModels
    ? t('management.orchestratorChat.noModels', {
        defaultValue:
          'You must configure at least one AI model before using orchestrator chat.',
      })
    : !orchestratorEnabled
      ? t('management.orchestratorChat.disabled', {
          defaultValue: 'Current workflow does not have orchestrator enabled.',
        })
      : !isRunning
        ? t('management.orchestratorChat.notRunning', {
            defaultValue: 'Only running workflows support orchestrator chat.',
          })
        : null;

  return (
    <Card>
      <CardContent className="pt-6 space-y-4">
        <div>
          <div className="flex items-center gap-2">
            <h3 className="text-sm font-semibold">
              {t('management.orchestratorChat.title', {
                defaultValue: 'Orchestrator Chat',
              })}
            </h3>
            {isPrimaryChannel ? (
              <span className="rounded-full border border-blue-300 bg-blue-50 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-blue-700">
                {t('management.orchestratorChat.primaryChannel', {
                  defaultValue: 'Primary Channel',
                })}
              </span>
            ) : null}
          </div>
          <p className="text-xs text-low mt-1">
            {t('management.orchestratorChat.description', {
              defaultValue:
                'Send instructions to the orchestrator agent to intervene in task coordination.',
            })}
          </p>
        </div>

        <div className="rounded-md border bg-background/60 p-3">
          {hint ? <p className="text-xs text-low">{hint}</p> : null}

          {canSendMessage ? (
            isLoading ? (
              <p className="text-xs text-low">
                {t('management.orchestratorChat.loading', {
                  defaultValue: 'Loading conversation...',
                })}
              </p>
            ) : error ? (
              <p className="text-xs text-error">
                {error.message}
              </p>
            ) : visibleMessages.length === 0 ? (
              <p className="text-xs text-low">
                {t('management.orchestratorChat.empty', {
                  defaultValue: 'No messages yet. Send your first instruction.',
                })}
              </p>
            ) : (
              <div className="max-h-64 overflow-y-auto space-y-3">
                {visibleMessages.map((message, index) => (
                  <div
                    key={`${message.role}-${index}`}
                    className={cn(
                      'rounded border px-3 py-2',
                      message.role === 'assistant'
                        ? 'border-blue-200/60 bg-blue-50/40'
                        : message.role === 'user'
                          ? 'border-border/60 bg-panel'
                          : message.role === 'tool-summary'
                            ? 'border-amber-300/60 bg-amber-50/40'
                            : 'border-zinc-300/60 bg-zinc-50/60'
                    )}
                  >
                    <div className="text-[11px] font-medium text-low mb-1">
                      {mapOrchestratorMessageRole(message.role, t)}
                    </div>
                    <div className="text-sm whitespace-pre-wrap break-words">
                      {message.content}
                    </div>
                  </div>
                ))}
              </div>
            )
          ) : null}
        </div>

        <div className="space-y-2">
          <textarea
            value={messageInput}
            onChange={(event) => {
              setMessageInput(event.target.value);
            }}
            placeholder={t('management.orchestratorChat.placeholder', {
              defaultValue:
                'For example: reprioritize tasks and complete the auth module first.',
            })}
            className="w-full min-h-[88px] rounded-md border bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-60"
            disabled={!canSendMessage || submitOrchestratorChatMutation.isPending}
          />
          <div className="flex items-center justify-between gap-3">
            <p className="text-xs text-low">
              {t('management.orchestratorChat.note', {
                defaultValue:
                  'Messages enter orchestrator context and may trigger new scheduling actions.',
              })}
            </p>
            <Button
              onClick={() => {
                void handleSendMessage();
              }}
              disabled={
                !canSendMessage ||
                submitOrchestratorChatMutation.isPending ||
                messageInput.trim().length === 0
              }
            >
              {submitOrchestratorChatMutation.isPending
                ? t('management.orchestratorChat.sending', {
                    defaultValue: 'Sending...',
                  })
                : t('management.orchestratorChat.send', {
                    defaultValue: 'Send to Agent',
                  })}
            </Button>
          </div>
          {submitError ? <p className="text-xs text-error">{submitError}</p> : null}
          {submitReceipt ? <p className="text-xs text-low">{submitReceipt}</p> : null}
        </div>
      </CardContent>
    </Card>
  );
}

function SelectedWorkflowView({
  workflow,
  mutations,
  onBack,
  onPrepare,
  onStart,
  onPause,
  onStop,
  onMerge,
  onDelete,
  runAsyncSafely,
  promptDialog,
}: Readonly<{
  workflow: WorkflowDetailDto;
  mutations: {
    preparePending: boolean;
    startPending: boolean;
    pausePending: boolean;
    stopPending: boolean;
    mergePending: boolean;
  };
  onBack: () => void;
  onPrepare: (workflowId: string) => Promise<void>;
  onStart: (workflowId: string) => Promise<void>;
  onPause: (workflowId: string) => Promise<void>;
  onStop: (workflowId: string) => Promise<void>;
  onMerge: (workflowId: string) => Promise<void>;
  onDelete: (workflowId: string) => Promise<void>;
  runAsyncSafely: (task: Promise<unknown>) => void;
  promptDialog: ReactNode;
}>) {
  const { t } = useTranslation('workflow');
  const actions = getWorkflowActions(workflow.status as WorkflowStatusEnum);
  const workflowTasks = workflow.tasks ?? [];
  const hasCompletedAllTasks = workflowTasks.every(
    (task) => task.status === 'completed'
  );
  const canTriggerMerge =
    actions.canMerge && hasCompletedAllTasks && !mutations.mergePending;
  const executionModeLabel = getExecutionModeLabel(workflow.executionMode, t);

  return (
    <div className="h-full min-h-0 overflow-auto space-y-6">
      <div className="flex items-center justify-between">
        <Button variant="outline" onClick={onBack}>
          {`\u2190 ${t('management.backToWorkflows')}`}
        </Button>
        <WorkflowDetailActions
          workflowId={workflow.id}
          actions={actions}
          hasCompletedAllTasks={hasCompletedAllTasks}
          mutations={mutations}
          handlers={{
            onPrepare: (workflowId) => {
              runAsyncSafely(onPrepare(workflowId));
            },
            onStart: (workflowId) => {
              runAsyncSafely(onStart(workflowId));
            },
            onPause: (workflowId) => {
              runAsyncSafely(onPause(workflowId));
            },
            onStop: (workflowId) => {
              runAsyncSafely(onStop(workflowId));
            },
            onMerge: (workflowId) => {
              runAsyncSafely(onMerge(workflowId));
            },
            onDelete: (workflowId) => {
              runAsyncSafely(onDelete(workflowId));
            },
          }}
        />
      </div>

      <Card>
        <CardContent className="pt-6 space-y-4">
          <div className="flex flex-wrap items-center gap-3">
            <span className="rounded-full bg-secondary px-3 py-1 text-xs font-medium text-normal">
              {executionModeLabel}
            </span>
            <span
              className={cn(
                'rounded-full px-3 py-1 text-xs font-medium',
                getWorkflowStatusBadgeClass(workflow.status)
              )}
            >
              {t(`status.${workflow.status}`, {
                defaultValue: workflow.status,
              })}
            </span>
          </div>
          {workflow.initialGoal ? (
            <div className="space-y-1">
              <div className="text-xs font-medium uppercase tracking-wide text-low">
                {t('management.goalLabel')}
              </div>
              <div className="text-sm text-normal">{workflow.initialGoal}</div>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <OrchestratorChatPanel
        workflowId={workflow.id}
        workflowStatus={workflow.status}
        orchestratorEnabled={workflow.orchestratorEnabled}
        executionMode={workflow.executionMode}
      />

      <PipelineView
        name={workflow.name}
        status={mapWorkflowStatus(workflow.status)}
        executionMode={workflow.executionMode}
        initialGoal={workflow.initialGoal}
        tasks={mapWorkflowTasks(workflowTasks)}
        mergeTerminal={{
          cliTypeId: workflow.mergeTerminalCliId ?? '',
          modelConfigId: workflow.mergeTerminalModelId ?? '',
          status: mapMergeTerminalStatus(workflow.status),
        }}
        onTerminalClick={undefined}
        onMergeTerminalClick={
          canTriggerMerge
            ? () => {
                runAsyncSafely(onMerge(workflow.id));
              }
            : undefined
        }
      />
      {promptDialog}
    </div>
  );
}

function WorkflowListContent({
  showWizard,
  workflows,
  onOpenWizard,
  onSelectWorkflow,
}: Readonly<{
  showWizard: boolean;
  workflows: WorkflowListItemDto[] | undefined;
  onOpenWizard: () => void;
  onSelectWorkflow: (workflowId: string) => void;
}>) {
  const { t } = useTranslation('workflow');

  if (showWizard) {
    return null;
  }

  if (!workflows || workflows.length === 0) {
    return (
      <Card className="p-12 text-center">
        <h3 className="text-lg font-semibold mb-2">{t('empty.title')}</h3>
        <p className="text-low mb-6">{t('empty.description')}</p>
        <Button onClick={onOpenWizard}>
          <Plus className="w-4 h-4 mr-2" />
          {t('empty.button')}
        </Button>
      </Card>
    );
  }

  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      {workflows.map((workflow) => (
        <Card
          key={workflow.id}
          className={cn(
            'cursor-pointer transition-all hover:shadow-lg',
            'border-2 hover:border-brand'
          )}
          onClick={() => onSelectWorkflow(workflow.id)}
        >
          <CardContent className="pt-6">
            <div className="flex items-start justify-between mb-4">
              <h3 className="font-semibold text-lg">{workflow.name}</h3>
              <span
                className={cn(
                  'px-2 py-1 rounded text-xs font-medium',
                  getWorkflowStatusBadgeClass(workflow.status)
                )}
              >
                {t(`status.${workflow.status}`, {
                  defaultValue: workflow.status,
                })}
              </span>
            </div>
            {workflow.description && (
              <p className="text-sm text-low mb-4">{workflow.description}</p>
            )}
            <div className="flex flex-wrap items-center gap-2 mb-4">
              <span className="rounded-full bg-secondary px-2 py-1 text-xs font-medium text-normal">
                {getExecutionModeLabel(workflow.executionMode, t)}
              </span>
            </div>
            <div className="flex items-center justify-between text-xs text-low">
              <span>
                {t('management.counts.tasks', { count: workflow.tasksCount })}
              </span>
              <span>
                {t('management.counts.terminals', {
                  count: workflow.terminalsCount,
                })}
              </span>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

export function Workflows() {
  const { t } = useTranslation('workflow');
  const { showToast } = useToast();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const [showWizard, setShowWizard] = useState(false);
  const [selectedWorkflowId, setSelectedWorkflowId] = useState<string | null>(
    null
  );
  const [isDeletingProject, setIsDeletingProject] = useState(false);

  // Get projectId from URL query params
  const projectIdFromUrl = searchParams.get('projectId');

  // Load projects list for project selector
  const {
    projects,
    isLoading: projectsLoading,
    error: projectsError,
  } = useProjects();

  // Validate projectId exists in projects list, fallback to first project if invalid
  const validProjectId =
    projectIdFromUrl && projects.some((p) => p.id === projectIdFromUrl)
      ? projectIdFromUrl
      : projects[0]?.id ?? null;

  // Update URL when projectId is invalid or missing
  useEffect(() => {
    if (projects.length > 0 && projectIdFromUrl !== validProjectId) {
      const newParams = new URLSearchParams(searchParams);
      newParams.set('projectId', validProjectId);
      setSearchParams(newParams, { replace: true });
      setSelectedWorkflowId(null);
    }
  }, [
    projectIdFromUrl,
    validProjectId,
    projects.length,
    searchParams,
    setSearchParams,
  ]);

  const {
    data: workflows,
    isLoading,
    error,
  } = useWorkflows(validProjectId || '');
  const createMutation = useCreateWorkflow();
  const prepareMutation = usePrepareWorkflow();
  const startMutation = useStartWorkflow();
  const pauseMutation = usePauseWorkflow();
  const stopMutation = useStopWorkflow();
  const mergeMutation = useMergeWorkflow();
  const deleteMutation = useDeleteWorkflow();
  const submitPromptResponseMutation = useSubmitWorkflowPromptResponse();

  const [promptQueue, setPromptQueue] = useState<WorkflowPromptQueueItem[]>([]);
  const [submittingPromptId, setSubmittingPromptId] = useState<string | null>(
    null
  );
  const [promptSubmitError, setPromptSubmitError] = useState<string | null>(
    null
  );

  const promptDetectedHistoryRef = useRef<Map<string, number>>(new Map());
  const promptSubmittedHistoryRef = useRef<Map<string, number>>(new Map());
  const submittingPromptRef = useRef<string | null>(null);
  const pendingPromptDecisionsRef = useRef<
    Map<string, TerminalPromptDecisionPayload>
  >(new Map());
  const sendPromptResponseOverWorkflowWs = useWsStore(
    (state) => state.sendPromptResponse
  );

  useEffect(() => {
    setPromptQueue([]);
    setSubmittingPromptId(null);
    submittingPromptRef.current = null;
    setPromptSubmitError(null);
    promptDetectedHistoryRef.current.clear();
    promptSubmittedHistoryRef.current.clear();
    pendingPromptDecisionsRef.current.clear();
  }, [selectedWorkflowId]);

  const handleTerminalPromptDetected = useCallback(
    (payload: TerminalPromptDetectedPayload) => {
      const now = Date.now();
      cleanupPromptHistory(promptDetectedHistoryRef.current, now);
      cleanupPromptHistory(promptSubmittedHistoryRef.current, now);

      const promptItemId = getPromptQueueItemId(payload);
      const lastDetectedAt = promptDetectedHistoryRef.current.get(promptItemId);
      if (
        lastDetectedAt !== undefined &&
        now - lastDetectedAt < PROMPT_DUPLICATE_WINDOW_MS
      ) {
        return;
      }

      const lastSubmittedAt = promptSubmittedHistoryRef.current.get(promptItemId);
      if (
        lastSubmittedAt !== undefined &&
        now - lastSubmittedAt < PROMPT_HISTORY_TTL_MS
      ) {
        return;
      }

      const pendingDecision = pendingPromptDecisionsRef.current.get(
        getPromptContextKey(payload)
      );
      if (pendingDecision && pendingDecision.decision !== 'ask_user') {
        return;
      }

      promptDetectedHistoryRef.current.set(promptItemId, now);

      setPromptQueue((previousQueue) => {
        if (previousQueue.some((item) => item.id === promptItemId)) {
          return previousQueue;
        }

        return [
          ...previousQueue,
          {
            id: promptItemId,
            detected: payload,
            decision:
              pendingDecision?.decision === 'ask_user'
                ? pendingDecision
                : null,
          },
        ];
      });

      setPromptSubmitError(null);
    },
    []
  );

  const handleTerminalPromptDecision = useCallback(
    (payload: TerminalPromptDecisionPayload) => {
      const contextKey = getPromptContextKey(payload);

      if (payload.decision === 'ask_user') {
        pendingPromptDecisionsRef.current.set(contextKey, payload);
      } else {
        pendingPromptDecisionsRef.current.delete(contextKey);
      }

      setPromptQueue((previousQueue) => {
        if (payload.decision === 'ask_user') {
          return previousQueue.map((item) =>
            isSamePromptContext(item.detected, payload)
              ? { ...item, decision: payload }
              : item
          );
        }

        return previousQueue.filter(
          (item) => !isSamePromptContext(item.detected, payload)
        );
      });

      if (payload.decision !== 'ask_user') {
        setPromptSubmitError(null);
      }
    },
    []
  );

  const handleRealtimeWorkflowSignal = useCallback(() => {
    if (!selectedWorkflowId) {
      return;
    }

    queryClient.invalidateQueries({
      queryKey: workflowKeys.byId(selectedWorkflowId),
    });

    if (validProjectId) {
      queryClient.invalidateQueries({
        queryKey: workflowKeys.forProject(validProjectId),
      });
    }
  }, [queryClient, selectedWorkflowId, validProjectId]);

  const workflowEventHandlers = useMemo(
    () => ({
      onTerminalPromptDetected: handleTerminalPromptDetected,
      onTerminalPromptDecision: handleTerminalPromptDecision,
      onWorkflowStatusChanged: handleRealtimeWorkflowSignal,
      onTaskStatusChanged: handleRealtimeWorkflowSignal,
      onTerminalStatusChanged: handleRealtimeWorkflowSignal,
      onTerminalCompleted: handleRealtimeWorkflowSignal,
    }),
    [
      handleTerminalPromptDetected,
      handleTerminalPromptDecision,
      handleRealtimeWorkflowSignal,
    ]
  );

  useWorkflowEvents(selectedWorkflowId, workflowEventHandlers);

  const activePrompt = useMemo(() => promptQueue[0] ?? null, [promptQueue]);

  useEffect(() => {
    if (
      submittingPromptId &&
      !promptQueue.some((item) => item.id === submittingPromptId)
    ) {
      setSubmittingPromptId(null);
      if (submittingPromptRef.current === submittingPromptId) {
        submittingPromptRef.current = null;
      }
    }
  }, [promptQueue, submittingPromptId]);

  // Helper to handle successful prompt submission
  const handlePromptSubmitSuccess = useCallback(
    (promptId: string, promptContextKey: string) => {
      pendingPromptDecisionsRef.current.delete(promptContextKey);
      setPromptQueue((previousQueue) =>
        previousQueue.filter((item) => item.id !== promptId)
      );
    },
    []
  );

  // Helper to handle prompt submission error with WebSocket fallback
  const handlePromptSubmitErrorWithFallback = useCallback(
    (
      currentPrompt: WorkflowPromptQueueItem,
      isEnterConfirmResponse: boolean,
      sendPromptResponseOverWorkflowWs: (
        payload: TerminalPromptResponsePayload
      ) => boolean
    ): boolean => {
      if (!isEnterConfirmResponse) return false;

      const sent = sendPromptResponseOverWorkflowWs({
        workflowId: currentPrompt.detected.workflowId,
        terminalId: currentPrompt.detected.terminalId,
        response: '',
      });

      if (sent) {
        const promptContextKey = getPromptContextKey(currentPrompt.detected);
        handlePromptSubmitSuccess(currentPrompt.id, promptContextKey);
        return true;
      }

      promptSubmittedHistoryRef.current.delete(currentPrompt.id);
      setPromptSubmitError('Failed to submit prompt response over WebSocket');
      return true;
    },
    [handlePromptSubmitSuccess]
  );

  // Helper to handle general prompt submission error
  const handlePromptSubmitError = useCallback(
    (currentPrompt: WorkflowPromptQueueItem, error: unknown) => {
      promptSubmittedHistoryRef.current.delete(currentPrompt.id);
      const message =
        error instanceof Error && error.message
          ? error.message
          : 'Failed to submit prompt response';
      setPromptSubmitError(message);
    },
    []
  );

  const handleSubmitPromptResponse = useCallback(
    async (response: string) => {
      const currentPrompt = activePrompt;
      if (!currentPrompt) return;

      const isEnterConfirmResponse =
        response === ENTER_CONFIRM_RESPONSE_TOKEN &&
        currentPrompt.detected.promptKind === 'enter_confirm';

      const requestResponse = isEnterConfirmResponse ? '' : response;

      if (submittingPromptRef.current === currentPrompt.id) return;

      submittingPromptRef.current = currentPrompt.id;
      setSubmittingPromptId(currentPrompt.id);
      setPromptSubmitError(null);

      const now = Date.now();
      cleanupPromptHistory(promptSubmittedHistoryRef.current, now);
      promptSubmittedHistoryRef.current.set(currentPrompt.id, now);

      try {
        const promptContextKey = getPromptContextKey(currentPrompt.detected);

        await submitPromptResponseMutation.mutateAsync({
          workflow_id: currentPrompt.detected.workflowId,
          terminal_id: currentPrompt.detected.terminalId,
          response: requestResponse,
        });

        handlePromptSubmitSuccess(currentPrompt.id, promptContextKey);
      } catch (error) {
        const handled = handlePromptSubmitErrorWithFallback(
          currentPrompt,
          isEnterConfirmResponse,
          sendPromptResponseOverWorkflowWs
        );

        if (!handled) {
          handlePromptSubmitError(currentPrompt, error);
        }
      } finally {
        if (submittingPromptRef.current === currentPrompt.id) {
          submittingPromptRef.current = null;
        }
        setSubmittingPromptId((currentId) =>
          currentId === currentPrompt.id ? null : currentId
        );
      }
    },
    [
      activePrompt,
      sendPromptResponseOverWorkflowWs,
      submitPromptResponseMutation,
      handlePromptSubmitSuccess,
      handlePromptSubmitErrorWithFallback,
      handlePromptSubmitError,
    ]
  );

  const isSubmittingActivePrompt =
    !!activePrompt &&
    (submittingPromptId === activePrompt.id ||
      submitPromptResponseMutation.isPending);

  const activePromptDecision = activePrompt?.decision ?? null;

  const runAsyncSafely = useCallback((task: Promise<unknown>) => {
    task.catch((error) => {
      console.error('Async workflow action failed:', error);
    });
  }, []);

  const promptDialog = activePrompt ? (
    <WorkflowPromptDialog
      prompt={activePrompt.detected}
      decision={activePromptDecision}
      submitError={promptSubmitError}
      isSubmitting={isSubmittingActivePrompt}
      onSubmit={(response) => {
        runAsyncSafely(handleSubmitPromptResponse(response));
      }}
    />
  ) : null;

  // Fetch workflow detail when selected
  const { data: selectedWorkflowDetail } = useWorkflow(
    selectedWorkflowId || ''
  );

  // Handle project change (preserve other URL params)
  const handleProjectChange = (newProjectId: string) => {
    const newParams = new URLSearchParams(searchParams);
    newParams.set('projectId', newProjectId);
    setSearchParams(newParams, { replace: true });
    setSelectedWorkflowId(null); // Clear selected workflow when switching projects
  };

  const handleCreateProject = async () => {
    const result = await CreateProjectDialog.show({});
    if (result.status !== 'saved') {
      return;
    }

    const newParams = new URLSearchParams(searchParams);
    newParams.set('projectId', result.project.id);
    setSearchParams(newParams, { replace: true });
    setSelectedWorkflowId(null);
    showToast(`Project "${result.project.name}" created`, 'success');
  };

  const handleDeleteProject = async () => {
    if (!validProjectId) {
      return;
    }

    if (projects.length <= 1) {
      showToast('Cannot delete the last project', 'error');
      return;
    }

    const deletingProject = projects.find((project) => project.id === validProjectId);
    const result = await ConfirmDialog.show({
      title: t('management.dialogs.deleteProjectTitle'),
      message: t('management.dialogs.deleteProjectMessage', {
        name: deletingProject?.name ?? validProjectId,
      }),
      confirmText: t('management.actions.delete'),
      cancelText: t('wizard.buttons.cancel'),
      variant: 'destructive',
    });

    if (result !== 'confirmed') {
      return;
    }

    try {
      setIsDeletingProject(true);
      await projectsApi.delete(validProjectId);

      const fallbackProjectId = projects.find(
        (project) => project.id !== validProjectId
      )?.id;
      const newParams = new URLSearchParams(searchParams);

      if (fallbackProjectId) {
        newParams.set('projectId', fallbackProjectId);
      } else {
        newParams.delete('projectId');
      }

      setSearchParams(newParams, { replace: true });
      setSelectedWorkflowId(null);
      showToast(t('management.toasts.projectDeleted'), 'success');
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : t('management.toasts.deleteProjectFailed');
      showToast(message, 'error');
    } finally {
      setIsDeletingProject(false);
    }
  };

  // Get current project name for display
  const currentProject = projects.find((p) => p.id === validProjectId);

  const blockingView = renderBlockingView({
    projectsLoading,
    projectsErrorMessage: projectsError?.message ?? null,
    projectCount: projects.length,
    workflowsLoading: isLoading,
    workflowsErrorMessage: error?.message ?? null,
    loadFailedText: t('errors.loadFailed'),
    loadingProjectsText: t('management.loadingProjects'),
    noProjectsTitleText: t('management.noProjectsTitle'),
    noProjectsDescriptionText: t('management.noProjectsDescription'),
    loadingWorkflowsText: t('management.loadingWorkflows'),
  });
  if (blockingView) {
    return blockingView;
  }


  const handleCreateWorkflow = async (wizardConfig: WizardConfig) => {
    const workingDir = wizardConfig.project.workingDirectory?.trim();
    const fallbackProjectId = validProjectId;

    try {
      const projectId = workingDir
        ? await resolveProjectIdFromPath(workingDir, fallbackProjectId)
        : fallbackProjectId;

      if (!projectId) {
        throw new Error(t('management.errors.noProjectSelected'));
      }

      const request = wizardConfigToCreateRequest(projectId, wizardConfig);
      const newWorkflow = await createMutation.mutateAsync(request);

      const newParams = new URLSearchParams(searchParams);
      newParams.set('projectId', projectId);
      setSearchParams(newParams, { replace: true });
      setSelectedWorkflowId(newWorkflow.id);
      setShowWizard(false);
    } catch (error) {
      console.error('Failed to create workflow:', error);
      throw error instanceof Error
        ? error
        : new Error(t('management.errors.createWorkflowFailed'));
    }
  };

  const handlePrepareWorkflow = async (workflowId: string) => {
    await prepareMutation.mutateAsync(workflowId);
  };

  const handleStartWorkflow = async (workflowId: string) => {
    await startMutation.mutateAsync({ workflow_id: workflowId });
  };

  const handlePauseWorkflow = async (workflowId: string) => {
    await pauseMutation.mutateAsync({ workflow_id: workflowId });
  };

  const handleStopWorkflow = async (workflowId: string) => {
    const result = await ConfirmDialog.show({
      title: t('management.dialogs.stopWorkflowTitle'),
      message: t('workflowDebug.confirmStop'),
      confirmText: t('management.actions.stop'),
      cancelText: t('wizard.buttons.cancel'),
      variant: 'destructive',
    });

    if (result === 'confirmed') {
      await stopMutation.mutateAsync({ workflow_id: workflowId });
    }
  };

  const handleMergeWorkflow = async (workflowId: string) => {
    await mergeMutation.mutateAsync({
      workflow_id: workflowId,
      merge_strategy: 'squash',
    });
  };

  const handleDeleteWorkflow = async (workflowId: string) => {
    const result = await ConfirmDialog.show({
      title: t('management.dialogs.deleteWorkflowTitle'),
      message: t('errors.deleteConfirm'),
      confirmText: t('management.actions.delete'),
      cancelText: t('wizard.buttons.cancel'),
      variant: 'destructive',
    });

    if (result === 'confirmed') {
      await deleteMutation.mutateAsync(workflowId);
    }
  };

  if (selectedWorkflowDetail && selectedWorkflowId) {
    return (
      <SelectedWorkflowView
        workflow={selectedWorkflowDetail}
        mutations={{
          preparePending: prepareMutation.isPending,
          startPending: startMutation.isPending,
          pausePending: pauseMutation.isPending,
          stopPending: stopMutation.isPending,
          mergePending: mergeMutation.isPending,
        }}
        onBack={() => setSelectedWorkflowId(null)}
        onPrepare={handlePrepareWorkflow}
        onStart={handleStartWorkflow}
        onPause={handlePauseWorkflow}
        onStop={handleStopWorkflow}
        onMerge={handleMergeWorkflow}
        onDelete={handleDeleteWorkflow}
        runAsyncSafely={runAsyncSafely}
        promptDialog={promptDialog}
      />
    );
  }

  return (
    <div className="h-full min-h-0 overflow-auto space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4 flex-wrap">
          <div>
            <h1 className="text-2xl font-bold">{t('management.title')}</h1>
            <p className="text-low">
              {t('management.description')}
            </p>
          </div>
          <div className="flex items-center gap-2 flex-wrap">
            <Select
              value={validProjectId || ''}
              onValueChange={handleProjectChange}
            >
              <SelectTrigger className="w-[220px]">
                <SelectValue placeholder={t('management.selectProject')}>
                  {currentProject?.name || t('management.selectProject')}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {projects.map((project) => (
                  <SelectItem key={project.id} value={project.id}>
                    {project.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button variant="outline" onClick={() => handleCreateProject()}>
              <Plus className="w-4 h-4 mr-2" />
              {t('management.createProject')}
            </Button>
            <Button
              variant="outline"
              onClick={() => handleDeleteProject()}
              disabled={!validProjectId || projects.length <= 1 || isDeletingProject}
            >
              <Trash2 className="w-4 h-4 mr-2" />
              {isDeletingProject ? t('management.deletingProject') : t('management.deleteProject')}
            </Button>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={() => navigate('/workspaces/create')}>
            {t('viewSwitcher.createWorkspace')}
          </Button>
          <Button onClick={() => setShowWizard(true)}>
            <Plus className="w-4 h-4 mr-2" />
            {t('management.createWorkflow')}
          </Button>
        </div>
      </div>

      <div className="flex flex-wrap gap-2">
        {projects.map((project) => (
          <Button
            key={project.id}
            variant={project.id === validProjectId ? 'default' : 'outline'}
            size="sm"
            onClick={() => handleProjectChange(project.id)}
          >
            {project.name}
          </Button>
        ))}
      </div>

      {showWizard && (
        <WorkflowWizard
          onComplete={handleCreateWorkflow}
          onCancel={() => setShowWizard(false)}
        />
      )}

      <WorkflowListContent
        showWizard={showWizard}
        workflows={workflows}
        onOpenWizard={() => setShowWizard(true)}
        onSelectWorkflow={(workflowId) => setSelectedWorkflowId(workflowId)}
      />
      {promptDialog}
    </div>
  );
}

