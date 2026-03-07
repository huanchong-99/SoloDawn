import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { WorkflowSidebar } from './WorkflowSidebar';

vi.mock('@/hooks/useWorkflows', () => ({
  useWorkflows: vi.fn(),
}));

import { useWorkflows } from '@/hooks/useWorkflows';

const renderWithRouter = (ui: React.ReactElement) => {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
};

const defaultProjects = [{ id: 'proj-1', name: 'Project One' }] as any[];

describe('WorkflowSidebar', () => {
  it('renders workflows list', () => {
    vi.mocked(useWorkflows).mockReturnValue({
      data: [
        {
          id: 'wf-1',
          projectId: 'proj-1',
          name: 'Workflow A',
          description: null,
          status: 'created',
          createdAt: '',
          updatedAt: '',
          tasksCount: 0,
          terminalsCount: 0,
        },
      ],
      isLoading: false,
      error: null,
    } as any);

    renderWithRouter(
      <WorkflowSidebar
        projects={defaultProjects}
        activeProjectId="proj-1"
        onProjectChange={() => {}}
        selectedWorkflowId={null}
        onSelectWorkflow={() => {}}
      />
    );

    expect(screen.getByText('Workflow A')).toBeInTheDocument();
  });

  it('shows loading state', () => {
    vi.mocked(useWorkflows).mockReturnValue({
      data: [],
      isLoading: true,
      error: null,
    } as any);

    renderWithRouter(
      <WorkflowSidebar
        projects={defaultProjects}
        activeProjectId="proj-1"
        onProjectChange={() => {}}
        selectedWorkflowId={null}
        onSelectWorkflow={() => {}}
      />
    );

    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it('shows project selector when multiple projects exist', () => {
    const multipleProjects = [
      { id: 'proj-1', name: 'Project One' },
      { id: 'proj-2', name: 'Project Two' },
    ] as any[];

    vi.mocked(useWorkflows).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    } as any);

    renderWithRouter(
      <WorkflowSidebar
        projects={multipleProjects}
        activeProjectId="proj-1"
        onProjectChange={() => {}}
        selectedWorkflowId={null}
        onSelectWorkflow={() => {}}
      />
    );

    expect(screen.getByText('Project One')).toBeInTheDocument();
  });
});
