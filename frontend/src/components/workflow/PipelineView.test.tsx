import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen } from '@testing-library/react';
import { PipelineView } from './PipelineView';
import type { Terminal } from './TerminalCard';
import type { WorkflowTaskDto, TerminalDto } from '@/shared/types';
import { renderWithI18n, setTestLanguage, i18n } from '@/test/renderWithI18n';

vi.mock('@/hooks/useQualityGate', () => ({
  useTerminalLatestQuality: () => ({ data: null, isLoading: false }),
  qualityKeys: {
    all: ['quality'],
    runsForWorkflow: (id: string) => ['quality', 'runs', 'workflow', id],
    runDetail: (id: string) => ['quality', 'run', id],
    issuesForRun: (id: string) => ['quality', 'issues', id],
    latestForTerminal: (id: string) => ['quality', 'latest', 'terminal', id],
  },
}));

describe('PipelineView', () => {
  // Create realistic DTO-based mock data
  const mockTerminalDto1: TerminalDto = {
    id: 'terminal-1',
    workflowTaskId: 'task-1',
    cliTypeId: 'claude-code',
    modelConfigId: 'model-1',
    customBaseUrl: null,
    customApiKey: null,
    role: 'Developer',
    roleDescription: null,
    orderIndex: 0,
    status: 'completed',
    createdAt: '2025-01-25T10:00:00Z',
    updatedAt: '2025-01-25T10:05:00Z',
  };

  const mockTerminalDto2: TerminalDto = {
    id: 'terminal-2',
    workflowTaskId: 'task-1',
    cliTypeId: 'qwen-code',
    modelConfigId: 'model-2',
    customBaseUrl: null,
    customApiKey: null,
    role: 'Reviewer',
    roleDescription: null,
    orderIndex: 1,
    status: 'working',
    createdAt: '2025-01-25T10:05:00Z',
    updatedAt: '2025-01-25T10:10:00Z',
  };

  const mockTerminalDto3: TerminalDto = {
    id: 'terminal-3',
    workflowTaskId: 'task-2',
    cliTypeId: 'codex',
    modelConfigId: 'model-3',
    customBaseUrl: null,
    customApiKey: null,
    role: null,
    roleDescription: null,
    orderIndex: 0,
    status: 'not_started',
    createdAt: '2025-01-25T10:10:00Z',
    updatedAt: '2025-01-25T10:10:00Z',
  };

  const mockWorkflowTaskDto1: WorkflowTaskDto = {
    id: 'task-1',
    workflowId: 'workflow-1',
    vkTaskId: null,
    name: 'Implement Authentication',
    description: 'Add login and signup functionality',
    branch: 'feat/auth',
    status: 'in_progress',
    orderIndex: 0,
    startedAt: '2025-01-25T10:00:00Z',
    completedAt: null,
    createdAt: '2025-01-25T09:00:00Z',
    updatedAt: '2025-01-25T10:05:00Z',
    terminals: [mockTerminalDto1, mockTerminalDto2],
  };

  const mockWorkflowTaskDto2: WorkflowTaskDto = {
    id: 'task-2',
    workflowId: 'workflow-1',
    vkTaskId: null,
    name: 'Add Database Integration',
    description: 'Set up PostgreSQL database',
    branch: 'feat/database',
    status: 'pending',
    orderIndex: 1,
    startedAt: null,
    completedAt: null,
    createdAt: '2025-01-25T09:00:00Z',
    updatedAt: '2025-01-25T09:00:00Z',
    terminals: [mockTerminalDto3],
  };

  // Convert DTOs to component props format
  const mockTasks = [
    {
      id: mockWorkflowTaskDto1.id,
      name: mockWorkflowTaskDto1.name,
      branch: mockWorkflowTaskDto1.branch,
      terminals: mockWorkflowTaskDto1.terminals.map((dto) => ({
        id: dto.id,
        orderIndex: dto.orderIndex,
        cliTypeId: dto.cliTypeId,
        role: dto.role ?? undefined,
        status: dto.status as Terminal['status'],
      })),
    },
    {
      id: mockWorkflowTaskDto2.id,
      name: mockWorkflowTaskDto2.name,
      branch: mockWorkflowTaskDto2.branch,
      terminals: mockWorkflowTaskDto2.terminals.map((dto) => ({
        id: dto.id,
        orderIndex: dto.orderIndex,
        cliTypeId: dto.cliTypeId,
        role: dto.role ?? undefined,
        status: dto.status as Terminal['status'],
      })),
    },
  ];

  const mockMergeTerminal = {
    cliTypeId: 'claude-code',
    modelConfigId: 'model-1',
    status: 'not_started' as Terminal['status'],
  };

  beforeEach(() => {
    void setTestLanguage();
  });

  it('should render workflow name and status badge', () => {
    renderWithI18n(
      <PipelineView
        name="Test Workflow"
        status="idle"
        tasks={mockTasks}
        mergeTerminal={mockMergeTerminal}
      />
    );

    expect(screen.getByText('Test Workflow')).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:pipeline.status.idle'))).toBeInTheDocument();
  });

  it('should render all tasks with their numbers and names', () => {
    renderWithI18n(
      <PipelineView
        name="Test Workflow"
        status="running"
        tasks={mockTasks}
        mergeTerminal={mockMergeTerminal}
      />
    );

    expect(screen.getByText(i18n.t('workflow:pipeline.taskLabel', { index: 1 }))).toBeInTheDocument();
    expect(screen.getByText('Implement Authentication')).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:pipeline.taskLabel', { index: 2 }))).toBeInTheDocument();
    expect(screen.getByText('Add Database Integration')).toBeInTheDocument();
  });

  it('should render git branch badges for each task', () => {
    renderWithI18n(
      <PipelineView
        name="Test Workflow"
        status="running"
        tasks={mockTasks}
        mergeTerminal={mockMergeTerminal}
      />
    );

    expect(screen.getByText('feat/auth')).toBeInTheDocument();
    expect(screen.getByText('feat/database')).toBeInTheDocument();
  });

  it('should display terminals horizontally for each task', () => {
    const { container } = renderWithI18n(
      <PipelineView
        name="Test Workflow"
        status="running"
        tasks={mockTasks}
        mergeTerminal={mockMergeTerminal}
      />
    );

    const terminalCards = container.querySelectorAll('[class*="w-32"]');
    expect(terminalCards.length).toBeGreaterThan(0);
  });

  it('should render connector lines between terminals', () => {
    const { container } = renderWithI18n(
      <PipelineView
        name="Test Workflow"
        status="running"
        tasks={mockTasks}
        mergeTerminal={mockMergeTerminal}
      />
    );

    const connectors = container.querySelectorAll(String.raw`.w-8.h-0\.5.bg-muted\/30`);
    expect(connectors.length).toBe(1);
  });

  it('should render merge terminal with dashed border', () => {
    const { container } = renderWithI18n(
      <PipelineView
        name="Test Workflow"
        status="running"
        tasks={mockTasks}
        mergeTerminal={mockMergeTerminal}
      />
    );

    const dashedBorder = container.querySelector('.border-dashed');
    expect(dashedBorder).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:pipeline.mergeTerminalRole'))).toBeInTheDocument();
  });

  it('should display correct status labels', () => {
    const statuses: PipelineViewProps['status'][] = [
      'idle',
      'running',
      'paused',
      'completed',
      'failed',
    ];

    const expectedLabels = {
      idle: i18n.t('workflow:pipeline.status.idle'),
      running: i18n.t('workflow:pipeline.status.running'),
      paused: i18n.t('workflow:pipeline.status.paused'),
      completed: i18n.t('workflow:pipeline.status.completed'),
      failed: i18n.t('workflow:pipeline.status.failed'),
    };

    statuses.forEach((status) => {
      const { unmount } = renderWithI18n(
        <PipelineView
          name="Test Workflow"
          status={status}
          tasks={mockTasks}
          mergeTerminal={mockMergeTerminal}
        />
      );

      expect(screen.getByText(expectedLabels[status])).toBeInTheDocument();
      unmount();
    });
  });

  it('should call onTerminalClick when terminal is clicked', () => {
    const onTerminalClick = vi.fn();

    const { container } = renderWithI18n(
      <PipelineView
        name="Test Workflow"
        status="running"
        tasks={mockTasks}
        mergeTerminal={mockMergeTerminal}
        onTerminalClick={onTerminalClick}
      />
    );

    const terminalCards = container.querySelectorAll<HTMLElement>('[class*="w-32"]');
    const terminalCard = terminalCards.item(0);
    terminalCard.click();
    expect(onTerminalClick).toHaveBeenCalledWith('task-1', 'terminal-1');
  });

  it('should call onMergeTerminalClick when merge terminal is clicked', () => {
    const onMergeTerminalClick = vi.fn();

    const { container } = renderWithI18n(
      <PipelineView
        name="Test Workflow"
        status="running"
        tasks={mockTasks}
        mergeTerminal={mockMergeTerminal}
        onMergeTerminalClick={onMergeTerminalClick}
      />
    );

    const dashedBorder = container.querySelector('.border-dashed');
    if (dashedBorder instanceof HTMLElement) {
      const terminalCard = dashedBorder.querySelector<HTMLElement>('[class*="w-32"]');
      if (terminalCard) {
        terminalCard.click();
        expect(onMergeTerminalClick).toHaveBeenCalled();
      }
    }
  });

  it('should handle tasks with single terminal (no connectors)', () => {
    const tasksWithSingleTerminal = [
      {
        id: 'task-1',
        name: 'Single Terminal Task',
        branch: 'feat/single',
        terminals: [
          {
            id: 'terminal-1',
            orderIndex: 0,
            cliTypeId: 'claude-code',
            status: 'not_started' as Terminal['status'],
          },
        ],
      },
    ];

    const { container } = renderWithI18n(
      <PipelineView
        name="Test Workflow"
        status="idle"
        tasks={tasksWithSingleTerminal}
        mergeTerminal={mockMergeTerminal}
      />
    );

    const connectors = container.querySelectorAll(String.raw`.w-8.h-0\.5.bg-muted\/30`);
    expect(connectors.length).toBe(0);
  });

  it('should handle empty tasks array', () => {
    renderWithI18n(
      <PipelineView
        name="Test Workflow"
        status="idle"
        tasks={[]}
        mergeTerminal={mockMergeTerminal}
      />
    );

    expect(screen.getByText('Test Workflow')).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:pipeline.status.idle'))).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:pipeline.emptyTitle'))).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:pipeline.emptyDescription'))).toBeInTheDocument();
    expect(screen.queryByText(i18n.t('workflow:pipeline.mergeTerminalRole'))).not.toBeInTheDocument();
  });

  it('should show the initial goal for empty agent-planned workflows', () => {
    renderWithI18n(
      <PipelineView
        name="Test Workflow"
        status="idle"
        executionMode="agent_planned"
        initialGoal="Coordinate recovery and rebuild the runtime graph"
        tasks={[]}
        mergeTerminal={mockMergeTerminal}
      />
    );

    expect(screen.getByText(i18n.t('workflow:pipeline.emptyDescriptionAgentPlanned'))).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:pipeline.initialGoalLabel') + ':')).toBeInTheDocument();
    expect(screen.getByText('Coordinate recovery and rebuild the runtime graph')).toBeInTheDocument();
  });
});

type PipelineViewProps = React.ComponentProps<typeof PipelineView>;
