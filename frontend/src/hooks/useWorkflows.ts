import {
  useQuery,
  useMutation,
  useQueryClient,
  UseQueryResult,
} from '@tanstack/react-query';
import { handleApiResponse, logApiError, makeRequest, ApiError } from '@/lib/api';
import { getErrorMessage } from '@/lib/modals';
import { useToast } from '@/components/ui/toast';
import type {
  WorkflowDetailDto,
  WorkflowListItemDto,
  WorkflowTaskDto,
} from 'shared/types';

// Type alias for convenience
export type Workflow = WorkflowDetailDto;

// G30-006: Retry function that skips retry for 4xx client errors.
// Only retries on 5xx server errors and network failures.
function shouldRetryOnServerError(failureCount: number, error: Error): boolean {
  if (error instanceof ApiError && error.status !== undefined && error.status >= 400 && error.status < 500) {
    return false;
  }
  return failureCount < 3;
}

// ============================================================================
// Create Request Types (not in generated types yet)
// ============================================================================

// G02-007: Backend WorkflowStatus enum does NOT include 'draft'.
// 'draft' is a client-only status used in the wizard before the workflow is
// persisted.  We split the types so callers that talk to the backend never
// accidentally send 'draft'.
export type BackendWorkflowStatus =
  | 'created'
  | 'ready'
  | 'starting'
  | 'running'
  | 'paused'
  | 'merging'
  | 'completed'
  | 'failed'
  | 'cancelled';

/** Client-side superset that adds the wizard-only 'draft' status. */
export type WorkflowStatusEnum = BackendWorkflowStatus | 'draft';

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
    canPrepare: false,
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
    canStop: false,
    canMerge: false,
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
  executionMode?: 'diy' | 'agent_planned';
  initialGoal?: string;
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

export interface SubmitOrchestratorChatRequest {
  workflow_id: string;
  message: string;
  source?: 'web' | 'api' | 'social';
  externalMessageId?: string;
}

export interface SubmitOrchestratorChatResponse {
  command_id: string;
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled';
  error?: string | null;
  retryable: boolean;
}

export interface OrchestratorChatMessage {
  role: string;
  content: string;
}

export interface ListOrchestratorMessagesParams {
  cursor?: number;
  limit?: number;
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

function normalizeWorkflowDetail(workflow: Workflow): Workflow {
  return {
    ...workflow,
    tasks: Array.isArray(workflow.tasks)
      ? workflow.tasks.map((task) => ({
          ...task,
          terminals: Array.isArray(task.terminals) ? task.terminals : [],
        }))
      : [],
    commands: Array.isArray(workflow.commands) ? workflow.commands : [],
  };
}

// ============================================================================
// Workflow API
// ============================================================================

const workflowsApi = {
  /**
   * Get all workflows for a project
   */
  getForProject: async (projectId: string): Promise<WorkflowListItemDto[]> => {
    const response = await makeRequest(
      `/api/workflows?project_id=${encodeURIComponent(projectId)}`
    );
    return handleApiResponse<WorkflowListItemDto[]>(response);
  },

  /**
   * Get a single workflow by ID
   */
  getById: async (workflowId: string): Promise<Workflow> => {
    const response = await makeRequest(
      `/api/workflows/${encodeURIComponent(workflowId)}`
    );
    const workflow = await handleApiResponse<Workflow>(response);
    return normalizeWorkflowDetail(workflow);
  },

  /**
   * Create a new workflow
   */
  create: async (data: CreateWorkflowRequest): Promise<Workflow> => {
    const response = await makeRequest('/api/workflows', {
      method: 'POST',
      body: JSON.stringify(data),
    });
    const workflow = await handleApiResponse<Workflow>(response);
    return normalizeWorkflowDetail(workflow);
  },

  /**
   * Prepare a workflow (start terminals, created → starting → ready)
   */
  prepare: async (workflowId: string): Promise<void> => {
    const response = await makeRequest(
      `/api/workflows/${encodeURIComponent(workflowId)}/prepare`,
      {
        method: 'POST',
      }
    );
    return handleApiResponse<void>(response);
  },

  /**
   * Start a workflow execution (ready → running)
   */
  start: async (data: StartWorkflowRequest): Promise<WorkflowExecution> => {
    const response = await makeRequest(
      `/api/workflows/${encodeURIComponent(data.workflow_id)}/start`,
      {
        method: 'POST',
      }
    );
    return handleApiResponse<WorkflowExecution>(response);
  },

  /**
   * Pause a running workflow
   */
  pause: async (data: PauseWorkflowRequest): Promise<WorkflowExecution> => {
    const response = await makeRequest(
      `/api/workflows/${encodeURIComponent(data.workflow_id)}/pause`,
      {
        method: 'POST',
      }
    );
    return handleApiResponse<WorkflowExecution>(response);
  },

  /**
   * Stop a workflow
   */
  stop: async (data: StopWorkflowRequest): Promise<WorkflowExecution> => {
    const response = await makeRequest(
      `/api/workflows/${encodeURIComponent(data.workflow_id)}/stop`,
      {
        method: 'POST',
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
    const { workflow_id, ...body } = data;
    const response = await makeRequest(
      `/api/workflows/${encodeURIComponent(workflow_id)}/prompts/respond`,
      {
        method: 'POST',
        body: JSON.stringify(body),
      }
    );
    return handleApiResponse<void>(response);
  },

  /**
   * Submit a direct chat message to the workflow orchestrator.
   */
  submitOrchestratorChat: async (
    data: SubmitOrchestratorChatRequest
  ): Promise<SubmitOrchestratorChatResponse> => {
    const response = await makeRequest(
      `/api/workflows/${encodeURIComponent(data.workflow_id)}/orchestrator/chat`,
      {
        method: 'POST',
        body: JSON.stringify({
          message: data.message,
          source: data.source ?? 'web',
          externalMessageId: data.externalMessageId,
        }),
      }
    );
    return handleApiResponse<SubmitOrchestratorChatResponse>(response);
  },

  /**
   * List orchestrator conversation messages for a workflow.
   */
  getOrchestratorMessages: async (
    workflowId: string,
    params?: ListOrchestratorMessagesParams
  ): Promise<OrchestratorChatMessage[]> => {
    const query = new URLSearchParams();
    if (typeof params?.cursor === 'number') {
      query.set('cursor', String(params.cursor));
    }
    if (typeof params?.limit === 'number') {
      query.set('limit', String(params.limit));
    }
    const querySuffix = query.toString() ? `?${query.toString()}` : '';
    const response = await makeRequest(
      `/api/workflows/${encodeURIComponent(workflowId)}/orchestrator/messages${querySuffix}`
    );
    return handleApiResponse<OrchestratorChatMessage[]>(response);
  },

  /**
   * Merge workflow task branches into target branch
   */
  merge: async (data: MergeWorkflowRequest): Promise<WorkflowMergeResult> => {
    const response = await makeRequest(
      `/api/workflows/${encodeURIComponent(data.workflow_id)}/merge`,
      {
        method: 'POST',
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
    const response = await makeRequest(
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
    const response = await makeRequest(
      `/api/workflows/${encodeURIComponent(workflowId)}/tasks/${encodeURIComponent(taskId)}/status`,
      {
        method: 'PUT',
        body: JSON.stringify({ status }),
      }
    );
    return handleApiResponse<WorkflowTaskDto>(response);
  },
};

// ============================================================================
// Helpers
// ============================================================================

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
    retry: options?.retry ?? shouldRetryOnServerError,
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
  const { showToast } = useToast();

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
    onError: (error: Error, variables) => {
      logApiError('Failed to create workflow:', error);
      showToast(getErrorMessage(error), 'error');
      // G26-006: Invalidate cache on error to ensure consistency
      queryClient.invalidateQueries({
        queryKey: workflowKeys.forProject(variables.projectId),
      });
    },
  });
}

/**
 * Hook to prepare a workflow (start terminals, created → starting → ready)
 * @returns Mutation object for preparing workflows
 */
export function usePrepareWorkflow() {
  const queryClient = useQueryClient();
  const { showToast } = useToast();

  return useMutation({
    mutationFn: (workflowId: string) => workflowsApi.prepare(workflowId),
    // G26-003: Optimistic update — immediately reflect 'starting' status
    onMutate: async (workflowId) => {
      await queryClient.cancelQueries({ queryKey: workflowKeys.byId(workflowId) });
      const previous = queryClient.getQueryData<Workflow>(workflowKeys.byId(workflowId));
      if (previous) {
        queryClient.setQueryData<Workflow>(workflowKeys.byId(workflowId), {
          ...previous,
          status: 'starting',
        });
      }
      return { previous };
    },
    // G02-004 / G26-005 / G26-006: Rollback + invalidate + toast on error
    onError: (error: Error, workflowId, context) => {
      logApiError('Failed to prepare workflow:', error);
      showToast(getErrorMessage(error), 'error');
      if (context?.previous) {
        queryClient.setQueryData(workflowKeys.byId(workflowId), context.previous);
      }
    },
    // G26-012: Invalidate only the specific workflow detail on settle
    onSettled: (_data, _error, workflowId) => {
      queryClient.invalidateQueries({ queryKey: workflowKeys.byId(workflowId) });
    },
  });
}

/**
 * Hook to start a workflow execution (ready → running)
 * @returns Mutation object for starting workflows
 */
export function useStartWorkflow() {
  const queryClient = useQueryClient();
  const { showToast } = useToast();

  return useMutation({
    mutationFn: (data: StartWorkflowRequest) => workflowsApi.start(data),
    // G26-003: Optimistic update — immediately reflect 'running' status
    onMutate: async (variables) => {
      await queryClient.cancelQueries({ queryKey: workflowKeys.byId(variables.workflow_id) });
      const previous = queryClient.getQueryData<Workflow>(workflowKeys.byId(variables.workflow_id));
      if (previous) {
        queryClient.setQueryData<Workflow>(workflowKeys.byId(variables.workflow_id), {
          ...previous,
          status: 'running',
        });
      }
      return { previous };
    },
    // G26-006: Rollback + toast on error
    onError: (error: Error, variables, context) => {
      logApiError('Failed to start workflow:', error);
      showToast(getErrorMessage(error), 'error');
      if (context?.previous) {
        queryClient.setQueryData(workflowKeys.byId(variables.workflow_id), context.previous);
      }
    },
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({ queryKey: workflowKeys.byId(variables.workflow_id) });
      queryClient.invalidateQueries({ queryKey: workflowKeys.all });
    },
  });
}

/**
 * Hook to pause a workflow
 * @returns Mutation object for pausing workflows
 */
export function usePauseWorkflow() {
  const queryClient = useQueryClient();
  const { showToast } = useToast();

  return useMutation({
    mutationFn: (data: PauseWorkflowRequest) => workflowsApi.pause(data),
    // G05-009 / G26-003: Optimistic update — immediately reflect 'paused' status
    onMutate: async (variables) => {
      await queryClient.cancelQueries({ queryKey: workflowKeys.byId(variables.workflow_id) });
      const previous = queryClient.getQueryData<Workflow>(workflowKeys.byId(variables.workflow_id));
      if (previous) {
        queryClient.setQueryData<Workflow>(workflowKeys.byId(variables.workflow_id), {
          ...previous,
          status: 'paused',
        });
      }
      return { previous };
    },
    // G26-006: Rollback + toast on error
    onError: (error: Error, variables, context) => {
      logApiError('Failed to pause workflow:', error);
      showToast(getErrorMessage(error), 'error');
      if (context?.previous) {
        queryClient.setQueryData(workflowKeys.byId(variables.workflow_id), context.previous);
      }
    },
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({ queryKey: workflowKeys.byId(variables.workflow_id) });
      queryClient.invalidateQueries({ queryKey: workflowKeys.all });
    },
  });
}

/**
 * Hook to stop a workflow
 * @returns Mutation object for stopping workflows
 */
export function useStopWorkflow() {
  const queryClient = useQueryClient();
  const { showToast } = useToast();

  return useMutation({
    mutationFn: (data: StopWorkflowRequest) => workflowsApi.stop(data),
    // G05-009 / G26-003: Optimistic update — immediately reflect 'cancelled' status
    onMutate: async (variables) => {
      await queryClient.cancelQueries({ queryKey: workflowKeys.byId(variables.workflow_id) });
      const previous = queryClient.getQueryData<Workflow>(workflowKeys.byId(variables.workflow_id));
      if (previous) {
        queryClient.setQueryData<Workflow>(workflowKeys.byId(variables.workflow_id), {
          ...previous,
          status: 'cancelled',
        });
      }
      return { previous };
    },
    // G26-006: Rollback + toast on error
    onError: (error: Error, variables, context) => {
      logApiError('Failed to stop workflow:', error);
      showToast(getErrorMessage(error), 'error');
      if (context?.previous) {
        queryClient.setQueryData(workflowKeys.byId(variables.workflow_id), context.previous);
      }
    },
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({ queryKey: workflowKeys.byId(variables.workflow_id) });
      queryClient.invalidateQueries({ queryKey: workflowKeys.all });
    },
  });
}

/**
 * Hook to submit user response for an interactive workflow prompt
 * @returns Mutation object for submitting prompt responses
 */
export function useSubmitWorkflowPromptResponse() {
  const queryClient = useQueryClient();
  const { showToast } = useToast();

  return useMutation({
    mutationFn: (data: SubmitWorkflowPromptResponseRequest) =>
      workflowsApi.submitPromptResponse(data),
    // G26-006 / G30-004: Invalidate cache + toast on error
    onError: (error: Error, variables) => {
      logApiError('Failed to submit workflow prompt response:', error);
      showToast(getErrorMessage(error), 'error');
      queryClient.invalidateQueries({
        queryKey: workflowKeys.byId(variables.workflow_id),
      });
    },
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({
        queryKey: workflowKeys.byId(variables.workflow_id),
      });
    },
  });
}

/**
 * Hook to send a direct chat message to orchestrator
 * @returns Mutation object for sending orchestrator chat messages
 */
export function useSubmitOrchestratorChat() {
  const queryClient = useQueryClient();
  const { showToast } = useToast();

  return useMutation({
    mutationFn: (data: SubmitOrchestratorChatRequest) =>
      workflowsApi.submitOrchestratorChat(data),
    // G26-006 / G30-004: Invalidate cache + toast on error
    onError: (error: Error, variables) => {
      logApiError('Failed to submit orchestrator chat message:', error);
      showToast(getErrorMessage(error), 'error');
      queryClient.invalidateQueries({
        queryKey: workflowKeys.byId(variables.workflow_id),
      });
    },
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({
        queryKey: workflowKeys.byId(variables.workflow_id),
      });
    },
  });
}

interface UseOrchestratorMessagesOptions {
  enabled?: boolean;
  refetchInterval?: number | false;
  cursor?: number;
  limit?: number;
}

/**
 * Hook to fetch orchestrator conversation messages.
 */
export function useOrchestratorMessages(
  workflowId: string,
  options?: UseOrchestratorMessagesOptions
): UseQueryResult<OrchestratorChatMessage[], Error> {
  return useQuery({
    queryKey: [
      ...workflowKeys.byId(workflowId),
      'orchestratorMessages',
      options?.cursor ?? null,
      options?.limit ?? null,
    ],
    queryFn: () =>
      workflowsApi.getOrchestratorMessages(workflowId, {
        cursor: options?.cursor,
        limit: options?.limit,
      }),
    enabled: Boolean(workflowId) && (options?.enabled ?? true),
    refetchInterval: options?.refetchInterval ?? false,
    staleTime: 1000,
  });
}

/**
 * Hook to merge workflow task branches
 * @returns Mutation object for merging workflow branches
 */
export function useMergeWorkflow() {
  const queryClient = useQueryClient();
  const { showToast } = useToast();

  return useMutation({
    mutationFn: (data: MergeWorkflowRequest) => workflowsApi.merge(data),
    // G06-010 / G26-003: Optimistic update — immediately reflect 'merging' status
    onMutate: async (variables) => {
      await queryClient.cancelQueries({ queryKey: workflowKeys.byId(variables.workflow_id) });
      const previous = queryClient.getQueryData<Workflow>(workflowKeys.byId(variables.workflow_id));
      if (previous) {
        queryClient.setQueryData<Workflow>(workflowKeys.byId(variables.workflow_id), {
          ...previous,
          status: 'merging',
        });
      }
      return { previous };
    },
    // G26-006: Rollback + toast on error
    onError: (error: Error, variables, context) => {
      logApiError('Failed to merge workflow:', error);
      showToast(getErrorMessage(error), 'error');
      if (context?.previous) {
        queryClient.setQueryData(workflowKeys.byId(variables.workflow_id), context.previous);
      }
    },
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({ queryKey: workflowKeys.byId(variables.workflow_id) });
      queryClient.invalidateQueries({ queryKey: workflowKeys.all });
    },
  });
}

/**
 * Hook to delete a workflow
 * @returns Mutation object for deleting workflows
 */
export function useDeleteWorkflow() {
  const queryClient = useQueryClient();
  const { showToast } = useToast();

  return useMutation({
    mutationFn: (workflowId: string) => workflowsApi.delete(workflowId),
    onSuccess: (_, workflowId) => {
      // Remove the workflow from the cache
      queryClient.removeQueries({
        queryKey: workflowKeys.byId(workflowId),
      });
    },
    // G26-006 / G30-004: Invalidate cache + toast on error
    onError: (error: Error, workflowId) => {
      logApiError('Failed to delete workflow:', error);
      showToast(getErrorMessage(error), 'error');
      queryClient.invalidateQueries({ queryKey: workflowKeys.byId(workflowId) });
    },
    onSettled: () => {
      // Invalidate all workflows queries (we don't have project_id here)
      queryClient.invalidateQueries({ queryKey: workflowKeys.all });
    },
  });
}

/**
 * Hook to update a task's status within a workflow (for Kanban drag-and-drop)
 * @returns Mutation object for updating task status
 */
export function useUpdateTaskStatus() {
  const queryClient = useQueryClient();
  const { showToast } = useToast();

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
          tasks: (previousWorkflow.tasks ?? []).map((task) =>
            task.id === taskId ? { ...task, status } : task
          ),
        });
      }

      return { previousWorkflow };
    },
    onError: (error: Error, { workflowId }, context) => {
      // Rollback on error
      if (context?.previousWorkflow) {
        queryClient.setQueryData(
          workflowKeys.byId(workflowId),
          context.previousWorkflow
        );
      }
      logApiError('Failed to update task status:', error);
      showToast(getErrorMessage(error), 'error');
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
