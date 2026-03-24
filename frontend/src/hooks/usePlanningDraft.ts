import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  planningDraftsApi,
  attemptsApi,
  type PlanningDraftResponse,
  type PlanningMessageResponse,
} from '@/lib/api';

export const planningDraftKeys = {
  all: ['planningDrafts'] as const,
  byId: (draftId: string) => ['planningDrafts', draftId] as const,
  messages: (draftId: string) =>
    ['planningDrafts', draftId, 'messages'] as const,
};

/** Fetch all planning drafts (cross-project) */
export function usePlanningDrafts() {
  return useQuery({
    queryKey: planningDraftKeys.all,
    queryFn: () => planningDraftsApi.list(),
    staleTime: 30_000,
  });
}

/** Fetch a single planning draft by ID */
export function usePlanningDraft(draftId: string | null) {
  return useQuery({
    queryKey: planningDraftKeys.byId(draftId ?? ''),
    queryFn: () => planningDraftsApi.get(draftId!),
    enabled: !!draftId,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      // Poll while in active states
      if (status === 'gathering' || status === 'spec_ready') return 3000;
      return false;
    },
  });
}

/** Fetch conversation messages for a planning draft */
export function usePlanningDraftMessages(draftId: string | null) {
  return useQuery({
    queryKey: planningDraftKeys.messages(draftId ?? ''),
    queryFn: () => planningDraftsApi.listMessages(draftId!),
    enabled: !!draftId,
  });
}

/** Send a message to the planning draft and get LLM response */
export function useSendPlanningMessage() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      draftId,
      message,
    }: {
      draftId: string;
      message: string;
    }): Promise<PlanningMessageResponse[]> => {
      return planningDraftsApi.sendMessage(draftId, message);
    },
    onSuccess: (_data, variables) => {
      // Refresh messages and draft status
      queryClient.invalidateQueries({
        queryKey: planningDraftKeys.messages(variables.draftId),
      });
      queryClient.invalidateQueries({
        queryKey: planningDraftKeys.byId(variables.draftId),
      });
    },
  });
}

/** Confirm a planning draft specification */
export function useConfirmDraft() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (draftId: string): Promise<PlanningDraftResponse> => {
      return planningDraftsApi.confirm(draftId);
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({
        queryKey: planningDraftKeys.byId(data.id),
      });
    },
  });
}

/** Materialize a confirmed draft into a workflow */
export function useMaterializeDraft() {
  return useMutation({
    mutationFn: async (draftId: string) => {
      return planningDraftsApi.materialize(draftId);
    },
  });
}

/** Fetch planning draft messages associated with a workspace */
export function useWorkspacePlanningMessages(workspaceId: string | null) {
  return useQuery({
    queryKey: ['workspace', workspaceId, 'planning-messages'] as const,
    queryFn: () => attemptsApi.getPlanningMessages(workspaceId!),
    enabled: !!workspaceId,
    staleTime: Infinity,
  });
}
