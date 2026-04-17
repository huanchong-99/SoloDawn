import { useQuery } from '@tanstack/react-query';
import { getSharedTaskAssignees } from '@/lib/remoteApi';
import type { SharedTask, UserData } from 'shared/types';
import { useEffect, useMemo, useRef } from 'react';

interface UseAssigneeUserNamesOptions {
  projectId: string | undefined;
  sharedTasks?: SharedTask[];
  enabled?: boolean;
}

export function useAssigneeUserNames(options: UseAssigneeUserNamesOptions) {
  const { projectId, sharedTasks, enabled = true } = options;

  const { data: assignees, refetch } = useQuery<UserData[], Error>({
    queryKey: ['project', 'assignees', projectId],
    queryFn: () => getSharedTaskAssignees(projectId!),
    enabled: Boolean(projectId) && enabled,
    staleTime: 5 * 60 * 1000, // 5 minutes
  });

  const assignedUserIds = useMemo(() => {
    if (!sharedTasks) return null;
    return Array.from(
      new Set(sharedTasks.map((task) => task.assigneeUserId))
    );
  }, [sharedTasks]);

  // Refetch when assignee ids change. Use a ref for refetch so it does not
  // retrigger the effect when the query client produces a new function identity.
  const refetchRef = useRef(refetch);
  useEffect(() => {
    refetchRef.current = refetch;
  }, [refetch]);

  useEffect(() => {
    if (!assignedUserIds || !enabled) return;
    refetchRef.current();
  }, [assignedUserIds, enabled]);

  return {
    assignees,
    refetchAssignees: refetch,
  };
}
