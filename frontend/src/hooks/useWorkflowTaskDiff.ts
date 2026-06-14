import { useQuery } from '@tanstack/react-query';
import { handleApiResponse, makeRequest } from '@/lib/api';
import type { Diff } from 'shared/types';

// ============================================================================
// Query Keys
// ============================================================================

export const workflowTaskDiffKeys = {
  all: ['workflowTaskDiff'] as const,
  forTask: (workflowId: string, taskId: string) =>
    ['workflowTaskDiff', 'workflow', workflowId, 'task', taskId] as const,
};

// Sentinel key segment used when either id is missing, preventing disabled
// consumers from sharing the same cache entry. [E19-09]
const DISABLED_KEY = '__disabled__';

// ============================================================================
// API
// ============================================================================

const workflowTaskDiffApi = {
  getDiff: async (workflowId: string, taskId: string): Promise<Diff[]> => {
    const response = await makeRequest(
      `/api/workflows/${encodeURIComponent(workflowId)}/tasks/${encodeURIComponent(taskId)}/diff`
    );
    return handleApiResponse<Diff[]>(response);
  },
};

// ============================================================================
// Hook
// ============================================================================

/**
 * Fetches the branch-vs-target diff for a workflow task.
 *
 * Calls GET /api/workflows/{workflowId}/tasks/{taskId}/diff and returns the
 * same Diff[] shape that ChangesPanel / ChangesPanelContainer consume.
 * Backend diffs task.branch vs workflow.target_branch via GitService::get_diffs
 * (DiffTarget::Branch) — no worktree creation, no Workspace keying.
 *
 * Pass both ids as undefined/null to disable the query (e.g. when no task is
 * selected). The query is cached per (workflowId, taskId) pair and does not
 * auto-refetch (diffs are a point-in-time snapshot; callers can invalidate on
 * WS events such as terminal.completed or acceptance.review_result).
 */
export function useWorkflowTaskDiff(
  workflowId: string | null | undefined,
  taskId: string | null | undefined
) {
  const enabled = Boolean(workflowId) && Boolean(taskId);

  return useQuery({
    queryKey: workflowTaskDiffKeys.forTask(
      workflowId ?? DISABLED_KEY,
      taskId ?? DISABLED_KEY
    ),
    queryFn: () => workflowTaskDiffApi.getDiff(workflowId!, taskId!),
    enabled,
    // Diffs are stable once a task completes; callers should invalidate this
    // key on terminal.completed / acceptance.review_result WS events rather
    // than relying on background refetch.
    staleTime: 5 * 60 * 1000,
  });
}
