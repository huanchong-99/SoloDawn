import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { Pipeline } from './Pipeline';

// Keep the real status→actions table so the resume wiring is exercised against
// the same transition rules the app ships with; only the data/mutation hooks
// are stubbed.
vi.mock('@/hooks/useWorkflows', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/hooks/useWorkflows')>();
  return {
    getWorkflowActions: actual.getWorkflowActions,
    useWorkflow: vi.fn(),
    useStartWorkflow: vi.fn(),
  };
});

vi.mock('@/hooks/useWorkflowInvalidation', () => ({
  useWorkflowInvalidation: vi.fn(),
}));

vi.mock('@/components/pipeline/OrchestratorHeader', () => ({
  OrchestratorHeader: ({
    canResume,
    isResuming,
    onResume,
  }: {
    canResume?: boolean;
    isResuming?: boolean;
    onResume?: () => void;
  }) => (
    <div data-testid="orchestrator-header">
      <span data-testid="can-resume">{String(canResume)}</span>
      <span data-testid="is-resuming">{String(isResuming)}</span>
      <button type="button" onClick={onResume}>
        resume
      </button>
    </div>
  ),
}));

vi.mock('@/components/pipeline/TaskPipeline', () => ({
  TaskPipeline: () => <div data-testid="task-pipeline" />,
}));

import { useStartWorkflow, useWorkflow } from '@/hooks/useWorkflows';

function renderPipeline() {
  return render(
    <MemoryRouter initialEntries={['/pipeline/wf-1']}>
      <Routes>
        <Route path="/pipeline/:workflowId" element={<Pipeline />} />
      </Routes>
    </MemoryRouter>
  );
}

function mockWorkflow(status: string) {
  vi.mocked(useWorkflow).mockReturnValue({
    data: { name: 'Workflow X', status, orchestratorModel: 'gpt-4o' },
    isLoading: false,
    error: null,
  } as never);
}

describe('Pipeline page', () => {
  const mutate = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useStartWorkflow).mockReturnValue({
      mutate,
      isPending: false,
    } as never);
  });

  it('renders header and pipeline when data exists', () => {
    mockWorkflow('running');
    renderPipeline();

    expect(screen.getByTestId('orchestrator-header')).toBeInTheDocument();
    expect(screen.getByTestId('task-pipeline')).toBeInTheDocument();
  });

  it('does not offer resume while the workflow is running', () => {
    mockWorkflow('running');
    renderPipeline();

    expect(screen.getByTestId('can-resume')).toHaveTextContent('false');
  });

  it('offers resume for a paused workflow', () => {
    mockWorkflow('paused');
    renderPipeline();

    expect(screen.getByTestId('can-resume')).toHaveTextContent('true');
  });

  it('resumes a paused workflow through the start mutation', async () => {
    mockWorkflow('paused');
    renderPipeline();

    await userEvent.click(screen.getByRole('button', { name: 'resume' }));

    expect(mutate).toHaveBeenCalledWith({ workflow_id: 'wf-1' });
  });

  it('reports in-flight resume state to the header', () => {
    vi.mocked(useStartWorkflow).mockReturnValue({
      mutate,
      isPending: true,
    } as never);
    mockWorkflow('paused');
    renderPipeline();

    expect(screen.getByTestId('is-resuming')).toHaveTextContent('true');
  });
});
