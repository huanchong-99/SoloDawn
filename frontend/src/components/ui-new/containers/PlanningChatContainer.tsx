import { useCallback, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '@/components/ui/toast';
import { useCreateMode } from '@/contexts/CreateModeContext';
import { useUserSystem } from '@/components/ConfigProvider';
import type { ModelConfig as WorkflowModelConfig } from '@/components/workflow/types';
import {
  planningDraftsApi,
  type PlanningMessageResponse,
} from '@/lib/api';
import {
  usePlanningDraft,
  usePlanningDraftMessages,
  useSendPlanningMessage,
  useConfirmDraft,
  useMaterializeDraft,
} from '@/hooks/usePlanningDraft';
import { PlanningChat } from '../primitives/PlanningChat';

/**
 * Container for the orchestration workspace planning chat.
 * Manages the Planning Draft lifecycle: create → gather requirements → confirm → materialize.
 */
export function PlanningChatContainer() {
  const navigate = useNavigate();
  const { config } = useUserSystem();
  const { showToast } = useToast();
  const { selectedProjectId, message, setMessage } = useCreateMode();

  const [draftId, setDraftId] = useState<string | null>(null);
  const [localMessages, setLocalMessages] = useState<
    PlanningMessageResponse[]
  >([]);
  const [isThinking, setIsThinking] = useState(false);

  // Fetch draft and messages from server
  const { data: draft } = usePlanningDraft(draftId);
  const { data: serverMessages } = usePlanningDraftMessages(draftId);
  const sendMessageMutation = useSendPlanningMessage();
  const confirmMutation = useConfirmDraft();
  const materializeMutation = useMaterializeDraft();

  // Use server messages when available, local messages as optimistic fallback.
  // E07-04: memoize so downstream props get a stable reference when neither
  // input changed (prevents unnecessary re-renders of <PlanningChat />).
  const messages = useMemo(
    () => serverMessages ?? localMessages,
    [serverMessages, localMessages]
  );

  // Get planner model config from workflow_model_library
  const getFirstModelConfig = useCallback((): WorkflowModelConfig | null => {
    const lib = (config as Record<string, unknown>)
      ?.workflow_model_library as WorkflowModelConfig[] | undefined;
    if (!lib || lib.length === 0) return null;
    // Prefer first model with API key
    return lib.find((m) => m.apiKey) ?? lib[0] ?? null;
  }, [config]);

  // Handle first message: create draft + send
  const handleSend = useCallback(async () => {
    const trimmed = message.trim();
    if (!trimmed || !selectedProjectId) return;

    if (draftId) {
      // Follow-up message
      setIsThinking(true);
      setMessage('');
      try {
        const newMessages = await sendMessageMutation.mutateAsync({
          draftId,
          message: trimmed,
        });
        setLocalMessages((prev) => {
          const existingIds = new Set(prev.map((m) => m.id));
          const deduped = newMessages.filter((m) => !existingIds.has(m.id));
          return [...prev, ...deduped];
        });
      } catch (e) {
        console.error('Failed to send planning message:', e);
        const err = e as { message?: string };
        showToast(err.message ?? 'Failed to send planning message', 'error');
      } finally {
        setIsThinking(false);
      }
    } else {
      // First message — create draft then send
      setIsThinking(true);
      try {
        const modelConfig = getFirstModelConfig();
        const draft = await planningDraftsApi.create({
          projectId: selectedProjectId,
          name: trimmed.slice(0, 100),
          plannerModelId: modelConfig?.modelId,
          plannerApiType: modelConfig?.apiType,
          plannerBaseUrl: modelConfig?.baseUrl,
          plannerApiKey: modelConfig?.apiKey,
        });
        setDraftId(draft.id);
        const newMessages = await planningDraftsApi.sendMessage(
          draft.id,
          trimmed
        );
        setLocalMessages(newMessages);
        setMessage('');
      } catch (e) {
        console.error('Failed to create planning draft:', e);
        const err = e as { message?: string };
        showToast(err.message ?? 'Failed to create planning draft', 'error');
      } finally {
        setIsThinking(false);
      }
    }
  }, [
    message,
    selectedProjectId,
    draftId,
    getFirstModelConfig,
    setMessage,
    sendMessageMutation,
    showToast,
  ]);

  // Confirm the spec
  const handleConfirm = useCallback(async () => {
    if (!draftId) return;
    try {
      await confirmMutation.mutateAsync({ draftId });
    } catch (e) {
      console.error('Failed to confirm draft:', e);
      const err = e as { message?: string };
      showToast(err.message ?? 'Failed to confirm draft', 'error');
    }
  }, [draftId, confirmMutation, showToast]);

  // Materialize into workflow and navigate
  const handleMaterialize = useCallback(async () => {
    if (!draftId) return;
    try {
      const result = await materializeMutation.mutateAsync(draftId);
      // Navigate to the workflow board
      navigate(`/board?workflowId=${result.workflowId}`);
    } catch (e) {
      console.error('Failed to materialize draft:', e);
      const err = e as { message?: string };
      showToast(err.message ?? 'Failed to materialize draft', 'error');
    }
  }, [draftId, materializeMutation, navigate, showToast]);

  if (!selectedProjectId) {
    return null;
  }

  return (
    <div className="relative flex flex-1 flex-col bg-primary h-full justify-end">
      <PlanningChat
        draft={draft ?? null}
        messages={messages}
        editor={{
          value: message,
          onChange: setMessage,
        }}
        isThinking={isThinking}
        isConfirming={confirmMutation.isPending}
        isMaterializing={materializeMutation.isPending}
        projectId={selectedProjectId}
        onSend={handleSend}
        onConfirm={handleConfirm}
        onMaterialize={handleMaterialize}
      />
    </div>
  );
}
