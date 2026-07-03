import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  requirementItemsApi,
  type RequirementCapsule,
  type RequirementItemResponse,
} from '@/lib/api';

export const requirementItemKeys = {
  byProject: (projectId: string) =>
    ['requirementItems', projectId] as const,
};

/** Project-scoped requirement ledger (评分点账本). */
export function useRequirementItems(projectId: string | null) {
  return useQuery({
    queryKey: requirementItemKeys.byProject(projectId ?? ''),
    queryFn: () => requirementItemsApi.list(projectId!),
    enabled: !!projectId,
    staleTime: 15_000,
  });
}

/** Edit the text of a pending point (pre-confirm curation). */
export function useUpdateRequirementItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      projectId,
      itemId,
      text,
    }: {
      projectId: string;
      itemId: string;
      text: string;
    }) => requirementItemsApi.update(projectId, itemId, text),
    onSuccess: (_data, { projectId }) => {
      queryClient.invalidateQueries({
        queryKey: requirementItemKeys.byProject(projectId),
      });
    },
  });
}

/** Delete a pending point (pre-confirm curation). */
export function useDeleteRequirementItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      projectId,
      itemId,
    }: {
      projectId: string;
      itemId: string;
    }) => requirementItemsApi.remove(projectId, itemId),
    onSuccess: (_data, { projectId }) => {
      queryClient.invalidateQueries({
        queryKey: requirementItemKeys.byProject(projectId),
      });
    },
  });
}

/** Parse a point's contextCapsule JSON; null when absent or malformed. */
export function parseCapsule(
  item: RequirementItemResponse
): RequirementCapsule | null {
  if (!item.contextCapsule) return null;
  try {
    const raw = JSON.parse(item.contextCapsule) as Partial<RequirementCapsule>;
    return {
      built: raw.built ?? '',
      livesWhere: raw.livesWhere ?? '',
      decisions: raw.decisions ?? '',
      extensionNotes: raw.extensionNotes ?? '',
    };
  } catch {
    return null;
  }
}
