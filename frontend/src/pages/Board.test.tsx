import { beforeEach, describe, it, expect, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { Board } from './Board';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useWorkflowEvents } from '@/stores/wsStore';
import { MemoryRouter } from 'react-router-dom';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, opts?: { defaultValue?: string }) => opts?.defaultValue ?? '',
  }),
}));

vi.mock('@/stores/wsStore', () => ({
  useWorkflowEvents: vi.fn(),
}));

vi.mock('@/hooks/useProjects', () => ({
  useProjects: () => ({
    projects: [{ id: 'proj-1', name: 'Test Project' }],
    projectsById: { 'proj-1': { id: 'proj-1', name: 'Test Project' } },
    isLoading: false,
    isConnected: true,
    error: null,
  }),
}));

vi.mock('@/components/board/WorkflowSidebar', () => ({
  WorkflowSidebar: ({
    onSelectWorkflow,
  }: {
    onSelectWorkflow: (workflowId: string | null) => void;
  }) => (
    <aside data-testid="workflow-sidebar">
      <button type="button" onClick={() => onSelectWorkflow('wf-selected')}>
        select-workflow
      </button>
    </aside>
  ),
}));
vi.mock('@/components/board/WorkflowKanbanBoard', () => ({
  WorkflowKanbanBoard: () => <section data-testid="workflow-board" />,
}));
vi.mock('@/components/board/TerminalActivityPanel', () => ({
  TerminalActivityPanel: () => <div data-testid="terminal-activity" />,
}));
vi.mock('@/components/board/StatusBar', () => ({
  StatusBar: () => <footer data-testid="status-bar" />,
}));

describe('Board', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  const renderBoard = () => {
    const queryClient = new QueryClient();
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/board']}>
          <Board />
        </MemoryRouter>
      </QueryClientProvider>
    );
  };

  const expectWorkflowEventHandlers = (handlers: unknown) => {
    expect(handlers).toEqual(
      expect.objectContaining({
        onWorkflowStatusChanged: expect.any(Function),
        onTaskStatusChanged: expect.any(Function),
        onTerminalStatusChanged: expect.any(Function),
        onTerminalCompleted: expect.any(Function),
        onGitCommitDetected: expect.any(Function),
      })
    );
  };

  it('renders board layout sections', () => {
    renderBoard();
    expect(screen.getByTestId('workflow-sidebar')).toBeInTheDocument();
    expect(screen.getByTestId('workflow-board')).toBeInTheDocument();
    expect(screen.getByTestId('terminal-activity')).toBeInTheDocument();
    expect(screen.getByTestId('status-bar')).toBeInTheDocument();
  });

  it('updates useWorkflowEvents arguments in order when workflow is selected', async () => {
    const workflowEventsMock = vi.mocked(useWorkflowEvents);

    renderBoard();

    expect(workflowEventsMock).toHaveBeenCalled();
    expect(workflowEventsMock.mock.calls[0][0]).toBeNull();
    expectWorkflowEventHandlers(workflowEventsMock.mock.calls[0][1]);

    fireEvent.click(screen.getByRole('button', { name: 'select-workflow' }));

    await waitFor(() => {
      expect(
        workflowEventsMock.mock.calls.some(
          ([workflowId]) => workflowId === 'wf-selected'
        )
      ).toBe(true);
    });

    const firstSelectedCallIndex = workflowEventsMock.mock.calls.findIndex(
      ([workflowId]) => workflowId === 'wf-selected'
    );
    expect(firstSelectedCallIndex).toBeGreaterThan(0);

    for (let i = 0; i < firstSelectedCallIndex; i += 1) {
      expect(workflowEventsMock.mock.calls[i][0]).toBeNull();
    }

    const selectedCall = workflowEventsMock.mock.calls[firstSelectedCallIndex];
    expect(selectedCall[0]).toBe('wf-selected');
    expectWorkflowEventHandlers(selectedCall[1]);
  });
});
