import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import {
  render,
  screen,
  waitFor,
  fireEvent,
  within,
  act,
} from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { Workflows } from './Workflows';
import type { WorkflowDetailDto, WorkflowListItemDto } from 'shared/types';
import { I18nextProvider } from 'react-i18next';
import { i18n, setTestLanguage } from '@/test/renderWithI18n';
import { ToastProvider } from '@/components/ui/toast';
import type {
  TerminalPromptDecisionPayload,
  TerminalPromptDetectedPayload,
  WorkflowEventHandlers,
} from '@/stores/wsStore';

const workflowWizardMock = vi.hoisted(() => ({
  submitConfig: {
    project: {
      workingDirectory: String.raw`E:\test\test`,
      gitStatus: { isGitRepo: true, isDirty: false },
    },
    basic: {
      name: 'Wizard Created Workflow',
      description: 'created by mocked wizard',
      taskCount: 1,
      importFromKanban: false,
    },
    tasks: [
      {
        id: 'task-0',
        name: 'Task 1',
        description: 'First task',
        branch: 'feat/task-1',
        terminalCount: 1,
      },
    ],
    models: [
      {
        id: 'model-1',
        displayName: 'Claude 3.5',
        apiType: 'anthropic',
        baseUrl: 'https://api.anthropic.com',
        apiKey: 'sk-ant-xxx',
        modelId: 'claude-3-5-sonnet-20241022',
        isVerified: true,
      },
    ],
    terminals: [
      {
        id: 'term-1',
        taskId: 'task-0',
        orderIndex: 0,
        cliTypeId: 'claude-code',
        modelConfigId: 'model-1',
      },
    ],
    commands: {
      enabled: false,
      presetIds: [],
    },
    advanced: {
      orchestrator: { modelConfigId: 'model-1' },
      errorTerminal: { enabled: false },
      mergeTerminal: {
        cliTypeId: 'claude-code',
        modelConfigId: 'model-1',
        runTestsBeforeMerge: true,
        pauseOnConflict: true,
      },
      targetBranch: 'main',
    },
  },
}));

vi.mock('@/components/workflow/WorkflowWizard', () => ({
  WorkflowWizard: ({
    onComplete,
    onCancel,
  }: {
    onComplete: (config: typeof workflowWizardMock.submitConfig) =>
      | void
      | Promise<void>;
    onCancel: () => void;
  }) => (
    <div data-testid="mock-workflow-wizard">
      <button
        data-testid="mock-workflow-wizard-submit"
        onClick={() => void onComplete(workflowWizardMock.submitConfig)}
      >
        Submit Mock Wizard
      </button>
      <button data-testid="mock-workflow-wizard-cancel" onClick={onCancel}>
        Cancel
      </button>
    </div>
  ),
}));

const wsStoreMock = vi.hoisted(() => ({
  sendPromptResponse: vi.fn(() => true),
  subscribeToWorkflow: vi.fn(() => vi.fn()),
  workflowId: null,
  handlers: undefined,
}));

// Mock useProjects hook to avoid WebSocket connection in tests
vi.mock('@/hooks/useProjects', () => ({
  useProjects: () => ({
    projects: [{ id: 'proj-1', name: 'Test Project', path: '/test' }],
    isLoading: false,
    error: null,
  }),
}));

vi.mock('@/stores/wsStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/stores/wsStore')>();
  const origUseWsStore = actual.useWsStore;
  return {
    useWorkflowEvents: vi.fn(
      (workflowId: string | null | undefined, handlers?: WorkflowEventHandlers) => {
        wsStoreMock.workflowId = workflowId;
        wsStoreMock.handlers = handlers;
        return { connectionStatus: 'connected' };
      }
    ),
    useWsStore: new Proxy(origUseWsStore, {
      apply(target, thisArg, args) {
        // When called as a hook (selector), use mock data
        return args[0]({
          sendPromptResponse: wsStoreMock.sendPromptResponse,
          subscribeToWorkflow: wsStoreMock.subscribeToWorkflow,
        });
      },
      get(target, prop) {
        if (prop === 'getState') {
          return () => ({
            subscribeToWorkflow: wsStoreMock.subscribeToWorkflow,
            sendPromptResponse: wsStoreMock.sendPromptResponse,
          });
        }
        return Reflect.get(target, prop);
      },
    }),
  };
});

vi.mock('@/components/ConfigProvider', () => ({
  useUserSystem: () => ({
    config: {
      workflow_model_library: [{ modelId: 'gpt-4.1' }],
    },
  }),
}));

// ============================================================================
// Test Utilities
// ============================================================================

const createMockQueryClient = () =>
{
  const queryClient = new QueryClient();
  queryClient.setDefaultOptions({
    queries: {
      retry: false,
    },
    mutations: {
      retry: false,
    },
  });
  return queryClient;
};

const wrapper = ({ children }: Readonly<{ children: React.ReactNode }>) => (
  <I18nextProvider i18n={i18n}>
    <ToastProvider>
      <QueryClientProvider client={createMockQueryClient()}>
        <MemoryRouter initialEntries={['/projects/proj-1/workflows']}>
          <Routes>
            <Route path="/projects/:projectId/workflows" element={children} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    </ToastProvider>
  </I18nextProvider>
);

const WORKFLOWS_LIST_ENDPOINT_PREFIX = '/api/workflows?project_id=';
const toRequestUrl = (input: string | URL) =>
  typeof input === 'string' ? input : input.toString();

const createApiSuccess = <TData,>(data: TData) =>
  Promise.resolve({
    ok: true,
    json: async () => ({ success: true, data }),
  } satisfies Partial<Response>);

const createApiFailure = (
  status: number,
  statusText: string,
  message: string
) =>
  Promise.resolve({
    ok: false,
    status,
    statusText,
    json: async () => ({ success: false, message }),
  } satisfies Partial<Response>);

const rejectUnexpectedRequest = (url: string) =>
  Promise.reject(new Error(`Unexpected request: ${url}`));

// Mock workflow list data matching WorkflowListItemDto
const mockWorkflows: WorkflowListItemDto[] = [
  {
    id: 'workflow-1',
    projectId: 'proj-1',
    name: 'Test Workflow 1',
    description: 'Test description',
    status: 'draft',
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z',
    tasksCount: 3,
    terminalsCount: 6,
  },
  {
    id: 'workflow-2',
    projectId: 'proj-1',
    name: 'Test Workflow 2',
    description: 'Another description',
    status: 'running',
    createdAt: '2024-01-02T00:00:00Z',
    updatedAt: '2024-01-02T01:00:00Z',
    tasksCount: 2,
    terminalsCount: 4,
  },
  {
    id: 'workflow-3',
    projectId: 'proj-1',
    name: 'Completed Workflow',
    description: 'A completed workflow',
    status: 'completed',
    createdAt: '2024-01-03T00:00:00Z',
    updatedAt: '2024-01-03T02:00:00Z',
    tasksCount: 1,
    terminalsCount: 2,
  },
  {
    id: 'workflow-4',
    projectId: 'proj-1',
    name: 'Cancelled Workflow',
    description: 'A cancelled workflow',
    status: 'cancelled',
    createdAt: '2024-01-04T00:00:00Z',
    updatedAt: '2024-01-04T02:00:00Z',
    tasksCount: 2,
    terminalsCount: 3,
  },
];

type MockFetchRouteHandler = (
  init?: RequestInit
) => Promise<Partial<Response>>;

type WorkflowFetchMockOptions = {
  listData?: WorkflowListItemDto[];
  handlers?: Record<string, MockFetchRouteHandler>;
};

const createWorkflowFetchMock = ({
  listData = mockWorkflows,
  handlers = {},
}: WorkflowFetchMockOptions = {}) =>
  vi.fn((input: string | URL, init?: RequestInit) => {
    const url = toRequestUrl(input);

    if (url.startsWith(WORKFLOWS_LIST_ENDPOINT_PREFIX)) {
      return createApiSuccess(listData);
    }

    const handler = handlers[url];
    if (handler) {
      return handler(init);
    }

    return rejectUnexpectedRequest(url);
  });

const stubWorkflowsListFetch = (data: WorkflowListItemDto[] = mockWorkflows) => {
  vi.stubGlobal('fetch', vi.fn(() => createApiSuccess(data)));
};

async function renderAndOpenWorkflowDetail(workflowName: string) {
  render(<Workflows />, { wrapper });

  await waitFor(() => {
    expect(screen.getByText(workflowName)).toBeInTheDocument();
  });

  fireEvent.click(screen.getByText(workflowName).closest('.cursor-pointer'));

  await waitFor(() => {
    expect(
      screen.getByRole('button', { name: 'Merge Workflow' })
    ).toBeInTheDocument();
  });
}

const mockCompletedWorkflowDetail: WorkflowDetailDto = {
  id: 'workflow-3',
  projectId: 'proj-1',
  name: 'Completed Workflow',
  description: 'A completed workflow',
  status: 'completed',
  useSlashCommands: false,
  orchestratorEnabled: false,
  orchestratorApiType: null,
  orchestratorBaseUrl: null,
  orchestratorModel: null,
  errorTerminalEnabled: false,
  errorTerminalCliId: null,
  errorTerminalModelId: null,
  mergeTerminalCliId: 'test-cli',
  mergeTerminalModelId: 'test-model',
  targetBranch: 'main',
  readyAt: null,
  startedAt: null,
  completedAt: '2024-01-03T02:00:00Z',
  createdAt: '2024-01-03T00:00:00Z',
  updatedAt: '2024-01-03T02:00:00Z',
  tasks: [
    {
      id: 'task-1',
      workflowId: 'workflow-3',
      vkTaskId: null,
      name: 'Task 1',
      description: null,
      branch: 'workflow/task-1',
      status: 'completed',
      orderIndex: 0,
      startedAt: null,
      completedAt: '2024-01-03T02:00:00Z',
      createdAt: '2024-01-03T00:00:00Z',
      updatedAt: '2024-01-03T02:00:00Z',
      terminals: [
        {
          id: 'terminal-1',
          workflowTaskId: 'task-1',
          cliTypeId: 'test-cli',
          modelConfigId: 'test-model',
          customBaseUrl: null,
          role: 'developer',
          roleDescription: null,
          orderIndex: 0,
          status: 'completed',
          createdAt: '2024-01-03T00:00:00Z',
          updatedAt: '2024-01-03T02:00:00Z',
        },
      ],
    },
  ],
  commands: [],
};

const mockUnorderedWorkflowDetail: WorkflowDetailDto = {
  ...mockCompletedWorkflowDetail,
  id: 'workflow-unordered',
  projectId: 'proj-1',
  name: 'Unordered Workflow',
  tasks: [
    {
      ...mockCompletedWorkflowDetail.tasks[0],
      id: 'task-2',
      name: 'Task B',
      orderIndex: 1,
      terminals: [
        {
          ...mockCompletedWorkflowDetail.tasks[0].terminals[0],
          id: 'terminal-b2',
          orderIndex: 1,
          status: 'completed',
        },
        {
          ...mockCompletedWorkflowDetail.tasks[0].terminals[0],
          id: 'terminal-b1',
          orderIndex: 0,
          status: 'completed',
        },
      ],
    },
    {
      ...mockCompletedWorkflowDetail.tasks[0],
      id: 'task-1',
      name: 'Task A',
      orderIndex: 0,
      terminals: [
        {
          ...mockCompletedWorkflowDetail.tasks[0].terminals[0],
          id: 'terminal-a2',
          orderIndex: 1,
          status: 'completed',
        },
        {
          ...mockCompletedWorkflowDetail.tasks[0].terminals[0],
          id: 'terminal-a1',
          orderIndex: 0,
          status: 'completed',
        },
      ],
    },
  ],
};

const mockAgentPlannedWorkflowDetail: WorkflowDetailDto = {
  ...mockCompletedWorkflowDetail,
  id: 'workflow-agent',
  name: 'Agent Planned Workflow',
  status: 'running',
  executionMode: 'agent_planned',
  orchestratorEnabled: true,
  initialGoal: 'Coordinate multi-terminal implementation',
};

const basePromptDetectedPayload: TerminalPromptDetectedPayload = {
  workflowId: 'workflow-3',
  terminalId: 'terminal-1',
  taskId: 'task-1',
  sessionId: 'session-1',
  promptKind: 'yes_no',
  promptText: 'Continue? [y/n]',
  confidence: 0.95,
  hasDangerousKeywords: false,
  options: ['yes', 'no'],
  selectedIndex: 0,
};

function emitPromptDetected(
  payload: Partial<TerminalPromptDetectedPayload> = {}
) {
  const handler = wsStoreMock.handlers?.onTerminalPromptDetected;
  if (!handler) {
    throw new Error('onTerminalPromptDetected handler is not registered');
  }
  act(() => {
    handler({
      ...basePromptDetectedPayload,
      ...payload,
    });
  });
}

function emitPromptDecision(payload: TerminalPromptDecisionPayload) {
  const handler = wsStoreMock.handlers?.onTerminalPromptDecision;
  if (!handler) {
    throw new Error('onTerminalPromptDecision handler is not registered');
  }
  act(() => {
    handler(payload);
  });
}

// ============================================================================
// Tests
// ============================================================================

describe('Workflows Page', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    wsStoreMock.sendPromptResponse.mockReset();
    wsStoreMock.sendPromptResponse.mockReturnValue(true);
    wsStoreMock.workflowId = null;
    wsStoreMock.handlers = undefined;
    await setTestLanguage('en');
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('List Rendering', () => {
    it('should render workflow list from API', async () => {
      // Mock fetch for workflows list
      stubWorkflowsListFetch();

      render(<Workflows />, { wrapper });

      // Wait for workflows to load
      await waitFor(() => {
        expect(screen.getByText('Test Workflow 1')).toBeInTheDocument();
      });

      // Check all workflows are rendered
      expect(screen.getByText('Test Workflow 1')).toBeInTheDocument();
      expect(screen.getByText('Test Workflow 2')).toBeInTheDocument();
      expect(screen.getByText('Completed Workflow')).toBeInTheDocument();
    });

    it('should display workflow descriptions', async () => {
      stubWorkflowsListFetch();

      render(<Workflows />, { wrapper });

      await waitFor(() => {
        expect(screen.getByText('Test description')).toBeInTheDocument();
      });

      expect(screen.getByText('Another description')).toBeInTheDocument();
      expect(screen.getByText('A completed workflow')).toBeInTheDocument();
    });

    it('should display tasks and terminals count from DTO', async () => {
      stubWorkflowsListFetch();

      render(<Workflows />, { wrapper });

      await waitFor(() => {
        expect(screen.getByText('3 tasks')).toBeInTheDocument();
      });

      expect(screen.getByText('3 tasks')).toBeInTheDocument();
      expect(screen.getByText('6 terminals')).toBeInTheDocument();
      expect(screen.getAllByText('2 tasks').length).toBeGreaterThan(0);
      expect(screen.getByText('4 terminals')).toBeInTheDocument();
    });
  });

  describe('Status Badge', () => {
    it('should render status badges with correct styling', async () => {
      stubWorkflowsListFetch();

      render(<Workflows />, { wrapper });

      await waitFor(() => {
        expect(screen.getByText('draft')).toBeInTheDocument();
      });

      // Check status badges exist
      const draftBadge = screen.getByText('draft');
      const runningBadge = screen.getByText('Running');
      const completedBadge = screen.getByText('Completed');
      const cancelledBadge = screen.getByText('Cancelled');

      expect(draftBadge).toBeInTheDocument();
      expect(runningBadge).toBeInTheDocument();
      expect(completedBadge).toBeInTheDocument();
      expect(cancelledBadge).toBeInTheDocument();

      // cancelled should have neutral style instead of failed(red) style
      expect(cancelledBadge).toHaveClass('bg-zinc-100');
      expect(cancelledBadge).not.toHaveClass('bg-red-100');
    });
  });

  describe('Navigation', () => {
    it('should navigate to workflow detail when clicking a workflow card', async () => {
      stubWorkflowsListFetch();

      render(<Workflows />, { wrapper });

      await waitFor(() => {
        expect(screen.getByText('Test Workflow 1')).toBeInTheDocument();
      });

      // Click on first workflow card
      const workflowCard = screen
        .getByText('Test Workflow 1')
        .closest('.cursor-pointer');
      expect(workflowCard).toBeInTheDocument();

      // Note: Full navigation test would require more setup
      // This test verifies the card is clickable
      expect(workflowCard).toHaveClass('cursor-pointer');
    });

    it('should trigger merge API from workflow detail view', async () => {
      const fetchMock = createWorkflowFetchMock({
        handlers: {
          '/api/workflows/workflow-3': () =>
            createApiSuccess(mockCompletedWorkflowDetail),
          '/api/workflows/workflow-3/merge': () =>
            createApiSuccess({
              success: true,
              message: 'Merge completed successfully',
              workflowId: 'workflow-3',
              targetBranch: 'main',
              mergedTasks: [],
            }),
        },
      });

      vi.stubGlobal('fetch', fetchMock);

      render(<Workflows />, { wrapper });

      await waitFor(() => {
        expect(screen.getByText('Completed Workflow')).toBeInTheDocument();
      });

      const workflowCard = screen
        .getByText('Completed Workflow')
        .closest('.cursor-pointer');
      expect(workflowCard).toBeInTheDocument();
      fireEvent.click(workflowCard);

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Merge Workflow' })).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole('button', { name: 'Merge Workflow' }));

      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledWith(
          '/api/workflows/workflow-3/merge',
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify({ merge_strategy: 'squash' }),
          })
        );
      });

      await waitFor(() => {
        const detailCallCount = fetchMock.mock.calls.filter(
          ([url]) => String(url) === '/api/workflows/workflow-3'
        ).length;
        expect(detailCallCount).toBeGreaterThan(1);
      });
    });
  });

  describe('Empty State', () => {
    it('should show empty state when no workflows exist', async () => {
      stubWorkflowsListFetch([]);

      render(<Workflows />, { wrapper });

      await waitFor(() => {
        expect(screen.getByText('No workflows yet')).toBeInTheDocument();
      });

      expect(screen.getByText('No workflows yet')).toBeInTheDocument();
      expect(
        screen.getByText('Create Your First Workflow')
      ).toBeInTheDocument();
    });
  });

  describe('Loading State', () => {
    it('should show loading indicator while fetching', () => {
      vi.stubGlobal(
        'fetch',
        vi.fn(() => new Promise(() => {}))
      ); // Never resolves

      render(<Workflows />, { wrapper });

      expect(screen.getByText('Loading workflows...')).toBeInTheDocument();
    });
  });

  describe('Error State', () => {
    it('should show error message when fetch fails', async () => {
      vi.stubGlobal(
        'fetch',
        vi.fn(() =>
          createApiFailure(500, 'Internal Server Error', 'Failed to fetch')
        )
      );

      render(<Workflows />, { wrapper });

      await waitFor(() => {
        expect(
          screen.getByText(/Failed to load workflows/)
        ).toBeInTheDocument();
      });
    });
  });

  describe('Prompt Interaction', () => {
    it('registers realtime workflow status event handlers', async () => {
      stubWorkflowsListFetch();

      render(<Workflows />, { wrapper });

      await waitFor(() => {
        expect(screen.getByText('Test Workflow 1')).toBeInTheDocument();
      });

      expect(wsStoreMock.handlers?.onWorkflowStatusChanged).toEqual(
        expect.any(Function)
      );
      expect(wsStoreMock.handlers?.onTaskStatusChanged).toEqual(
        expect.any(Function)
      );
      expect(wsStoreMock.handlers?.onTerminalStatusChanged).toEqual(
        expect.any(Function)
      );
      expect(wsStoreMock.handlers?.onTerminalCompleted).toEqual(
        expect.any(Function)
      );
    });

    it('shows prompt dialog and submits yes/no response via API', async () => {
      const fetchMock = createWorkflowFetchMock({
        handlers: {
          '/api/workflows/workflow-3': () =>
            createApiSuccess(mockCompletedWorkflowDetail),
          '/api/workflows/workflow-3/prompts/respond': () =>
            createApiSuccess(null),
        },
      });

      vi.stubGlobal('fetch', fetchMock);
      await renderAndOpenWorkflowDetail('Completed Workflow');

      emitPromptDetected({
        workflowId: 'workflow-3',
        terminalId: 'terminal-1',
        promptKind: 'yes_no',
        promptText: 'Proceed with operation? [y/n]',
      });

      const dialog = await screen.findByTestId('workflow-prompt-dialog');
      expect(within(dialog).getByText('Proceed with operation? [y/n]')).toBeInTheDocument();

      fireEvent.click(within(dialog).getByRole('button', { name: 'Yes' }));

      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledWith(
          '/api/workflows/workflow-3/prompts/respond',
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify({ terminal_id: 'terminal-1', response: 'y' }),
          })
        );
      });

      await waitFor(() => {
        expect(screen.queryByTestId('workflow-prompt-dialog')).not.toBeInTheDocument();
      });
    });

    it('submits choice and input/password prompt responses', async () => {
      const fetchMock = createWorkflowFetchMock({
        handlers: {
          '/api/workflows/workflow-3': () =>
            createApiSuccess(mockCompletedWorkflowDetail),
          '/api/workflows/workflow-3/prompts/respond': () =>
            createApiSuccess(null),
        },
      });

      vi.stubGlobal('fetch', fetchMock);
      await renderAndOpenWorkflowDetail('Completed Workflow');

      emitPromptDetected({
        promptKind: 'choice',
        promptText: 'Select an option',
        options: ['Apple', 'Banana', 'Cherry'],
        selectedIndex: 0,
      });

      const choiceDialog = await screen.findByTestId('workflow-prompt-dialog');
      fireEvent.click(within(choiceDialog).getByTestId('workflow-prompt-option-1'));
      fireEvent.click(within(choiceDialog).getByTestId('workflow-prompt-submit-option'));

      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledWith(
          '/api/workflows/workflow-3/prompts/respond',
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify({
              terminal_id: 'terminal-1',
              response: 'Banana',
            }),
          })
        );
      });

      emitPromptDetected({
        promptKind: 'arrow_select',
        promptText: 'Select framework',
        options: ['React', 'Vue', 'Svelte'],
        selectedIndex: 0,
      });

      const arrowDialog = await screen.findByTestId('workflow-prompt-dialog');
      fireEvent.click(within(arrowDialog).getByTestId('workflow-prompt-option-2'));
      fireEvent.click(within(arrowDialog).getByTestId('workflow-prompt-submit-option'));

      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledWith(
          '/api/workflows/workflow-3/prompts/respond',
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify({
              terminal_id: 'terminal-1',
              response: '\u001b[B\u001b[B',
            }),
          })
        );
      });

      emitPromptDetected({
        promptKind: 'input',
        promptText: 'Enter username',
        options: [],
        selectedIndex: null,
      });

      const inputDialog = await screen.findByTestId('workflow-prompt-dialog');
      fireEvent.change(within(inputDialog).getByTestId('workflow-prompt-input'), {
        target: { value: 'alice' },
      });
      fireEvent.click(within(inputDialog).getByTestId('workflow-prompt-submit-input'));

      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledWith(
          '/api/workflows/workflow-3/prompts/respond',
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify({ terminal_id: 'terminal-1', response: 'alice' }),
          })
        );
      });

      emitPromptDetected({
        promptKind: 'password',
        promptText: 'Password:',
        options: [],
        selectedIndex: null,
      });

      const passwordDialog = await screen.findByTestId('workflow-prompt-dialog');
      const passwordInput = within(passwordDialog).getByTestId(
        'workflow-prompt-input'
      ) as HTMLInputElement;
      expect(passwordInput.type).toBe('password');

      fireEvent.change(passwordInput, {
        target: { value: 'secret-token' },
      });
      fireEvent.click(
        within(passwordDialog).getByTestId('workflow-prompt-submit-input')
      );

      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledWith(
          '/api/workflows/workflow-3/prompts/respond',
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify({
              terminal_id: 'terminal-1',
              response: 'secret-token',
            }),
          })
        );
      });
    });

    it('prevents duplicate prompt enqueue and closes by prompt decision', async () => {
      const fetchMock = createWorkflowFetchMock({
        handlers: {
          '/api/workflows/workflow-3': () =>
            createApiSuccess(mockCompletedWorkflowDetail),
        },
      });

      vi.stubGlobal('fetch', fetchMock);
      await renderAndOpenWorkflowDetail('Completed Workflow');

      emitPromptDetected({
        promptKind: 'yes_no',
        promptText: 'Duplicate guard?',
      });
      emitPromptDetected({
        promptKind: 'yes_no',
        promptText: 'Duplicate guard?',
      });

      await waitFor(() => {
        expect(screen.getByTestId('workflow-prompt-dialog')).toBeInTheDocument();
      });
      expect(screen.getAllByTestId('workflow-prompt-dialog')).toHaveLength(1);

      emitPromptDecision({
        workflowId: 'workflow-3',
        terminalId: 'terminal-1',
        taskId: 'task-1',
        sessionId: 'session-1',
        decision: 'llm_decision',
      });

      await waitFor(() => {
        expect(screen.queryByTestId('workflow-prompt-dialog')).not.toBeInTheDocument();
      });
    });

    it('falls back to workflow WS submission for enter_confirm when API rejects empty response', async () => {
      const fetchMock = createWorkflowFetchMock({
        handlers: {
          '/api/workflows/workflow-3': () =>
            createApiSuccess(mockCompletedWorkflowDetail),
          '/api/workflows/workflow-3/prompts/respond': () =>
            createApiFailure(400, 'Bad Request', 'response is required'),
        },
      });

      vi.stubGlobal('fetch', fetchMock);
      await renderAndOpenWorkflowDetail('Completed Workflow');

      emitPromptDetected({
        promptKind: 'enter_confirm',
        promptText: 'Press Enter to continue',
        options: [],
        selectedIndex: null,
      });

      const dialog = await screen.findByTestId('workflow-prompt-dialog');
      fireEvent.click(
        within(dialog).getByTestId('workflow-prompt-enter-confirm')
      );

      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledWith(
          '/api/workflows/workflow-3/prompts/respond',
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify({ terminal_id: 'terminal-1', response: '' }),
          })
        );
      });

      await waitFor(() => {
        expect(wsStoreMock.sendPromptResponse).toHaveBeenCalledWith({
          workflowId: 'workflow-3',
          terminalId: 'terminal-1',
          response: '',
        });
      });

      await waitFor(() => {
        expect(screen.queryByTestId('workflow-prompt-dialog')).not.toBeInTheDocument();
      });
    });

    it('keeps enter_confirm dialog open when workflow WS fallback send fails', async () => {
      wsStoreMock.sendPromptResponse.mockReturnValue(false);

      const fetchMock = createWorkflowFetchMock({
        handlers: {
          '/api/workflows/workflow-3': () =>
            createApiSuccess(mockCompletedWorkflowDetail),
          '/api/workflows/workflow-3/prompts/respond': () =>
            createApiFailure(400, 'Bad Request', 'response is required'),
        },
      });

      vi.stubGlobal('fetch', fetchMock);
      await renderAndOpenWorkflowDetail('Completed Workflow');

      emitPromptDetected({
        promptKind: 'enter_confirm',
        promptText: 'Press Enter to continue',
        options: [],
        selectedIndex: null,
      });

      const dialog = await screen.findByTestId('workflow-prompt-dialog');
      fireEvent.click(
        within(dialog).getByTestId('workflow-prompt-enter-confirm')
      );

      await waitFor(() => {
        expect(wsStoreMock.sendPromptResponse).toHaveBeenCalledWith({
          workflowId: 'workflow-3',
          terminalId: 'terminal-1',
          response: '',
        });
      });

      expect(screen.getByTestId('workflow-prompt-dialog')).toBeInTheDocument();
      expect(
        screen.getByText('Failed to submit prompt response over WebSocket')
      ).toBeInTheDocument();
    });
  });

  describe('Orchestrator Chat Panel', () => {
    it('shows primary-channel badge and system/summary messages for agent_planned workflow', async () => {
      const fetchMock = createWorkflowFetchMock({
        listData: [
          {
            ...mockWorkflows[0],
            id: 'workflow-agent',
            name: 'Agent Planned Workflow',
            status: 'running',
            executionMode: 'agent_planned',
          },
        ],
        handlers: {
          '/api/workflows/workflow-agent': () =>
            createApiSuccess(mockAgentPlannedWorkflowDetail),
          '/api/workflows/workflow-agent/orchestrator/messages?limit=80': () =>
            createApiSuccess([
              { role: 'user', content: 'Prioritize auth first' },
              { role: 'system', content: 'Command accepted' },
              { role: 'tool-summary', content: 'Execution summary: succeeded' },
              { role: 'assistant', content: 'Done. Auth is now prioritized.' },
            ]),
        },
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<Workflows />, { wrapper });
      await waitFor(() => {
        expect(screen.getByText('Agent Planned Workflow')).toBeInTheDocument();
      });

      fireEvent.click(
        screen.getByText('Agent Planned Workflow').closest('.cursor-pointer')
      );

      await waitFor(() => {
        expect(screen.getByText('Primary Channel')).toBeInTheDocument();
      });
      await waitFor(() => {
        expect(screen.getByText('Command accepted')).toBeInTheDocument();
        expect(screen.getByText('Execution summary: succeeded')).toBeInTheDocument();
      });
    });

    it('sends orchestrator message and shows forbidden error', async () => {
      const fetchMock = createWorkflowFetchMock({
        listData: [
          {
            ...mockWorkflows[0],
            id: 'workflow-agent',
            name: 'Agent Planned Workflow',
            status: 'running',
            executionMode: 'agent_planned',
          },
        ],
        handlers: {
          '/api/workflows/workflow-agent': () =>
            createApiSuccess(mockAgentPlannedWorkflowDetail),
          '/api/workflows/workflow-agent/orchestrator/messages?limit=80': () =>
            createApiSuccess([]),
          '/api/workflows/workflow-agent/orchestrator/chat': () =>
            createApiFailure(
              403,
              'Forbidden',
              "orchestrator role 'viewer' is not allowed to issue commands"
            ),
        },
      });
      vi.stubGlobal('fetch', fetchMock);

      render(<Workflows />, { wrapper });
      await waitFor(() => {
        expect(screen.getByText('Agent Planned Workflow')).toBeInTheDocument();
      });

      fireEvent.click(
        screen.getByText('Agent Planned Workflow').closest('.cursor-pointer')
      );

      const input = await screen.findByPlaceholderText(
        'For example: reprioritize tasks and complete the auth module first.'
      );
      fireEvent.change(input, { target: { value: 'Please prioritize auth.' } });

      fireEvent.click(screen.getByRole('button', { name: 'Send to Agent' }));

      await waitFor(() => {
        expect(
          screen.getByText("orchestrator role 'viewer' is not allowed to issue commands")
        ).toBeInTheDocument();
      });
    });
  });

  describe('Project and Pipeline Consistency', () => {
    it('renders tasks and terminals in orderIndex order', async () => {
      const fetchMock = createWorkflowFetchMock({
        listData: [
          {
            ...mockWorkflows[0],
            id: 'workflow-unordered',
            name: 'Unordered Workflow',
          },
        ],
        handlers: {
          '/api/workflows/workflow-unordered': () =>
            createApiSuccess(mockUnorderedWorkflowDetail),
        },
      });

      vi.stubGlobal('fetch', fetchMock);

      render(<Workflows />, { wrapper });

      await waitFor(() => {
        expect(screen.getByText('Unordered Workflow')).toBeInTheDocument();
      });

      fireEvent.click(screen.getByText('Unordered Workflow').closest('.cursor-pointer'));

      await waitFor(() => {
        expect(screen.getByText('Task A')).toBeInTheDocument();
        expect(screen.getByText('Task B')).toBeInTheDocument();
      });

      const taskTitles = screen
        .getAllByText(/Task [AB]/)
        .map((el) => el.textContent);
      expect(taskTitles[0]).toBe('Task A');
      expect(taskTitles[1]).toBe('Task B');

      const taskHeaderSpans = screen
        .getAllByText(/Task [12]/)
        .map((el) => el.textContent);
      expect(taskHeaderSpans.filter((text) => text === 'Task 1').length).toBeGreaterThan(0);
      expect(taskHeaderSpans.filter((text) => text === 'Task 2').length).toBeGreaterThan(0);
    });
  });

  describe('Workflow Creation', () => {
    it('falls back to selected project when resolve-by-path fails', async () => {
      const workflowNewDetail = {
        ...mockCompletedWorkflowDetail,
        id: 'workflow-new',
        projectId: 'proj-1',
        name: 'Wizard Created Workflow',
        status: 'draft' as const,
      };

      const fetchMock = createWorkflowFetchMock({
        listData: [],
        handlers: {
          '/api/projects/resolve-by-path': () =>
            createApiFailure(500, 'Internal Server Error', 'resolve failed'),
          '/api/workflows': (init) => {
            const body = init?.body ? JSON.parse(init.body as string) : null;
            expect(body?.projectId).toBe('proj-1');
            return createApiSuccess(workflowNewDetail);
          },
          '/api/workflows/workflow-new': () => createApiSuccess(workflowNewDetail),
        },
      });

      vi.stubGlobal('fetch', fetchMock);

      render(<Workflows />, { wrapper });

      const createButton = await screen.findByRole('button', {
        name: 'Create Workflow',
      });
      fireEvent.click(createButton);

      const submitButton = await screen.findByTestId(
        'mock-workflow-wizard-submit'
      );
      fireEvent.click(submitButton);

      // With the fix, when a project is already selected (proj-1 from URL),
      // resolve-by-path is NOT called — the selected project ID is used directly.
      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledWith(
          '/api/workflows',
          expect.objectContaining({ method: 'POST' })
        );
      });

      // Verify resolve-by-path was NOT called (project already selected)
      expect(fetchMock).not.toHaveBeenCalledWith(
        '/api/projects/resolve-by-path',
        expect.anything()
      );

      await waitFor(() => {
        expect(
          screen.queryByTestId('mock-workflow-wizard')
        ).not.toBeInTheDocument();
      });
    });
  });
});
