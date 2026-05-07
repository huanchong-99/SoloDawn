import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigateWithSearch } from '@/hooks';
import { useAuth } from '@/hooks/auth/useAuth';
import { tasksApi } from '@/lib/api';
import { paths } from '@/lib/paths';
import { taskRelationshipsKeys } from '@/hooks/useTaskRelationships';
import { workspaceSummaryKeys } from '@/components/ui-new/hooks/useWorkspaces';
import type {
  CreateTask,
  CreateAndStartTaskRequest,
  Task,
  TaskWithAttemptStatus,
  UpdateTask,
  SharedTaskDetails,
} from 'shared/types';
import { taskKeys } from './useTask';

export function useTaskMutations(projectId?: string) {
  const queryClient = useQueryClient();
  const navigate = useNavigateWithSearch();
  const { isSignedIn } = useAuth();

  // Guard all cache invalidations by auth state — the sign-out path calls
  // `removeQueries`, and a late onSuccess from an in-flight mutation
  // would otherwise repopulate caches we just cleared. [E19-06]
  const invalidateQueries = (taskId?: string) => {
    if (!isSignedIn) return;
    queryClient.invalidateQueries({ queryKey: taskKeys.all });
    if (taskId) {
      queryClient.invalidateQueries({ queryKey: taskKeys.byId(taskId) });
    }
  };

  const invalidateTaskAndWorkspaceQueries = (taskId?: string) => {
    if (!isSignedIn) return;
    invalidateQueries(taskId);
    queryClient.invalidateQueries({ queryKey: workspaceSummaryKeys.all });
  };

  const createTask = useMutation({
    mutationFn: (data: CreateTask) => tasksApi.create(data),
    onSuccess: (createdTask: Task) => {
      invalidateQueries();
      // Invalidate parent's relationships cache if this is a subtask
      if (createdTask.parentWorkspaceId) {
        queryClient.invalidateQueries({
          queryKey: taskRelationshipsKeys.byAttempt(
            createdTask.parentWorkspaceId
          ),
        });
      }
      if (projectId) {
        navigate(paths.task(projectId, createdTask.id));
      }
    },
    onError: (err) => {
      console.error('Failed to create task:', err);
    },
  });

  const createAndStart = useMutation({
    mutationFn: (data: CreateAndStartTaskRequest) =>
      tasksApi.createAndStart(data),
    onSuccess: (createdTask: TaskWithAttemptStatus) => {
      invalidateTaskAndWorkspaceQueries(createdTask.id);
      // Invalidate parent's relationships cache if this is a subtask
      if (createdTask.parentWorkspaceId) {
        queryClient.invalidateQueries({
          queryKey: taskRelationshipsKeys.byAttempt(
            createdTask.parentWorkspaceId
          ),
        });
      }
      if (projectId) {
        navigate(`${paths.task(projectId, createdTask.id)}/attempts/latest`);
      }
    },
    onError: (err) => {
      console.error('Failed to create and start task:', err);
    },
  });

  const updateTask = useMutation({
    mutationFn: ({ taskId, data }: { taskId: string; data: UpdateTask }) =>
      tasksApi.update(taskId, data),
    onSuccess: (updatedTask: Task) => {
      invalidateQueries(updatedTask.id);
    },
    onError: (err) => {
      console.error('Failed to update task:', err);
    },
  });

  const deleteTask = useMutation({
    mutationFn: (taskId: string) => tasksApi.delete(taskId),
    onSuccess: (_: unknown, taskId: string) => {
      if (!isSignedIn) return;
      invalidateQueries(taskId);
      // Remove single-task cache entry to avoid stale data flashes
      queryClient.removeQueries({
        queryKey: taskKeys.byId(taskId),
        exact: true,
      });
      // Invalidate all task relationships caches (safe approach since we don't know parent)
      queryClient.invalidateQueries({ queryKey: taskRelationshipsKeys.all });
      // Invalidate workspace summaries so they refresh with the deleted workspace removed
      queryClient.invalidateQueries({ queryKey: workspaceSummaryKeys.all });
    },
    onError: (err) => {
      console.error('Failed to delete task:', err);
    },
  });

  const shareTask = useMutation({
    mutationFn: (taskId: string) => tasksApi.share(taskId),
    onSuccess: (_: unknown, taskId: string) => {
      invalidateTaskAndWorkspaceQueries(taskId);
    },
    onError: (err) => {
      console.error('Failed to share task:', err);
    },
  });

  const unshareSharedTask = useMutation({
    mutationFn: (sharedTaskId: string) => tasksApi.unshare(sharedTaskId),
    onSuccess: () => {
      invalidateQueries();
    },
    onError: (err) => {
      console.error('Failed to unshare task:', err);
    },
  });

  const linkSharedTaskToLocal = useMutation({
    mutationFn: (data: SharedTaskDetails) => tasksApi.linkToLocal(data),
    onSuccess: (createdTask: Task | null) => {
      console.log('Linked shared task to local successfully', createdTask);
      if (createdTask) {
        invalidateQueries(createdTask.id);
      }
    },
    onError: (err) => {
      console.error('Failed to link shared task to local:', err);
    },
  });

  return {
    createTask,
    createAndStart,
    updateTask,
    deleteTask,
    shareTask,
    stopShareTask: unshareSharedTask,
    linkSharedTaskToLocal,
  };
}
