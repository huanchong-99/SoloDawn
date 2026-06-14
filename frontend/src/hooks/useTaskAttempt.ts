import { useQuery } from '@tanstack/react-query';
import { attemptsApi } from '@/lib/api';
import type { WorkspaceWithSession } from '@/types/attempt';

/**
 * Hook for components that need executor field (e.g., for capability checks).
 * Fetches workspace with executor from latest session.
 */
export function useTaskAttemptWithSession(attemptId?: string) {
  return useQuery<WorkspaceWithSession>({
    queryKey: ['taskAttemptWithSession', attemptId],
    queryFn: () => attemptsApi.getWithSession(attemptId!),
    enabled: !!attemptId,
  });
}
