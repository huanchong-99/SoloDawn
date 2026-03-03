import {
  useQuery,
  useMutation,
  useQueryClient,
  UseQueryResult,
} from '@tanstack/react-query';
import { handleApiResponse, logApiError } from '@/lib/api';
import type {
  WorkflowDetailDto,
  WorkflowListItemDto,
  WorkflowTaskDto,
} from 'shared/types';

// Type alias for convenience
export type Workflow = WorkflowDetailDto;

// ============================================================================
// Create Request Types (not in generated types yet)
// ============================================================================

export type WorkflowStatusEnum =
  | 'draft'
  | 'created'
  | 'ready'
  | 'starting'
  | 'running'
  | 'paused'
  | 'merging'
  | 'completed'
  | 'failed'
  | 'cancelled';

export interface WorkflowActions {
  canPrepare: boolean; // created → starting → ready (启动终端)
  canStart: boolean; // ready → running (开始任务)
  canPause: boolean;
  canStop: boolean;
  canMerge: boolean;
  canDelete: boolean;
}

export const WORKFLOW_STATUS_TRANSITIONS: Record<
  WorkflowStatusEnum,
  WorkflowActions
> = {
  draft: {
    canPrepare: true,
    canStart: false,
    canPause: false,
    canStop: false,
    canMerge: false,
    canDelete: true,
  },
  created: {
    canPrepare: true,
    canStart: false,
    canPause: false,
    canStop: false,
    canMerge: false,
    canDelete: true,
  },
  ready: {
    canPrepare: false,
    canStart: true,
    canPause: false,
    canStop: false,
    canMerge: false,
    canDelete: true,
  },
  starting: {
    canPrepare: false,
    canStart: false,
    canPause: false,
    canStop: true,
    canMerge: false,
    canDelete: false,
  },
  running: {
    canPrepare: false,
    canStart: false,
    canPause: true,
    canStop: true,
    canMerge: false,
    canDelete: false,
  },
  paused: {
    canPrepare: false,
    canStart: true,
    canPause: false,
    canStop: true,
    canMerge: false,
    canDelete: true,
  },
  merging: {
    canPrepare: false,
    canStart: false,
    canPause: false,
    canStop: true,
    canMerge: true,
    canDelete: false,
  },
  completed: {
    canPrepare: false,
    canStart: false,
    canPause: false,
    canStop: false,
    canMerge: true,
    canDelete: true,
  },
  failed: {
    canPrepare: true,
    canStart: false,
    canPause: false,
    canStop: false,
    canMerge: false,
    canDelete: true,
  },
  cancelled: {
    canPrepare: false,
    canStart: false,
    canPause: false,
    canStop: false,
    canMerge: false,
    canDelete: true,
  },
};

export function getWorkflowActions(
  status: WorkflowStatusEnum
): WorkflowActions {
  return (
    WORKFLOW_STATUS_TRANSITIONS[status] ?? WORKFLOW_STATUS_TRANSITIONS.created
  );
}

// ============================================================================
// Create Request Types (not in generated types yet)
// ============================================================================

export interface InlineModelConfig {
  displayName: string;
  modelId: string;
}

export interface CreateWorkflowRequest {
  projectId: string;
  name: string;
  description?: string;
  useSlashCommands?: boolean;
  commandPresetIds?: string[];
  commands?: Array<{
    presetId: string;
    orderIndex: number;
    customParams?: string | null;
  }>;
  orchestratorConfig?: {
    apiType: string;
    baseUrl: string;
    apiKey: string;
    model: string;
  };
  errorTerminalConfig?: {
    cliTypeId: string;
    modelConfigId: string;
    modelConfig?: InlineModelConfig;
    customBaseUrl?: string | null;
    customApiKey?: string | null;
  };
  mergeTerminalConfig?: {
    cliTypeId: string;
    modelConfigId: string;
    modelConfig?: InlineModelConfig;
    customBaseUrl?: string | null;
    customApiKey?: string | null;
  };
  targetBranch?: string;
  gitWatcherEnabled?: boolean;
  tasks?: Array<{
    id?: string;
    name: string;
    description?: string;
    branch?: string;
    orderIndex: number;
    terminals: Array<{
      id?: string;
      cliTypeId: string;
      modelConfigId: string;
      modelConfig?: InlineModelConfig;
      customBaseUrl?: string | null;
      customApiKey?: string | null;
      role?: string;
      roleDescription?: string;
      autoConfirm?: boolean;
      orderIndex: number;
    }>;
  }>;
}

export interface StartWorkflowRequest {
  workflow_id: string; // Note: API still uses snake_case for IDs
}

export interface PauseWorkflowRequest {
  workflow_id: string;
}

export interface StopWorkflowRequest {
  workflow_id: string;
}

export interface MergeWorkflowRequest {
  workflow_id: string;
  merge_strategy?: 'squash';
}

export interface SubmitWorkflowPromptResponseRequest {
  workflow_id: string;
  terminal_id: string;
  response: string;
  prompt_id?: string;
  decision?: string;
  decision_detail?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface WorkflowExecution {
  execution_id: string;
  workflow_id: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  started_at: string;
  completed_at?: string;
  error?: string;
}

export interface WorkflowMergeResult {
  success: boolean;
  message: string;
  workflow_id?: string;
  workflowId: string;
  targetBranch: string;
  mergedTasks: Array<{
    taskId: string;
    branch: string;
    commitSha: string;
  }>;
}

// ============================================================================
// Query Keys
// ============================================================================

export const workflowKeys = {
  all: ['workflows'] as const,
  forProject: (projectId: string) =>
    ['workflows', 'project', projectId] as const,
  byId: (workflowId: string) => ['workflows', 'detail', workflowId] as const,
};

interface UseWorkflowOptions {
  refetchInterval?: number | false;
  staleTime?: number;
  retry?: number | boolean;
}

// ============================================================================
// Workflow API
// ============================================================================

const workflowsApi = {
  /**
   * Get all workflows for a project
   */
  getForProject: async (projectId: string): Promise<WorkflowListItemDto[]> => {
    const response = await fetch(
      `/api/workflows?project_id=${encodeURIComponent(projectId)}`
    );
    return handleApiResponse<WorkflowListItemDto[]>(response);
  },

  /**
   * Get a single workflow by ID
   */
  getById: async (workflowId: string): Promise<Workflow> => {
    const response = await fetch(
      `/api/workflows/${encodeURIComponent(workflowId)}`
    );
    return handleApiResponse<Workflow>(response);
  },

  /**
   * Create a new workflow
   */
  create: async (data: CreateWorkflowRequest): Promise<Workflow> => {
    const response = await fetch('/api/workflows', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    return handleApiResponse<Workflow>(response);
  },

  /**
   * Prepare a workflow (start terminals, created → starting → ready)
   */
  prepare: async (workflowId: string): Promise<void> => {
    const response = await fetch(
      `/api/workflows/${encodeURIComponent(workflowId)}/prepare`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      }
    );
    return handleApiResponse<void>(response);
  },

  /**
   * Start a workflow execution (ready → running)
   */
  start: async (data: StartWorkflowRequest): Promise<WorkflowExecution> => {
    const response = await fetch(
      `/api/workflows/${encodeURIComponent(data.workflow_id)}/start`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      }
    );
    return handleApiResponse<WorkflowExecution>(response);
  },

  /**
   * Pause a running workflow
   */
  pause: async (data: PauseWorkflowRequest): Promise<WorkflowExecution> => {
    const response = await fetch(
      `/api/workflows/${encodeURIComponent(data.workflow_id)}/pause`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      }
    );
    return handleApiResponse<WorkflowExecution>(response);
  },

  /**
   * Stop a workflow
   */
  stop: async (data: StopWorkflowRequest): Promise<WorkflowExecution> => {
    const response = await fetch(
      `/api/workflows/${encodeURIComponent(data.workflow_id)}/stop`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      }
    );
    return handleApiResponse<WorkflowExecution>(response);
  },

  /**
   * Submit user response for an interactive prompt
   */
  submitPromptResponse: async (
    data: SubmitWorkflowPromptResponseRequest
  ): Promise<void> => {
    const response = await fetch(
      `/api/workflows/${encodeURIComponent(data.workflow_id)}/prompts/respond`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          terminal_id: data.terminal_id,
          response: data.response,
        }),
      }
    );
    return handleApiResponse<void>(response);
  },

  /**
   * Merge workflow task branches into target branch
   */
  merge: async (data: MergeWorkflowRequest): Promise<WorkflowMergeResult> => {
    const response = await fetch(
      `/api/workflows/${encodeURIComponent(data.workflow_id)}/merge`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          merge_strategy: data.merge_strategy ?? 'squash',
        }),
      }
    );
    return handleApiResponse<WorkflowMergeResult>(response);
  },

  /**
   * Delete a workflow
   */
  delete: async (workflowId: string): Promise<void> => {
    const response = await fetch(
      `/api/workflows/${encodeURIComponent(workflowId)}`,
      {
        method: 'DELETE',
      }
    );
    return handleApiResponse<void>(response);
  },

  /**
   * Update a task's status within a workflow
   */
  updateTaskStatus: async (
    workflowId: string,
    taskId: string,
    status: string
  ): Promise<WorkflowTaskDto> => {
    const response = await fetch(
      `/api/workflows/${encodeURIComponent(workflowId)}/tasks/${encodeURIComponent(taskId)}/status`,
      {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status }),
      }
    );
    return handleApiResponse<WorkflowTaskDto>(response);
  },
};

// ============================================================================
// Hooks
// ============================================================================

/**
 * Hook to fetch all workflows for a project
 * @param projectId - The project ID to fetch workflows for
 * @returns Query result with workflows array
 */
export function useWorkflows(
  projectId: string
): UseQueryResult<WorkflowListItemDto[], Error> {
  return useQuery({
    queryKey: workflowKeys.forProject(projectId),
    queryFn: () => workflowsApi.getForProject(projectId),
    enabled: !!projectId,
    staleTime: 1000 * 60 * 5, // 5 minutes
  });
}

/**
 * Hook to fetch a single workflow by ID
 * @param workflowId - The workflow ID to fetch
 * @returns Query result with workflow details
 */
export function useWorkflow(
  workflowId: string,
  options?: UseWorkflowOptions
): UseQueryResult<Workflow, Error> {
  return useQuery({
    queryKey: workflowKeys.byId(workflowId),
    queryFn: () => workflowsApi.getById(workflowId),
    enabled: !!workflowId,
    staleTime: options?.staleTime ?? 1000 * 60 * 5, // default 5 minutes
    retry: options?.retry ?? 3,
    refetchInterval: (query) => {
      if (!options?.refetchInterval) {
        return false;
      }
      return query.state.error ? false : options.refetchInterval;
    },
  });
}

/**
 * Hook to create a new workflow
 * @returns Mutation object for creating workflows
 */
export function useCreateWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateWorkflowRequest) => workflowsApi.create(data),
    onSuccess: (newWorkflow, variables) => {
      // Invalidate the project's workflows list
      queryClient.invalidateQueries({
        queryKey: workflowKeys.forProject(variables.projectId),
      });
      // Add the new workflow to the cache
      queryClient.setQueryData(workflowKeys.byId(newWorkflow.id), newWorkflow);
    },
    onError: (error) => {
      logApiError('Failed to create workflow:', error);
    },
  });
}

/**
 * Hook to prepare a workflow (start terminals, created → starting → ready)
 * @returns Mutation object for preparing workflows
 */
export function usePrepareWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (workflowId: string) => workflowsApi.prepare(workflowId),
    onSuccess: (_result, workflowId) => {
      // Invalidate the workflow detail to reflect the new status
      queryClient.invalidateQueries({
        queryKey: workflowKeys.byId(workflowId),
      });
      queryClient.invalidateQueries({
        queryKey: workflowKeys.all,
      });
    },
    onError: (error) => {
      logApiError('Failed to prepare workflow:', error);
    },
  });
}

/**
 * Hook to start a workflow execution (ready → running)
 * @returns Mutation object for starting workflows
 */
export function useStartWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: StartWorkflowRequest) => workflowsApi.start(data),
    onSuccess: (_result, variables) => {
      // Invalidate the workflow detail to reflect the new status
      queryClient.invalidateQueries({
        queryKey: workflowKeys.byId(variables.workflow_id),
      });
      queryClient.invalidateQueries({
        queryKey: workflowKeys.all,
      });
    },
    onError: (error) => {
      logApiError('Failed to start workflow:', error);
    },
  });
}

/**
 * Hook to pause a workflow
 * @returns Mutation object for pausing workflows
 */
export function usePauseWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: PauseWorkflowRequest) => workflowsApi.pause(data),
    onSuccess: (_result, variables) => {
      queryClient.invalidateQueries({
        queryKey: workflowKeys.byId(variables.workflow_id),
      });
      queryClient.invalidateQueries({
        queryKey: workflowKeys.all,
      });
    },
    onError: (error) => {
      logApiError('Failed to pause workflow:', error);
    },
  });
}

/**
 * Hook to stop a workflow
 * @returns Mutation object for stopping workflows
 */
export function useStopWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: StopWorkflowRequest) => workflowsApi.stop(data),
    onSuccess: (_result, variables) => {
      queryClient.invalidateQueries({
        queryKey: workflowKeys.byId(variables.workflow_id),
      });
      queryClient.invalidateQueries({
        queryKey: workflowKeys.all,
      });
    },
    onError: (error) => {
      logApiError('Failed to stop workflow:', error);
    },
  });
}

/**
 * Hook to submit user response for an interactive workflow prompt
 * @returns Mutation object for submitting prompt responses
 */
export function useSubmitWorkflowPromptResponse() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: SubmitWorkflowPromptResponseRequest) =>
      workflowsApi.submitPromptResponse(data),
    onSuccess: (_result, variables) => {
      queryClient.invalidateQueries({
        queryKey: workflowKeys.byId(variables.workflow_id),
      });
    },
    onError: (error) => {
      logApiError('Failed to submit workflow prompt response:', error);
    },
  });
}

/**
 * Hook to merge workflow task branches
 * @returns Mutation object for merging workflow branches
 */
export function useMergeWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: MergeWorkflowRequest) => workflowsApi.merge(data),
    onSuccess: (_result, variables) => {
      queryClient.invalidateQueries({
        queryKey: workflowKeys.byId(variables.workflow_id),
      });
      queryClient.invalidateQueries({
        queryKey: workflowKeys.all,
      });
    },
    onError: (error) => {
      logApiError('Failed to merge workflow:', error);
    },
  });
}

/**
 * Hook to delete a workflow
 * @returns Mutation object for deleting workflows
 */
export function useDeleteWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (workflowId: string) => workflowsApi.delete(workflowId),
    onSuccess: (_, workflowId) => {
      // Remove the workflow from the cache
      queryClient.removeQueries({
        queryKey: workflowKeys.byId(workflowId),
      });
      // Invalidate all workflows queries (we don't have project_id here)
      queryClient.invalidateQueries({
        queryKey: workflowKeys.all,
      });
    },
    onError: (error) => {
      logApiError('Failed to delete workflow:', error);
    },
  });
}

/**
 * Hook to update a task's status within a workflow (for Kanban drag-and-drop)
 * @returns Mutation object for updating task status
 */
export function useUpdateTaskStatus() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      workflowId,
      taskId,
      status,
    }: {
      workflowId: string;
      taskId: string;
      status: string;
    }) => workflowsApi.updateTaskStatus(workflowId, taskId, status),
    onMutate: async ({ workflowId, taskId, status }) => {
      // Cancel any outgoing refetches
      await queryClient.cancelQueries({
        queryKey: workflowKeys.byId(workflowId),
      });

      // Snapshot the previous value
      const previousWorkflow = queryClient.getQueryData<Workflow>(
        workflowKeys.byId(workflowId)
      );

      // Optimistically update the cache
      if (previousWorkflow) {
        queryClient.setQueryData<Workflow>(workflowKeys.byId(workflowId), {
          ...previousWorkflow,
          tasks: previousWorkflow.tasks.map((task) =>
            task.id === taskId ? { ...task, status } : task
          ),
        });
      }

      return { previousWorkflow };
    },
    onError: (error, { workflowId }, context) => {
      // Rollback on error
      if (context?.previousWorkflow) {
        queryClient.setQueryData(
          workflowKeys.byId(workflowId),
          context.previousWorkflow
        );
      }
      logApiError('Failed to update task status:', error);
    },
    onSettled: (_, __, { workflowId }) => {
      // Refetch to ensure consistency
      queryClient.invalidateQueries({
        queryKey: workflowKeys.byId(workflowId),
      });
    },
  });
}

// Export types for convenience
export type { WorkflowListItemDto, WorkflowTaskDto, TerminalDto, WorkflowCommandDto, SlashCommandPresetDto } from 'shared/types';
