import { describe, it, expect, beforeEach, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ToastProvider } from '@/components/ui/toast';

const { mockLogApiError } = vi.hoisted(() => ({
  mockLogApiError: vi.fn(),
}));

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api');
  return {
    ...actual,
    logApiError: mockLogApiError,
  };
});

import {
  useWorkflows,
  useWorkflow,
  useCreateWorkflow,
  usePrepareWorkflow,
  useStartWorkflow,
  usePauseWorkflow,
  useStopWorkflow,
  useSubmitWorkflowPromptResponse,
  useMergeWorkflow,
  useDeleteWorkflow,
  getWorkflowActions,
  workflowKeys,
  type Workflow,
  type CreateWorkflowRequest,
} from './useWorkflows';

// ============================================================================
// Test Utilities
// ============================================================================

const createMockQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
      mutations: {
        retry: false,
      },
    },
  });

const wrapper = ({ children }: Readonly<{ children: React.ReactNode }>) => (
  <QueryClientProvider client={createMockQueryClient()}>
    <ToastProvider>{children}</ToastProvider>
  </QueryClientProvider>
);

const createScopedWrapper = () => {
  const queryClient = createMockQueryClient();
  const scopedWrapper = ({
    children,
  }: Readonly<{ children: React.ReactNode }>) => (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  );
  return { queryClient, scopedWrapper };
};

// Helper to create successful API response
const createSuccessResponse = (data: unknown) =>
  ({
    ok: true,
    json: async () => ({ success: true, data }),
  }) as Response;

// Helper to create error API response
const createErrorResponse = (message: string, status: number = 500) =>
  ({
    ok: false,
    status,
    statusText: message,
    json: async () => ({ success: false, message }),
  }) as Response;

// Mock workflows data
const mockWorkflows: Workflow[] = [
  {
    id: 'workflow-1',
    project_id: 'project-1',
    name: 'Test Workflow 1',
    description: 'Test description',
    status: 'draft',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    config: {
      tasks: [],
      models: [],
      terminals: [],
      commands: { enabled: false, presetIds: [] },
      orchestrator: {
        modelConfigId: 'model-1',
        mergeTerminal: {
          cliTypeId: 'claude-code',
          modelConfigId: 'model-1',
          runTestsBeforeMerge: true,
          pauseOnConflict: true,
        },
        targetBranch: 'main',
      },
    },
  },
  {
    id: 'workflow-2',
    project_id: 'project-1',
    name: 'Test Workflow 2',
    description: 'Another description',
    status: 'running',
    created_at: '2024-01-02T00:00:00Z',
    updated_at: '2024-01-02T01:00:00Z',
    config: {
      tasks: [],
      models: [],
      terminals: [],
      commands: { enabled: false, presetIds: [] },
      orchestrator: {
        modelConfigId: 'model-2',
        mergeTerminal: {
          cliTypeId: 'claude-code',
          modelConfigId: 'model-2',
          runTestsBeforeMerge: false,
          pauseOnConflict: false,
        },
        targetBranch: 'main',
      },
    },
  },
];

const mockWorkflow: Workflow = {
  id: 'workflow-1',
  project_id: 'project-1',
  name: 'Test Workflow 1',
  description: 'Test description',
  status: 'draft',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  tasks: [],
  commands: [],
  config: {
    tasks: [],
    models: [],
    terminals: [],
    commands: { enabled: false, presetIds: [] },
    orchestrator: {
      modelConfigId: 'model-1',
      mergeTerminal: {
        cliTypeId: 'claude-code',
        modelConfigId: 'model-1',
        runTestsBeforeMerge: true,
        pauseOnConflict: true,
      },
      targetBranch: 'main',
    },
  },
};

async function expectStatusMutationInvalidatesCaches<TVariables>(
  useMutationHook: () => {
    mutate: (variables: TVariables) => void;
    isSuccess: boolean;
  },
  variables: TVariables
) {
  const { queryClient, scopedWrapper } = createScopedWrapper();
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

  vi.stubGlobal('fetch', vi.fn(() => createSuccessResponse(undefined)));

  const { result } = renderHook(useMutationHook, {
    wrapper: scopedWrapper,
  });

  result.current.mutate(variables);

  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  expect(invalidateSpy).toHaveBeenCalledWith({
    queryKey: workflowKeys.byId('workflow-1'),
  });
  expect(invalidateSpy).toHaveBeenCalledWith({
    queryKey: workflowKeys.all,
  });
}

// ============================================================================
// Tests
// ============================================================================

beforeEach(() => {
  vi.clearAllMocks();
});

describe('useWorkflows', () => {
  it('should fetch workflows for a project', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => createSuccessResponse(mockWorkflows))
    );

    const { result } = renderHook(() => useWorkflows('proj-1'), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(mockWorkflows);
  });

  it('should handle fetch errors', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => createErrorResponse('Network error'))
    );

    const { result } = renderHook(() => useWorkflows('proj-1'), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });

  it('should be disabled when projectId is empty', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => createSuccessResponse(mockWorkflows))
    );

    const { result } = renderHook(() => useWorkflows(''), {
      wrapper,
    });

    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useWorkflow', () => {
  it('should fetch a single workflow by ID', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => createSuccessResponse(mockWorkflow))
    );

    const { result } = renderHook(() => useWorkflow('workflow-1'), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(mockWorkflow);
  });

  it('should be disabled when workflowId is empty', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => createSuccessResponse(mockWorkflow))
    );

    const { result } = renderHook(() => useWorkflow(''), {
      wrapper,
    });

    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useCreateWorkflow', () => {
  it('should create a new workflow', async () => {
    const newWorkflow: Workflow = {
      ...mockWorkflow,
      id: 'new-workflow',
    };

    const requestData: CreateWorkflowRequest = {
      projectId: 'project-1',
      name: 'New Workflow',
      description: 'New description',
      tasks: [
        {
          name: 'Task 1',
          orderIndex: 0,
          terminals: [
            {
              cliTypeId: 'claude-code',
              modelConfigId: 'model-1',
              orderIndex: 0,
              autoConfirm: true,
            },
          ],
        },
      ],
    };

    const fetchMock = vi.fn(() => createSuccessResponse(newWorkflow));
    vi.stubGlobal('fetch', fetchMock);

    const { result } = renderHook(() => useCreateWorkflow(), {
      wrapper,
    });

    result.current.mutate(requestData);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(newWorkflow);

    // Verify autoConfirm is included in request body
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, requestInit] = fetchMock.mock.calls[0] as [string, RequestInit];
    const requestBody = JSON.parse(requestInit.body as string);
    expect(requestBody.tasks[0].terminals[0].autoConfirm).toBe(true);
  });

  it('should handle creation errors', async () => {
    const requestData: CreateWorkflowRequest = {
      project_id: 'project-1',
      name: 'New Workflow',
      config: mockWorkflow.config,
    };

    vi.stubGlobal(
      'fetch',
      vi.fn(() => createErrorResponse('Creation failed'))
    );
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    const { result } = renderHook(() => useCreateWorkflow(), {
      wrapper,
    });

    result.current.mutate(requestData);

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(errorSpy).not.toHaveBeenCalled();
    errorSpy.mockRestore();
  });
});

describe('useStartWorkflow', () => {
  it('should start a workflow execution', async () => {
    const executionResponse = {
      execution_id: 'exec-1',
      workflow_id: 'workflow-1',
      status: 'running' as const,
      started_at: '2024-01-01T00:00:00Z',
    };

    vi.stubGlobal(
      'fetch',
      vi.fn(() => createSuccessResponse(executionResponse))
    );

    const { result } = renderHook(() => useStartWorkflow(), {
      wrapper,
    });

    result.current.mutate({ workflow_id: 'workflow-1' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(executionResponse);
  });
});

describe('status mutations cache invalidation', () => {
  it.each([
    {
      name: 'start',
      useHook: useStartWorkflow,
      variables: { workflow_id: 'workflow-1' },
    },
    {
      name: 'pause',
      useHook: usePauseWorkflow,
      variables: { workflow_id: 'workflow-1' },
    },
    {
      name: 'stop',
      useHook: useStopWorkflow,
      variables: { workflow_id: 'workflow-1' },
    },
  ])(
    '$name invalidates detail and list caches',
    async ({ useHook, variables }) => {
      await expectStatusMutationInvalidatesCaches(
        useHook as () => {
          mutate: (payload: unknown) => void;
          isSuccess: boolean;
        },
        variables
      );
    }
  );

  // G26-012: prepare uses narrowed invalidation (byId only, not all)
  it('prepare invalidates detail cache only', async () => {
    const { queryClient, scopedWrapper } = createScopedWrapper();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

    vi.stubGlobal('fetch', vi.fn(() => createSuccessResponse(undefined)));

    const { result } = renderHook(usePrepareWorkflow, {
      wrapper: scopedWrapper,
    });

    result.current.mutate('workflow-1');

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: workflowKeys.byId('workflow-1'),
    });
  });
});

describe('useDeleteWorkflow', () => {
  it('should delete a workflow', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => createSuccessResponse(undefined))
    );

    const { result } = renderHook(() => useDeleteWorkflow(), {
      wrapper,
    });

    result.current.mutate('workflow-1');

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useMergeWorkflow', () => {
  it('should call merge API and invalidate workflow caches', async () => {
    const mergeResponse = {
      success: true,
      message: 'Merge completed successfully',
      workflowId: 'workflow-1',
      targetBranch: 'main',
      mergedTasks: [],
    };

    const { queryClient, scopedWrapper } = createScopedWrapper();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

    const fetchMock = vi.fn(() => createSuccessResponse(mergeResponse));
    vi.stubGlobal('fetch', fetchMock);

    const { result } = renderHook(() => useMergeWorkflow(), {
      wrapper: scopedWrapper,
    });

    result.current.mutate({ workflow_id: 'workflow-1' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/workflows/workflow-1/merge',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ merge_strategy: 'squash' }),
      })
    );
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: workflowKeys.byId('workflow-1'),
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: workflowKeys.all,
    });
  });
});

describe('useSubmitWorkflowPromptResponse', () => {
  it('should submit prompt response and invalidate workflow cache', async () => {
    const { queryClient, scopedWrapper } = createScopedWrapper();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

    const fetchMock = vi.fn(() => createSuccessResponse(undefined));
    vi.stubGlobal('fetch', fetchMock);

    const { result } = renderHook(() => useSubmitWorkflowPromptResponse(), {
      wrapper: scopedWrapper,
    });

    result.current.mutate({
      workflow_id: 'workflow-1',
      terminal_id: 'terminal-1',
      response: 'yes',
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/workflows/workflow-1/prompts/respond',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          terminal_id: 'terminal-1',
          response: 'yes',
        }),
      })
    );
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: workflowKeys.byId('workflow-1'),
    });
  });

  it('should log error when submitting prompt response fails', async () => {
    vi.stubGlobal('fetch', vi.fn(() => createErrorResponse('Submit failed')));

    const { result } = renderHook(() => useSubmitWorkflowPromptResponse(), {
      wrapper,
    });

    result.current.mutate({
      workflow_id: 'workflow-1',
      terminal_id: 'terminal-1',
      response: 'yes',
    });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(mockLogApiError).toHaveBeenCalledWith(
      'Failed to submit workflow prompt response:',
      expect.any(Error)
    );
  });
});

describe('getWorkflowActions', () => {
  it('should allow merge for completed workflows', () => {
    expect(getWorkflowActions('completed').canMerge).toBe(true);
  });

  it('should disallow merge for merging, running, and draft workflows', () => {
    expect(getWorkflowActions('merging').canMerge).toBe(false);
    expect(getWorkflowActions('running').canMerge).toBe(false);
    expect(getWorkflowActions('cancelled').canMerge).toBe(false);
    expect(getWorkflowActions('draft').canMerge).toBe(false);
  });
});
