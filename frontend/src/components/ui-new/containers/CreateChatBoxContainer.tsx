import { useMemo, useCallback, useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useToast } from '@/components/ui/toast';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { useCreateMode } from '@/contexts/CreateModeContext';
import { useUserSystem } from '@/components/ConfigProvider';
import { useCreateAttachments } from '@/hooks/useCreateAttachments';
import { WorkflowProgressContainer } from './WorkflowProgressContainer';
import { useWorkflowLiveStatus } from '@/hooks/useWorkflowLiveStatus';
import { getVariantOptions, areProfilesEqual } from '@/utils/executor';
import type { ExecutorProfileId, BaseCodingAgent } from 'shared/types';
import type { ModelConfig as WorkflowModelConfig } from '@/components/workflow/types';
import { useModelConfigForExecutor } from '@/hooks/useModelConfigForExecutor';
import {
  planningDraftsApi,
  feishuApi,
  type PlanningMessageResponse,
} from '@/lib/api';
import {
  planningDraftKeys,
  usePlanningDraft,
  usePlanningDraftMessages,
  useSendPlanningMessage,
  useConfirmDraft,
  useMaterializeDraft,
  useTogglePlanningFeishuSync,
} from '@/hooks/usePlanningDraft';
import { CreateChatBox } from '../primitives/CreateChatBox';
import { AuditDocPanel } from './AuditDocPanel';

function WorkflowStatusBadge({
  workflowId,
}: Readonly<{ workflowId: string | null }>) {
  const { workflowStatus } = useWorkflowLiveStatus(workflowId ?? '');
  const status = workflowStatus ?? 'loading';
  const isCompleted = status === 'completed';
  const isFailed = status === 'failed';
  let colorClass = 'bg-muted text-muted-foreground';
  let label = 'Loading';
  if (isCompleted) {
    colorClass = 'bg-success/10 text-success';
    label = 'Completed';
  } else if (isFailed) {
    colorClass = 'bg-error/10 text-error';
    label = 'Failed';
  } else if (status === 'running') {
    colorClass = 'bg-brand/10 text-brand animate-pulse';
    label = 'Running';
  } else if (status === 'ready') {
    colorClass = 'bg-brand/10 text-brand';
    label = 'Ready';
  } else if (status === 'cancelled') {
    colorClass = 'bg-muted text-muted-foreground';
    label = 'Cancelled';
  }
  return (
    <span
      className={`ml-auto text-sm px-base py-half rounded font-medium ${colorClass}`}
    >
      {label}
    </span>
  );
}

function useSyncDraftState(
  planningDraft:
    | { projectId?: string; materializedWorkflowId?: string | null }
    | null
    | undefined,
  selectedProjectId: string | null | undefined,
  setSelectedProjectId: (id: string) => void,
  setMaterializedWorkflowId: (id: string | null) => void
) {
  useEffect(() => {
    if (!planningDraft) return;
    if (
      planningDraft.projectId &&
      planningDraft.projectId !== selectedProjectId
    ) {
      setSelectedProjectId(planningDraft.projectId);
    }
    setMaterializedWorkflowId(planningDraft.materializedWorkflowId ?? null);
  }, [
    planningDraft,
    selectedProjectId,
    setSelectedProjectId,
    setMaterializedWorkflowId,
  ]);
}

function useFeishuConnectionStatus(planningDraftId: string | null): boolean {
  const [connected, setConnected] = useState(false);
  useEffect(() => {
    if (!planningDraftId) return;
    let cancelled = false;
    feishuApi
      .getStatus()
      .then((status) => {
        if (!cancelled) setConnected(status.connectionStatus === 'connected');
      })
      .catch(() => {
        if (!cancelled) setConnected(false);
      });
    return () => {
      cancelled = true;
    };
  }, [planningDraftId]);
  return connected;
}

function resolveEffectiveProfile(
  selectedProfile: ExecutorProfileId | null,
  savedProfile: ExecutorProfileId | undefined,
  profiles: Record<string, Record<string, unknown>> | null | undefined
): ExecutorProfileId | null {
  if (selectedProfile) return selectedProfile;
  if (savedProfile) return savedProfile;
  const firstExecutor = profiles
    ? (Object.keys(profiles)[0] as BaseCodingAgent | undefined)
    : undefined;
  if (!firstExecutor) return null;
  const variants = Object.keys(profiles![firstExecutor]);
  return { executor: firstExecutor, variant: variants[0] ?? null };
}

function checkProfileChanged(
  effectiveProfile: ExecutorProfileId | null,
  savedProfile: ExecutorProfileId | undefined
): boolean {
  if (!savedProfile || !effectiveProfile) return false;
  return !areProfilesEqual(effectiveProfile, savedProfile);
}

function extractPlannerModelConfig(
  config: Record<string, unknown> | null | undefined
): WorkflowModelConfig | null {
  const lib = (config as Record<string, unknown>)?.workflow_model_library as
    | WorkflowModelConfig[]
    | undefined;
  if (!lib || lib.length === 0) return null;
  return lib.find((m) => m.apiKey) ?? lib[0] ?? null;
}

function resolveVariantForExecutor(
  executor: BaseCodingAgent,
  profiles: Record<string, Record<string, unknown>> | null | undefined,
  savedProfile: ExecutorProfileId | null | undefined
): string | null {
  const executorConfig = profiles?.[executor];
  if (!executorConfig) return null;
  const variants = Object.keys(executorConfig);
  const savedVariant =
    savedProfile?.executor === executor ? savedProfile.variant : null;
  if (savedVariant && variants.includes(savedVariant)) return savedVariant;
  return variants.includes('DEFAULT') ? 'DEFAULT' : (variants[0] ?? null);
}

interface PlanningStatusBarProps {
  readonly draftStatus: string | undefined;
  readonly feishuConnected: boolean;
  readonly feishuSyncEnabled: boolean;
  readonly isSpecReady: boolean;
  readonly isConfirmed: boolean;
  readonly isMaterialized: boolean;
  readonly materializedWorkflowId: string | null;
  readonly confirmMutation: ReturnType<typeof useConfirmDraft>;
  readonly materializeMutation: ReturnType<typeof useMaterializeDraft>;
  readonly handleConfirm: () => void;
  readonly handleMaterialize: () => void;
  readonly handleToggleFeishuSync: () => void;
  readonly handleSyncHistory: () => void;
}

function PlanningStatusBar({
  draftStatus,
  feishuConnected,
  feishuSyncEnabled,
  isSpecReady,
  isConfirmed,
  isMaterialized,
  materializedWorkflowId,
  confirmMutation,
  materializeMutation,
  handleConfirm,
  handleMaterialize,
  handleToggleFeishuSync,
  handleSyncHistory,
}: PlanningStatusBarProps) {
  const { t: tTasks } = useTranslation('tasks');
  return (
    <div className="shrink-0 px-double py-half border-b flex items-center gap-half">
      <span className="text-xs text-low">
        {tTasks('conversation.planning.title')}
      </span>
      {draftStatus && (
        <span className="text-xs px-1 py-px rounded bg-brand/10 text-brand">
          {tTasks(`conversation.planning.status.${draftStatus}`)}
        </span>
      )}
      {feishuConnected && (
        <button
          type="button"
          onClick={handleToggleFeishuSync}
          className={`flex items-center gap-1 rounded px-half py-px text-xs transition-colors ${
            feishuSyncEnabled
              ? 'bg-brand/20 text-brand hover:bg-brand/30'
              : 'bg-secondary text-low hover:text-normal'
          }`}
          title={feishuSyncEnabled ? '飞书同步已开启' : '飞书同步已关闭'}
        >
          <span
            className={`inline-block size-1.5 rounded-full ${feishuSyncEnabled ? 'bg-brand' : 'bg-secondary'}`}
          />{' '}
          飞书同步
        </button>
      )}
      {feishuConnected && (
        <button
          type="button"
          onClick={handleSyncHistory}
          className="flex items-center gap-1 rounded px-half py-px text-xs bg-secondary text-low hover:text-normal hover:bg-tertiary transition-colors disabled:opacity-50"
          title="将历史消息全量发送到飞书"
        >
          同步历史
        </button>
      )}
      {isSpecReady && (
        <button
          onClick={handleConfirm}
          disabled={confirmMutation.isPending}
          className="ml-auto text-xs px-base py-half rounded bg-brand text-white hover:bg-brand/90 disabled:opacity-50"
        >
          {confirmMutation.isPending
            ? '...'
            : tTasks('conversation.planning.confirmButton')}
        </button>
      )}
      {isConfirmed && (
        <button
          onClick={handleMaterialize}
          disabled={materializeMutation.isPending}
          className="ml-auto text-xs px-base py-half rounded bg-brand text-white hover:bg-brand/90 disabled:opacity-50"
        >
          {materializeMutation.isPending
            ? '...'
            : tTasks('conversation.planning.materializeButton')}
        </button>
      )}
      {isMaterialized && (
        <WorkflowStatusBadge
          key={materializedWorkflowId}
          workflowId={materializedWorkflowId}
        />
      )}
    </div>
  );
}

function usePlanningDraftActions({
  planningDraftId,
  planningDraft,
  message,
  setMessage,
  setIsThinking,
  setLocalMessages,
  setMaterializedWorkflowId,
  sendMessageMutation,
  confirmMutation,
  materializeMutation,
  feishuSyncMutation,
  showToast,
  retainBuiltin,
}: Readonly<{
  planningDraftId: string | null;
  planningDraft: { feishuSync?: boolean } | null | undefined;
  message: string;
  setMessage: (v: string) => void;
  setIsThinking: (v: boolean) => void;
  setLocalMessages: React.Dispatch<
    React.SetStateAction<PlanningMessageResponse[]>
  >;
  setMaterializedWorkflowId: (id: string | null) => void;
  sendMessageMutation: ReturnType<typeof useSendPlanningMessage>;
  confirmMutation: ReturnType<typeof useConfirmDraft>;
  materializeMutation: ReturnType<typeof useMaterializeDraft>;
  feishuSyncMutation: ReturnType<typeof useTogglePlanningFeishuSync>;
  showToast: (message: string, type?: 'success' | 'error' | 'info') => void;
  retainBuiltin: boolean;
}>) {
  const handlePlanningMessage = useCallback(async () => {
    const trimmed = message.trim();
    if (!trimmed || !planningDraftId) return;

    setIsThinking(true);
    setMessage('');
    try {
      const newMessages = await sendMessageMutation.mutateAsync({
        draftId: planningDraftId,
        message: trimmed,
      });
      setLocalMessages((prev) => [...prev, ...newMessages]);
    } catch (e) {
      console.error('Failed to send planning message:', e);
      const err = e as { message?: string };
      showToast(err.message ?? 'Failed to send planning message', 'error');
    } finally {
      setIsThinking(false);
    }
  }, [
    message,
    planningDraftId,
    setMessage,
    sendMessageMutation,
    setIsThinking,
    setLocalMessages,
    showToast,
  ]);

  const handleConfirm = useCallback(async () => {
    if (!planningDraftId) return;
    try {
      await confirmMutation.mutateAsync({
        draftId: planningDraftId,
        retainBuiltin,
      });
    } catch (e) {
      console.error('Failed to confirm draft:', e);
      const err = e as { message?: string };
      showToast(err.message ?? 'Failed to confirm draft', 'error');
    }
  }, [planningDraftId, confirmMutation, showToast, retainBuiltin]);

  const handleMaterialize = useCallback(async () => {
    if (!planningDraftId) return;
    try {
      const result = await materializeMutation.mutateAsync(planningDraftId);
      setMaterializedWorkflowId(result.workflowId);
    } catch (e) {
      console.error('Failed to materialize draft:', e);
      const err = e as { message?: string };
      showToast(err.message ?? 'Failed to materialize draft', 'error');
    }
  }, [
    planningDraftId,
    materializeMutation,
    setMaterializedWorkflowId,
    showToast,
  ]);

  const handleToggleFeishuSync = useCallback(() => {
    if (!planningDraftId || !planningDraft) return;
    feishuSyncMutation.mutate(
      {
        draftId: planningDraftId,
        enabled: !planningDraft.feishuSync,
        syncHistory: false,
      },
      {
        onError: () => {
          showToast(
            '开启失败：未找到飞书聊天。请先在飞书中给 Bot 发送一条消息，建立连接后再试。',
            'error'
          );
        },
      }
    );
  }, [planningDraftId, planningDraft, feishuSyncMutation, showToast]);

  return {
    handlePlanningMessage,
    handleConfirm,
    handleMaterialize,
    handleToggleFeishuSync,
  };
}

export function CreateChatBoxContainer() {
  const { t } = useTranslation('common');
  const { t: tTasks } = useTranslation('tasks');
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const { profiles, config, updateAndSaveConfig } = useUserSystem();
  const {
    repos,
    selectedProfile,
    setSelectedProfile,
    message,
    setMessage,
    selectedProjectId,
    setSelectedProjectId,
    clearDraft: clearCreateDraft,
    hasInitialValue,
  } = useCreateMode();

  const [hasAttemptedSubmit, setHasAttemptedSubmit] = useState(false);
  const [saveAsDefault, setSaveAsDefault] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  // === Planning Draft state (persisted in URL) ===
  const [searchParams, setSearchParams] = useSearchParams();
  const planningDraftId = searchParams.get('draftId');
  const setPlanningDraftId = useCallback(
    (id: string | null) => {
      setSearchParams(
        (prev) => {
          id ? prev.set('draftId', id) : prev.delete('draftId');
          return prev;
        },
        { replace: true }
      );
    },
    [setSearchParams]
  );
  const [isCreatingDraft, setIsCreatingDraft] = useState(false);
  const [isThinking, setIsThinking] = useState(false);
  const [localMessages, setLocalMessages] = useState<PlanningMessageResponse[]>(
    []
  );
  const [materializedWorkflowId, setMaterializedWorkflowId] = useState<
    string | null
  >(null);
  const [retainBuiltin, setRetainBuiltin] = useState(true);

  const { data: planningDraft } = usePlanningDraft(planningDraftId);
  const { data: serverMessages } = usePlanningDraftMessages(planningDraftId);
  const sendMessageMutation = useSendPlanningMessage();
  const confirmMutation = useConfirmDraft();
  const materializeMutation = useMaterializeDraft();
  const feishuSyncMutation = useTogglePlanningFeishuSync();

  const feishuConnected = useFeishuConnectionStatus(planningDraftId);

  // Prefer serverMessages when available; fall back to localMessages during
  // the window between POST return and React Query cache invalidation.
  const hasServerMessages = !!serverMessages && serverMessages.length > 0;
  const planningMessages = hasServerMessages ? serverMessages : localMessages;

  // Sync draft-derived state when switching drafts
  useSyncDraftState(
    planningDraft,
    selectedProjectId,
    setSelectedProjectId,
    setMaterializedWorkflowId
  );

  // Attachment handling
  const handleInsertMarkdown = useCallback(
    (markdown: string) => {
      setMessage(message.trim() ? `${message}\n\n${markdown}` : markdown);
    },
    [message, setMessage]
  );

  const { uploadFiles, clearAttachments, localImages } =
    useCreateAttachments(handleInsertMarkdown);

  const effectiveProfile = useMemo(
    () =>
      resolveEffectiveProfile(
        selectedProfile,
        config?.executor_profile,
        profiles
      ),
    [selectedProfile, config?.executor_profile, profiles]
  );

  // Model config selection
  const {
    customModels,
    officialModels,
    allModels: availableModels,
    selectedModelConfigId,
    setSelectedModelConfigId,
  } = useModelConfigForExecutor(
    effectiveProfile?.executor ?? null,
    (config as Record<string, unknown>)?.workflow_model_library as
      | WorkflowModelConfig[]
      | undefined
  );

  const variantOptions = useMemo(
    () => getVariantOptions(effectiveProfile?.executor, profiles),
    [effectiveProfile?.executor, profiles]
  );

  const hasChangedFromDefault = useMemo(
    () => checkProfileChanged(effectiveProfile, config?.executor_profile),
    [effectiveProfile, config?.executor_profile]
  );

  useEffect(() => {
    if (!hasChangedFromDefault) setSaveAsDefault(false);
  }, [hasChangedFromDefault]);

  const projectId = selectedProjectId;

  const canSubmit =
    repos.length > 0 &&
    message.trim().length > 0 &&
    effectiveProfile !== null &&
    projectId !== undefined;

  const handleVariantChange = useCallback(
    (variant: string | null) => {
      if (!effectiveProfile) return;
      setSelectedProfile({ executor: effectiveProfile.executor, variant });
    },
    [effectiveProfile, setSelectedProfile]
  );

  const handleExecutorChange = useCallback(
    (executor: BaseCodingAgent) => {
      const variant = resolveVariantForExecutor(
        executor,
        profiles,
        config?.executor_profile
      );
      setSelectedProfile({ executor, variant });
    },
    [profiles, setSelectedProfile, config?.executor_profile]
  );

  const plannerModelConfig = useMemo(
    () => extractPlannerModelConfig(config),
    [config]
  );

  /** Resolve the model config that matches the user's dropdown selection. */
  const selectedPlannerModelConfig = useMemo(() => {
    if (!selectedModelConfigId) return plannerModelConfig;
    const lib = (config as Record<string, unknown>)?.workflow_model_library as
      | WorkflowModelConfig[]
      | undefined;
    if (!lib) return plannerModelConfig;
    return (
      lib.find((m) => m.id === selectedModelConfigId) ?? plannerModelConfig
    );
  }, [selectedModelConfigId, config, plannerModelConfig]);

  // === Phase 1: Initial submit — create planning draft ===
  const handleInitialSubmit = useCallback(async () => {
    setHasAttemptedSubmit(true);
    setSubmitError(null);
    if (!canSubmit || !projectId) return;

    const shouldSaveDefault =
      saveAsDefault && hasChangedFromDefault && effectiveProfile;
    if (shouldSaveDefault)
      await updateAndSaveConfig({ executor_profile: effectiveProfile });

    setIsCreatingDraft(true);
    setIsThinking(true);
    try {
      const modelConfig = selectedPlannerModelConfig;
      const draft = await planningDraftsApi.create({
        projectId,
        name: message.slice(0, 100),
        plannerModelId: modelConfig?.modelId,
        plannerApiType: modelConfig?.apiType,
        plannerBaseUrl: modelConfig?.baseUrl,
        plannerApiKey: modelConfig?.apiKey,
      });
      setPlanningDraftId(draft.id);

      const newMessages = await planningDraftsApi.sendMessage(
        draft.id,
        message
      );
      setLocalMessages(newMessages);
      // Invalidate React Query cache so serverMessages eventually catches up.
      // handleInitialSubmit bypasses the mutation, so invalidation must be manual.
      queryClient.invalidateQueries({
        queryKey: planningDraftKeys.messages(draft.id),
      });
      setMessage('');
      clearAttachments();
      await clearCreateDraft();
    } catch (e) {
      const err = e as { message?: string };
      setSubmitError(err.message ?? 'Failed to start planning');
    } finally {
      setIsCreatingDraft(false);
      setIsThinking(false);
    }
  }, [
    canSubmit,
    projectId,
    message,
    saveAsDefault,
    hasChangedFromDefault,
    effectiveProfile,
    updateAndSaveConfig,
    selectedPlannerModelConfig,
    setPlanningDraftId,
    setMessage,
    clearAttachments,
    clearCreateDraft,
    queryClient,
  ]);

  // === Planning draft action handlers (extracted to reduce cognitive complexity) ===
  const {
    handlePlanningMessage,
    handleConfirm,
    handleMaterialize,
    handleToggleFeishuSync,
  } = usePlanningDraftActions({
    planningDraftId,
    planningDraft,
    message,
    setMessage,
    setIsThinking,
    setLocalMessages,
    setMaterializedWorkflowId,
    sendMessageMutation,
    confirmMutation,
    materializeMutation,
    feishuSyncMutation,
    showToast,
    retainBuiltin,
  });

  const handleOpenBoard = useCallback(() => {
    if (!materializedWorkflowId) return;
    navigate(
      `/board?workflowId=${materializedWorkflowId}&projectId=${projectId ?? ''}`
    );
  }, [materializedWorkflowId, projectId, navigate]);

  // === Feishu history sync — one-time full push ===
  const handleSyncHistory = useCallback(() => {
    if (!planningDraftId) return;
    feishuSyncMutation.mutate(
      {
        draftId: planningDraftId,
        enabled: true,
        syncHistory: true,
      },
      {
        onError: () => {
          showToast(
            '同步失败：未找到飞书聊天。请先在飞书中给 Bot 发送一条消息，建立连接后再试。',
            'error'
          );
        },
      }
    );
  }, [planningDraftId, feishuSyncMutation, showToast]);

  // Determine error
  const displayError = (() => {
    if (submitError) return submitError;
    if (hasAttemptedSubmit && repos.length === 0) {
      return tTasks('conversation.planning.needRepo');
    }
    return null;
  })();

  if (!hasInitialValue) return null;

  if (!projectId) {
    return (
      <div className="flex h-full w-full items-center justify-center">
        <div className="text-center max-w-sm">
          <h2 className="text-lg font-medium text-high mb-2">
            {t('workspace.selectProjectTitle')}
          </h2>
          <p className="text-sm text-low">{t('workspace.selectProjectHint')}</p>
        </div>
      </div>
    );
  }

  // Determine if we're in planning conversation mode
  const isInPlanningMode = !!planningDraftId;
  const draftStatus = planningDraft?.status;
  const isMaterialized =
    draftStatus === 'materialized' || !!materializedWorkflowId;
  const isSpecReady = draftStatus === 'spec_ready';
  const isConfirmed = draftStatus === 'confirmed' && !isMaterialized;

  return (
    <div className="relative flex flex-1 h-full overflow-hidden">
      {/* Main chat area */}
      <div className="flex flex-1 flex-col bg-primary h-full overflow-hidden">
        {/* Planning conversation messages (Phase 2 only) */}
        {isInPlanningMode && (
          <>
            {/* Status badge */}
            <PlanningStatusBar
              draftStatus={draftStatus}
              feishuConnected={feishuConnected}
              feishuSyncEnabled={planningDraft?.feishuSync ?? false}
              isSpecReady={isSpecReady}
              isConfirmed={isConfirmed}
              isMaterialized={isMaterialized}
              materializedWorkflowId={materializedWorkflowId}
              confirmMutation={confirmMutation}
              materializeMutation={materializeMutation}
              handleConfirm={handleConfirm}
              handleMaterialize={handleMaterialize}
              handleToggleFeishuSync={handleToggleFeishuSync}
              handleSyncHistory={handleSyncHistory}
            />

            {/* Scrollable message list */}
            <div className="flex-1 min-h-0 overflow-y-auto px-double py-base space-y-base">
              {planningMessages.map((msg) => (
                <div
                  key={msg.id}
                  className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
                >
                  <div
                    className={`max-w-[80%] rounded-lg px-base py-half text-sm whitespace-pre-wrap ${
                      msg.role === 'user'
                        ? 'bg-brand/10 text-high'
                        : 'bg-secondary text-normal'
                    }`}
                  >
                    {msg.content}
                  </div>
                </div>
              ))}
              {isThinking && (
                <div className="flex justify-start">
                  <div className="bg-secondary rounded-lg px-base py-half text-sm text-low animate-pulse">
                    {tTasks('conversation.planning.thinking')}
                  </div>
                </div>
              )}
              {isMaterialized && materializedWorkflowId && (
                <WorkflowProgressContainer
                  workflowId={materializedWorkflowId}
                  onOpenBoard={handleOpenBoard}
                />
              )}
            </div>
          </>
        )}

        {/* Input area — same CreateChatBox for both phases */}
        <div
          className={
            isInPlanningMode
              ? 'shrink-0 pb-[48px]'
              : 'flex-1 flex flex-col justify-end'
          }
        >
          <div className="flex justify-center @container">
            <CreateChatBox
              editor={{
                value: message,
                onChange: setMessage,
              }}
              onSend={
                isInPlanningMode ? handlePlanningMessage : handleInitialSubmit
              }
              isSending={isCreatingDraft || isThinking}
              executor={{
                selected: effectiveProfile?.executor ?? null,
                options: Object.keys(profiles || {}) as BaseCodingAgent[],
                onChange: handleExecutorChange,
              }}
              modelConfig={
                availableModels.length > 0
                  ? {
                      customModels,
                      officialModels,
                      selectedId: selectedModelConfigId,
                      onChange: setSelectedModelConfigId,
                    }
                  : undefined
              }
              variant={
                effectiveProfile
                  ? {
                      selected: effectiveProfile.variant ?? 'DEFAULT',
                      options: variantOptions,
                      onChange: handleVariantChange,
                    }
                  : undefined
              }
              saveAsDefault={{
                checked: saveAsDefault,
                onChange: setSaveAsDefault,
                visible: hasChangedFromDefault,
              }}
              error={displayError}
              projectId={projectId}
              agent={effectiveProfile?.executor ?? null}
              onPasteFiles={uploadFiles}
              localImages={localImages}
              sendLabel={
                isInPlanningMode
                  ? tTasks('conversation.actions.send')
                  : undefined
              }
              sendingLabel={
                isInPlanningMode
                  ? tTasks('conversation.planning.thinking')
                  : undefined
              }
            />
          </div>
        </div>
      </div>

      {/* Audit document panel (right side, planning mode only) */}
      {isInPlanningMode && (
        <AuditDocPanel
          draftId={planningDraftId}
          draftStatus={planningDraft?.status}
          auditDocPath={planningDraft?.auditDocPath ?? null}
          auditMode={planningDraft?.auditMode ?? 'builtin'}
          onRetainBuiltinChange={setRetainBuiltin}
          retainBuiltin={retainBuiltin}
        />
      )}
    </div>
  );
}
