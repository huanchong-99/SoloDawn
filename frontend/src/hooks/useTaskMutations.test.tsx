import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ReactNode } from 'react';
import type { TaskWithAttemptStatus } from 'shared/types';
import { useTaskMutations } from './useTaskMutations';
import { taskKeys } from './useTask';
import { workspaceSummaryKeys } from '@/components/ui-new/hooks/useWorkspaces';

const { mockNavigate, mockCreateAndStart, mockShare } = vi.hoisted(() => ({
  mockNavigate: vi.fn(),
  mockCreateAndStart: vi.fn(),
  mockShare: vi.fn(),
}));

vi.mock('@/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/hooks')>('@/hooks');
  return {
    ...actual,
    useNavigateWithSearch: () => mockNavigate,
  };
});

vi.mock('@/hooks/auth/useAuth', () => ({
  useAuth: () => ({ isSignedIn: true }),
}));

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api');
  return {
    ...actual,
    tasksApi: {
      ...actual.tasksApi,
      createAndStart: mockCreateAndStart,
      share: mockShare,
    },
  };
});

const createWrapper = (queryClient: QueryClient) => {
  return function Wrapper({ children }: Readonly<{ children: ReactNode }>) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
};

const createdTask: TaskWithAttemptStatus = {
  id: 'task-101',
  projectId: 'project-1',
  title: 'New task',
  description: null,
  status: 'todo',
  parentWorkspaceId: null,
  sharedTaskId: null,
  hasInProgressAttempt: true,
  lastAttemptFailed: false,
  executor: 'claude-code',
  createdAt: '2026-02-08T00:00:00Z',
  updatedAt: '2026-02-08T00:00:00Z',
};

describe('useTaskMutations', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('invalidates task and workspace summary cache after createAndStart', async () => {
    mockCreateAndStart.mockResolvedValue(createdTask);

    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });

    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');
    const { result } = renderHook(() => useTaskMutations('project-1'), {
      wrapper: createWrapper(queryClient),
    });

    await result.current.createAndStart.mutateAsync({
      task: {
        projectId: 'project-1',
        title: 'New task',
        description: null,
        status: null,
        parentWorkspaceId: null,
        imageIds: null,
        sharedTaskId: null,
      },
      executor_profile_id: {
        executor: 'CLAUDE_CODE',
        variant: null,
      },
      repos: [
        {
          repo_id: 'repo-1',
          target_branch: 'main',
        },
      ],
    });

    await waitFor(() => {
      expect(mockCreateAndStart).toHaveBeenCalledTimes(1);
    });

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: taskKeys.all });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: taskKeys.byId('task-101'),
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: workspaceSummaryKeys.all,
    });
    expect(mockNavigate).toHaveBeenCalledWith(
      '/projects/project-1/tasks/task-101/attempts/latest'
    );
  });

  it('invalidates task and workspace summary cache after shareTask', async () => {
    mockShare.mockResolvedValue({ shared_task_id: 'shared-1' });

    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });

    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');
    const { result } = renderHook(() => useTaskMutations('project-1'), {
      wrapper: createWrapper(queryClient),
    });

    await result.current.shareTask.mutateAsync('task-share-1');

    await waitFor(() => {
      expect(mockShare).toHaveBeenCalledWith('task-share-1');
    });

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: taskKeys.all });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: taskKeys.byId('task-share-1'),
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: workspaceSummaryKeys.all,
    });
  });
});
