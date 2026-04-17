import { useCallback, useMemo } from 'react';
import { useJsonPatchWsStream } from './useJsonPatchWsStream';
import { useAuth } from '@/hooks';
import { useProject } from '@/contexts/ProjectContext';
import { useUserSystem } from '@/components/ConfigProvider';
import { useLiveQuery, eq, isNull } from '@tanstack/react-db';
import { sharedTasksCollection } from '@/lib/electric/sharedTasksCollection';
import { useAssigneeUserNames } from './useAssigneeUserName';
import { useAutoLinkSharedTasks } from './useAutoLinkSharedTasks';
import type {
  SharedTask,
  TaskStatus,
  TaskWithAttemptStatus,
} from 'shared/types';

export type SharedTaskRecord = SharedTask & {
  remote_project_id: string;
  assignee_first_name?: string | null;
  assignee_last_name?: string | null;
  assignee_username?: string | null;
};

type TasksState = {
  tasks: Record<string, TaskWithAttemptStatus>;
};

export interface UseProjectTasksResult {
  tasks: TaskWithAttemptStatus[];
  tasksById: Record<string, TaskWithAttemptStatus>;
  tasksByStatus: Record<TaskStatus, TaskWithAttemptStatus[]>;
  sharedTasksById: Record<string, SharedTaskRecord>;
  sharedOnlyByStatus: Record<TaskStatus, SharedTaskRecord[]>;
  isLoading: boolean;
  isConnected: boolean;
  error: string | null;
}

/**
 * Stream tasks for a project via WebSocket (JSON Patch) and expose as array + map.
 * Server sends initial snapshot: replace /tasks with an object keyed by id.
 * Live updates arrive at /tasks/<id> via add/replace/remove operations.
 */
export const useProjectTasks = (projectId: string): UseProjectTasksResult => {
  const { project } = useProject();
  const { isSignedIn } = useAuth();
  const { remoteFeaturesEnabled } = useUserSystem();
  const remoteProjectId = project?.remoteProjectId;
  // Remote shared-task APIs are currently disabled server-side; keep this feature
  // behind an explicit opt-in so clients don't subscribe to a removed shape.
  const sharedTasksFeatureEnabled =
    import.meta.env.VITE_ENABLE_SHARED_TASKS === 'true';
  const sharedTasksEnabled =
    sharedTasksFeatureEnabled &&
    remoteFeaturesEnabled &&
    Boolean(remoteProjectId) &&
    isSignedIn;

  const endpoint = `/api/tasks/stream/ws?project_id=${encodeURIComponent(projectId)}`;

  const initialData = useCallback((): TasksState => ({ tasks: {} }), []);

  const { data, isConnected, isInitialized, error } = useJsonPatchWsStream(
    endpoint,
    !!projectId,
    initialData
  );

  const sharedTasksQuery = useLiveQuery(
    useCallback(
      (q) => {
        if (!sharedTasksEnabled) {
          return undefined;
        }
        return q
          .from({ sharedTasks: sharedTasksCollection })
          .where(({ sharedTasks }) =>
            eq(sharedTasks.projectId, remoteProjectId)
          )
          .where(({ sharedTasks }) => isNull(sharedTasks.deletedAt));
      },
      [remoteProjectId, sharedTasksEnabled]
    ),
    [remoteProjectId, sharedTasksEnabled]
  );

  const sharedTasksList = useMemo(
    () => (sharedTasksEnabled ? sharedTasksQuery.data ?? [] : []),
    [sharedTasksQuery.data, sharedTasksEnabled]
  );

  const localTasksById = useMemo(() => data?.tasks || {}, [data?.tasks]);

  const referencedSharedIds = useMemo(
    () =>
      sharedTasksEnabled
        ? new Set(
            Object.values(localTasksById)
              .map((task) => task.sharedTaskId)
              .filter((id): id is string => Boolean(id))
          )
        : new Set<string>(),
    [localTasksById, sharedTasksEnabled]
  );

  const { assignees } = useAssigneeUserNames({
    projectId: remoteProjectId || undefined,
    sharedTasks: sharedTasksList,
    enabled: sharedTasksEnabled,
  });

  const sharedTasksById = useMemo(() => {
    if (!sharedTasksList) return {};
    const map: Record<string, SharedTaskRecord> = {};
    const list = Array.isArray(sharedTasksList) ? sharedTasksList : [];
    for (const task of list) {
      let assignee = null;
      if (task.assigneeUserId && assignees) {
        assignee = assignees.find((a) => a.userId === task.assigneeUserId) ?? null;
      }
      map[task.id] = {
        ...task,
        status: task.status,
        remote_project_id: task.projectId,
        assignee_first_name: assignee?.firstName ?? null,
        assignee_last_name: assignee?.lastName ?? null,
        assignee_username: assignee?.username ?? null,
      };
    }
    return map;
  }, [sharedTasksList, assignees]);

  const { tasks, tasksById, tasksByStatus } = useMemo(() => {
    const merged: Record<string, TaskWithAttemptStatus> = { ...localTasksById };
    const byStatus: Record<TaskStatus, TaskWithAttemptStatus[]> = {
      todo: [],
      inprogress: [],
      inreview: [],
      done: [],
      cancelled: [],
    };

    Object.values(merged).forEach((task) => {
      byStatus[task.status]?.push(task);
    });

    const sorted = Object.values(merged).sort(
      (a, b) =>
        new Date(b.createdAt).getTime() -
        new Date(a.createdAt).getTime()
    );

    Object.values(byStatus).forEach((list) => {
      list.sort(
        (a, b) =>
          new Date(b.createdAt).getTime() -
          new Date(a.createdAt).getTime()
      );
    });

    return { tasks: sorted, tasksById: merged, tasksByStatus: byStatus };
  }, [localTasksById]);

  const sharedOnlyByStatus = useMemo(() => {
    const grouped: Record<TaskStatus, SharedTaskRecord[]> = {
      todo: [],
      inprogress: [],
      inreview: [],
      done: [],
      cancelled: [],
    };

    Object.values(sharedTasksById).forEach((sharedTask) => {
      const hasLocal =
        Boolean(localTasksById[sharedTask.id]) ||
        referencedSharedIds.has(sharedTask.id);

      if (hasLocal) {
        return;
      }
      grouped[sharedTask.status]?.push(sharedTask);
    });

    Object.values(grouped).forEach((list) => {
      list.sort(
        (a, b) =>
          new Date(b.createdAt).getTime() -
          new Date(a.createdAt).getTime()
      );
    });

    return grouped;
  }, [localTasksById, sharedTasksById, referencedSharedIds]);

  const isLoading = !isInitialized && !error; // until first snapshot

  // Auto-link shared tasks assigned to current user
  useAutoLinkSharedTasks({
    sharedTasksById,
    localTasksById,
    referencedSharedIds,
    isLoading,
    remoteProjectId: project?.remoteProjectId || undefined,
    projectId,
    enabled: sharedTasksEnabled,
  });

  return {
    tasks,
    tasksById,
    tasksByStatus,
    sharedTasksById,
    sharedOnlyByStatus,
    isLoading,
    isConnected,
    error,
  };
};
