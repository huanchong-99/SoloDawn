import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect } from 'vitest';
import type { WorkflowTaskDto } from 'shared/types';
import { TaskCard } from './TaskCard';

const task: WorkflowTaskDto = {
  id: 'task-1',
  workflowId: 'wf-1',
  vkTaskId: null,
  name: 'Task One',
  description: null,
  branch: 'feature/task-one',
  status: 'created',
  orderIndex: 0,
  startedAt: null,
  completedAt: null,
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z',
  terminals: [
    {
      id: 'term-1',
      workflowTaskId: 'task-1',
      cliTypeId: 'claude-code',
      modelConfigId: 'model-1',
      customBaseUrl: null,
      customApiKey: null,
      role: 'Planner',
      roleDescription: null,
      orderIndex: 0,
      status: 'working',
      createdAt: '2024-01-01T00:00:00Z',
      updatedAt: '2024-01-01T00:00:00Z',
    },
    {
      id: 'term-2',
      workflowTaskId: 'task-1',
      cliTypeId: 'qwen-code',
      modelConfigId: 'model-2',
      customBaseUrl: null,
      customApiKey: null,
      role: 'Coder',
      roleDescription: null,
      orderIndex: 1,
      status: 'completed',
      createdAt: '2024-01-01T00:00:00Z',
      updatedAt: '2024-01-01T00:00:00Z',
    },
  ],
};

describe('TaskCard', () => {
  it('renders task details and terminal count', () => {
    render(
      <MemoryRouter>
        <TaskCard task={task} workflowId="wf-1" />
      </MemoryRouter>
    );

    expect(screen.getByText('Task One')).toBeInTheDocument();
    expect(screen.getByText('feature/task-one')).toBeInTheDocument();
    expect(screen.getAllByTestId('terminal-dot')).toHaveLength(2);
  });
});
