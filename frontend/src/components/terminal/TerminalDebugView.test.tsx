import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen, fireEvent, waitFor, act } from '@testing-library/react';
import { forwardRef } from 'react';
import { TerminalDebugView } from './TerminalDebugView';
import type { Terminal } from '@/components/workflow/TerminalCard';
import type { WorkflowTask } from '@/components/workflow/PipelineView';
import { renderWithI18n, setTestLanguage, i18n } from '@/test/renderWithI18n';

const terminalEmulatorPropsSpy = vi.fn();
const terminalEmulatorWsCreateSpy = vi.fn();
const fetchMock = vi.fn();

const createFetchOkResponse = () =>
  ({
    ok: true,
    status: 200,
    json: async () => ({}),
  }) as Response;

vi.mock('./TerminalEmulator', () => ({
  TerminalEmulator: forwardRef((props: { terminalId: string; wsUrl?: string }, _ref) => {
    terminalEmulatorPropsSpy(props);
    if (props.wsUrl) {
      terminalEmulatorWsCreateSpy(props.terminalId);
    }
    return <div data-testid="terminal-emulator" data-terminal-id={props.terminalId} />;
  }),
}));

vi.mock('@xterm/xterm', () => {
  class MockTerminal {
    onData = vi.fn<(handler: (data: string) => void) => void>();
    open = vi.fn<(container: HTMLElement) => void>();
    write = vi.fn<(data: string) => void>();
    clear = vi.fn<() => void>();
    dispose = vi.fn<() => void>();
    loadAddon = vi.fn<(addon: unknown) => void>();
    cols = 80;
    rows = 24;
  }
  return { Terminal: MockTerminal };
});

vi.mock('@xterm/addon-fit', () => {
  class MockFitAddon {
    fit = vi.fn<() => void>();
  }
  return { FitAddon: MockFitAddon };
});

vi.mock('@/hooks/useQualityGate', () => ({
  useTerminalLatestQuality: () => ({ data: null, isLoading: false }),
  useQualityIssues: () => ({ data: [], isLoading: false }),
  qualityKeys: {
    all: ['quality'],
    runsForWorkflow: (id: string) => ['quality', 'runs', 'workflow', id],
    runDetail: (id: string) => ['quality', 'run', id],
    issuesForRun: (id: string) => ['quality', 'issues', id],
    latestForTerminal: (id: string) => ['quality', 'latest', 'terminal', id],
  },
}));

class MockWebSocket {
  url = '';
  readyState = 0;
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((error: Event) => void) | null = null;
  onclose: (() => void) | null = null;

  constructor(url: string) {
    this.url = url;
    setTimeout(() => {
      this.readyState = 1;
      this.onopen?.();
    }, 0);
  }

  send() {
    // Mock send
  }

  close() {
    this.readyState = 3;
    this.onclose?.();
  }

  addEventListener(event: string, handler: () => void) {
    if (event === 'open') this.onopen = handler;
  }
}

globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket;

const mockTerminals: Terminal[] = [
  {
    id: 'term-1',
    workflowTaskId: 'task-1',
    cliTypeId: 'claude-code',
    modelConfigId: 'model-1',
    role: 'Developer',
    orderIndex: 0,
    status: 'working',
    processId: null,
    ptySessionId: null,
  },
  {
    id: 'term-2',
    workflowTaskId: 'task-1',
    cliTypeId: 'cursor',
    modelConfigId: 'model-2',
    role: 'Reviewer',
    orderIndex: 1,
    status: 'not_started',
    processId: null,
    ptySessionId: null,
  },
];

const mockTasks: (WorkflowTask & { terminals: Terminal[] })[] = [
  {
    id: 'task-1',
    name: 'Implementation Task',
    branch: 'feature/implementation',
    terminals: mockTerminals,
  },
];

const getDeveloperButton = () => screen.getByRole('button', { name: /^Developer -/ });
const getReviewerButton = () => screen.getByRole('button', { name: /^Reviewer -/ });

describe('TerminalDebugView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    terminalEmulatorPropsSpy.mockClear();
    terminalEmulatorWsCreateSpy.mockClear();
    fetchMock.mockReset();
    fetchMock.mockResolvedValue(createFetchOkResponse());
    vi.stubGlobal('fetch', fetchMock);
    void setTestLanguage();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe('Rendering', () => {
    it('should render terminal list sidebar', () => {
      renderWithI18n(<TerminalDebugView tasks={mockTasks} wsUrl="ws://localhost:8080" />);
      expect(screen.getByText(i18n.t('workflow:terminalDebug.listTitle'))).toBeInTheDocument();
    });

    it('should render all terminals in the list', () => {
      renderWithI18n(<TerminalDebugView tasks={mockTasks} wsUrl="ws://localhost:8080" />);
      expect(getDeveloperButton()).toBeInTheDocument();
      expect(getReviewerButton()).toBeInTheDocument();
    });

    it('should auto-select the first available terminal', async () => {
      renderWithI18n(<TerminalDebugView tasks={mockTasks} wsUrl="ws://localhost:8080" />);

      await waitFor(() => {
        expect(screen.getByText('claude-code - model-1')).toBeInTheDocument();
        expect(screen.getByTestId('terminal-emulator')).toHaveAttribute('data-terminal-id', 'term-1');
      });
    });

    it('should show an empty state until terminals arrive, then auto-select the first one', async () => {
      const { rerender } = renderWithI18n(
        <TerminalDebugView tasks={[]} wsUrl="ws://localhost:8080" />
      );

      expect(screen.getByText(i18n.t('workflow:terminalDebug.emptyTitle'))).toBeInTheDocument();
      expect(
        screen.getByText(i18n.t('workflow:terminalDebug.emptyDescription'))
      ).toBeInTheDocument();

      rerender(<TerminalDebugView tasks={mockTasks} wsUrl="ws://localhost:8080" />);

      await waitFor(() => {
        expect(screen.getByText('claude-code - model-1')).toBeInTheDocument();
        expect(screen.getByTestId('terminal-emulator')).toHaveAttribute('data-terminal-id', 'term-1');
      });
    });
  });

  describe('Terminal Selection', () => {
    it('should select terminal when clicked', async () => {
      renderWithI18n(<TerminalDebugView tasks={mockTasks} wsUrl="ws://localhost:8080" />);

      const devButton = getDeveloperButton();
      fireEvent.click(devButton);

      await waitFor(() => {
        expect(screen.getByText('claude-code - model-1')).toBeInTheDocument();
      });
    });

    it('should highlight selected terminal', async () => {
      renderWithI18n(<TerminalDebugView tasks={mockTasks} wsUrl="ws://localhost:8080" />);

      const devButton = getDeveloperButton();
      fireEvent.click(devButton);

      await waitFor(() => {
        expect(devButton).toHaveClass('bg-primary');
      });
    });
  });

  describe('Terminal Status Display', () => {
    it('should display status dot with correct label', () => {
      renderWithI18n(<TerminalDebugView tasks={mockTasks} wsUrl="ws://localhost:8080" />);

      const workingLabel = i18n.t('workflow:terminalDebug.status.working');
      expect(screen.getByText(workingLabel)).toBeInTheDocument();
    });

    it('should show task name for each terminal', () => {
      renderWithI18n(<TerminalDebugView tasks={mockTasks} wsUrl="ws://localhost:8080" />);
      expect(screen.getAllByText('Implementation Task')).toHaveLength(2);
    });
  });

  describe('Terminal View Panel', () => {
    it('should render terminal info when selected', async () => {
      renderWithI18n(<TerminalDebugView tasks={mockTasks} wsUrl="ws://localhost:8080" />);

      const devButton = getDeveloperButton();
      fireEvent.click(devButton);

      await waitFor(() => {
        expect(screen.getAllByText('Developer')).toHaveLength(2);
        expect(screen.getByText(/claude-code/)).toBeInTheDocument();
      });
    });

    it('should render TerminalEmulator when terminal selected', async () => {
      renderWithI18n(<TerminalDebugView tasks={mockTasks} wsUrl="ws://localhost:8080" />);

      const devButton = getDeveloperButton();
      fireEvent.click(devButton);

      await waitFor(() => {
        expect(screen.getByTestId('terminal-emulator')).toBeInTheDocument();
      });
    });

    it('should render TerminalEmulator for waiting terminal without auto-start request', async () => {
      const waitingTasks: (WorkflowTask & { terminals: Terminal[] })[] = [
        {
          ...mockTasks[0],
          terminals: mockTasks[0].terminals.map((terminal) =>
            terminal.id === 'term-2' ? { ...terminal, status: 'waiting' } : terminal
          ),
        },
      ];

      renderWithI18n(<TerminalDebugView tasks={waitingTasks} wsUrl="ws://localhost:8080" />);

      const reviewerButton = getReviewerButton();
      fireEvent.click(reviewerButton);

      await waitFor(() => {
        expect(screen.getByTestId('terminal-emulator')).toHaveAttribute('data-terminal-id', 'term-2');
      });

      expect(fetchMock).not.toHaveBeenCalledWith('/api/terminals/term-2/start', { method: 'POST' });
    });

    it('should rebuild terminal emulator when switching terminals', async () => {
      const switchableTasks: (WorkflowTask & { terminals: Terminal[] })[] = [
        {
          ...mockTasks[0],
          terminals: mockTasks[0].terminals.map((terminal) =>
            terminal.id === 'term-2' ? { ...terminal, status: 'working' } : terminal
          ),
        },
      ];

      renderWithI18n(<TerminalDebugView tasks={switchableTasks} wsUrl="ws://localhost:8080" />);

      const devButton = getDeveloperButton();
      const reviewerButton = getReviewerButton();

      fireEvent.click(devButton);

      await waitFor(() => {
        expect(screen.getByTestId('terminal-emulator')).toHaveAttribute('data-terminal-id', 'term-1');
      });

      const terminalIdsAfterFirstSelection = terminalEmulatorPropsSpy.mock.calls.map(
        (args) => args[0]?.terminalId as string
      );
      expect(terminalIdsAfterFirstSelection).toContain('term-1');
      expect(terminalIdsAfterFirstSelection).not.toContain('term-2');

      fireEvent.click(reviewerButton);

      await waitFor(() => {
        expect(screen.getByTestId('terminal-emulator')).toHaveAttribute('data-terminal-id', 'term-2');
      });

      const terminalIds = terminalEmulatorPropsSpy.mock.calls.map(
        (args) => args[0]?.terminalId as string
      );

      expect(terminalIds).toContain('term-1');
      expect(terminalIds).toContain('term-2');
      expect(fetchMock).not.toHaveBeenCalled();
    });

    it('should show starting placeholder when switching from working to not_started terminal', async () => {
      let resolveStartRequest: ((value: Response) => void) | null = null;
      fetchMock.mockImplementationOnce(
        () =>
          new Promise<Response>((resolve) => {
            resolveStartRequest = resolve;
          })
      );

      const { rerender } = renderWithI18n(
        <TerminalDebugView tasks={mockTasks} wsUrl="ws://localhost:8080" />
      );

      const devButton = getDeveloperButton();
      const reviewerButton = getReviewerButton();

      fireEvent.click(devButton);

      await waitFor(() => {
        expect(screen.getByTestId('terminal-emulator')).toHaveAttribute('data-terminal-id', 'term-1');
        expect(screen.getByText('claude-code - model-1')).toBeInTheDocument();
      });

      fireEvent.click(reviewerButton);

      await waitFor(() => {
        expect(screen.getByText('cursor - model-2')).toBeInTheDocument();
        expect(screen.queryByText('claude-code - model-1')).not.toBeInTheDocument();
        expect(screen.getByText(i18n.t('workflow:terminalDebug.starting'))).toBeInTheDocument();
        expect(screen.queryByTestId('terminal-emulator')).not.toBeInTheDocument();
      });

      expect(fetchMock).toHaveBeenCalledTimes(1);
      expect(fetchMock).toHaveBeenCalledWith('/api/terminals/term-2/start', { method: 'POST' });

      if (!resolveStartRequest) {
        throw new Error('Expected terminal start request to be pending.');
      }

      await act(async () => {
        resolveStartRequest?.(createFetchOkResponse());
      });

      rerender(
        <TerminalDebugView
          tasks={[
            {
              ...mockTasks[0],
              terminals: mockTasks[0].terminals.map((terminal) =>
                terminal.id === 'term-2' ? { ...terminal, status: 'waiting' } : terminal
              ),
            },
          ]}
          wsUrl="ws://localhost:8080"
        />
      );

      fireEvent.click(getReviewerButton());

      await waitFor(() => {
        expect(screen.getByTestId('terminal-emulator')).toHaveAttribute('data-terminal-id', 'term-2');
        expect(screen.queryByText(i18n.t('workflow:terminalDebug.starting'))).not.toBeInTheDocument();
      });
    });

    it('should not create TerminalEmulator or websocket before ready, then create both after ready', async () => {
      let resolveStartRequest: ((value: Response) => void) | null = null;
      fetchMock.mockImplementationOnce(
        () =>
          new Promise<Response>((resolve) => {
            resolveStartRequest = resolve;
          })
      );

      const pendingTasks: (WorkflowTask & { terminals: Terminal[] })[] = [
        {
          ...mockTasks[0],
          terminals: [
            {
              ...mockTasks[0].terminals[0],
              status: 'not_started',
            },
          ],
        },
      ];

      const { rerender } = renderWithI18n(
        <TerminalDebugView tasks={pendingTasks} wsUrl="ws://localhost:8080" />
      );

      const devButton = getDeveloperButton();

      fireEvent.click(devButton);

      await waitFor(() => {
        expect(screen.getByText(i18n.t('workflow:terminalDebug.starting'))).toBeInTheDocument();
        expect(screen.queryByTestId('terminal-emulator')).not.toBeInTheDocument();
      });

      expect(fetchMock).toHaveBeenCalledTimes(1);
      expect(fetchMock).toHaveBeenCalledWith('/api/terminals/term-1/start', { method: 'POST' });
      expect(terminalEmulatorPropsSpy).not.toHaveBeenCalled();
      expect(terminalEmulatorWsCreateSpy).not.toHaveBeenCalled();

      if (!resolveStartRequest) {
        throw new Error('Expected terminal start request to be pending.');
      }

      await act(async () => {
        resolveStartRequest?.(createFetchOkResponse());
      });

      rerender(
        <TerminalDebugView
          tasks={[
            {
              ...pendingTasks[0],
              terminals: pendingTasks[0].terminals.map((terminal) => ({
                ...terminal,
                status: 'waiting',
              })),
            },
          ]}
          wsUrl="ws://localhost:8080"
        />
      );

      await waitFor(() => {
        expect(screen.getByTestId('terminal-emulator')).toHaveAttribute('data-terminal-id', 'term-1');
      });

      expect(terminalEmulatorPropsSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          terminalId: 'term-1',
          wsUrl: 'ws://localhost:8080',
        })
      );
      expect(terminalEmulatorWsCreateSpy).toHaveBeenCalledWith('term-1');
    });

    it('should not auto-restart completed terminal on process-not-running error', async () => {
      const completedTasks: (WorkflowTask & { terminals: Terminal[] })[] = [
        {
          ...mockTasks[0],
          terminals: mockTasks[0].terminals.map((terminal) =>
            terminal.id === 'term-1' ? { ...terminal, status: 'completed' } : terminal
          ),
        },
      ];

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ success: true, data: [] }),
      } as Response);

      renderWithI18n(<TerminalDebugView tasks={completedTasks} wsUrl="ws://localhost:8080" />);

      const devButton = getDeveloperButton();
      fireEvent.click(devButton);

      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledWith('/api/terminals/term-1/logs?limit=1000');
        expect(screen.queryByTestId('terminal-emulator')).not.toBeInTheDocument();
      });

      expect(fetchMock).not.toHaveBeenCalledWith('/api/terminals/term-1/start', { method: 'POST' });
    });

    it('should auto-restart working terminal on process-not-running error', async () => {
      renderWithI18n(<TerminalDebugView tasks={mockTasks} wsUrl="ws://localhost:8080" />);

      const devButton = getDeveloperButton();
      fireEvent.click(devButton);

      await waitFor(() => {
        expect(screen.getByTestId('terminal-emulator')).toHaveAttribute('data-terminal-id', 'term-1');
      });

      const latestCall = terminalEmulatorPropsSpy.mock.calls[
        terminalEmulatorPropsSpy.mock.calls.length - 1
      ];
      const latestProps = latestCall?.[0] as
        | { onError?: (error: Error) => void }
        | undefined;

      await act(async () => {
        latestProps?.onError?.(
          new Error('Terminal process not running. Please start the terminal first.')
        );
      });

      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledWith('/api/terminals/term-1/start', { method: 'POST' });
      });
    });

    it('should render history view instead of websocket for completed terminal', async () => {
      const completedTasks: (WorkflowTask & { terminals: Terminal[] })[] = [
        {
          ...mockTasks[0],
          terminals: mockTasks[0].terminals.map((terminal) =>
            terminal.id === 'term-1' ? { ...terminal, status: 'completed' } : terminal
          ),
        },
      ];

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          success: true,
          data: [
            { id: 'l1', content: 'line 1\n' },
            { id: 'l2', content: 'line 2\n' },
          ],
        }),
      } as Response);

      renderWithI18n(<TerminalDebugView tasks={completedTasks} wsUrl="ws://localhost:8080" />);

      const devButton = getDeveloperButton();

      fireEvent.click(devButton);

      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledWith('/api/terminals/term-1/logs?limit=1000');
      });

      await waitFor(() => {
        expect(screen.queryByTestId('terminal-emulator')).not.toBeInTheDocument();
        const historyPanel = screen.getByText('Terminal history').parentElement;
        expect(historyPanel).toHaveTextContent('line 1');
        expect(historyPanel).toHaveTextContent('line 2');
      });
    });

    it('should sanitize ansi and control sequences in completed terminal history', async () => {
      const completedTasks: (WorkflowTask & { terminals: Terminal[] })[] = [
        {
          ...mockTasks[0],
          terminals: mockTasks[0].terminals.map((terminal) =>
            terminal.id === 'term-1' ? { ...terminal, status: 'completed' } : terminal
          ),
        },
      ];

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          success: true,
          data: [
            { id: 'l1', content: '\u001b[?2026h\u001b[38;2;215;119;87mMoseying\u001b[0m\n' },
            { id: 'l2', content: '\u0007Running required command\r\n' },
          ],
        }),
      } as Response);

      renderWithI18n(<TerminalDebugView tasks={completedTasks} wsUrl="ws://localhost:8080" />);

      const devButton = getDeveloperButton();

      fireEvent.click(devButton);

      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledWith('/api/terminals/term-1/logs?limit=1000');
      });

      await waitFor(() => {
        const historyPanel = screen.getByText('Terminal history').parentElement;
        expect(historyPanel).toHaveTextContent('Moseying');
        expect(historyPanel).toHaveTextContent('Running required command');

        const content = historyPanel?.textContent ?? '';
        expect(content).not.toContain('\u001b');
        expect(content).not.toContain('[?2026h');
        expect(content).not.toContain('\u0007');
      });
    });
  });

  describe('Control Buttons', () => {
    it('should render control buttons when terminal selected', async () => {
      renderWithI18n(<TerminalDebugView tasks={mockTasks} wsUrl="ws://localhost:8080" />);

      const devButton = getDeveloperButton();
      fireEvent.click(devButton);

      await waitFor(() => {
        expect(screen.getByText(i18n.t('workflow:terminalDebug.clear'))).toBeInTheDocument();
        expect(screen.getByText(i18n.t('workflow:terminalDebug.restart'))).toBeInTheDocument();
      });
    });
  });
});
