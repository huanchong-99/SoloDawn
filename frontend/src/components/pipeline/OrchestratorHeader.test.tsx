import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'pipeline.orchestrator.statusLabel': 'Status:',
        'pipeline.orchestrator.modelLabel': 'Model:',
        'pipeline.orchestrator.resume': 'Resume',
        'pipeline.orchestrator.resuming': 'Resuming...',
        'pipeline.orchestrator.pausedHint':
          'Execution is paused — resume to continue.',
      };
      return translations[key] ?? key;
    },
  }),
}));

import { OrchestratorHeader } from './OrchestratorHeader';

describe('OrchestratorHeader', () => {
  it('renders workflow metadata', () => {
    render(<OrchestratorHeader name="Workflow X" status="running" model="gpt-4o" />);

    expect(screen.getByText('Workflow X')).toBeInTheDocument();
    expect(screen.getByText(/status: running/i)).toBeInTheDocument();
    expect(screen.getByText(/model: gpt-4o/i)).toBeInTheDocument();
  });

  it('hides the resume action when the workflow is not resumable', () => {
    render(
      <OrchestratorHeader
        name="Workflow X"
        status="running"
        model="gpt-4o"
        canResume={false}
        onResume={vi.fn()}
      />
    );

    expect(
      screen.queryByRole('button', { name: /resume/i })
    ).not.toBeInTheDocument();
  });

  it('renders a resume action with a paused hint when resumable', async () => {
    const onResume = vi.fn();
    render(
      <OrchestratorHeader
        name="Workflow X"
        status="paused"
        model="gpt-4o"
        canResume
        onResume={onResume}
      />
    );

    expect(
      screen.getByText('Execution is paused — resume to continue.')
    ).toBeInTheDocument();

    const button = screen.getByRole('button', { name: 'Resume' });
    expect(button).toBeEnabled();

    await userEvent.click(button);
    expect(onResume).toHaveBeenCalledTimes(1);
  });

  it('disables the resume action and shows progress while resuming', () => {
    render(
      <OrchestratorHeader
        name="Workflow X"
        status="paused"
        model="gpt-4o"
        canResume
        isResuming
        onResume={vi.fn()}
      />
    );

    expect(screen.getByRole('button', { name: 'Resuming...' })).toBeDisabled();
  });

  it('omits the resume action when no handler is supplied', () => {
    render(
      <OrchestratorHeader
        name="Workflow X"
        status="paused"
        model="gpt-4o"
        canResume
      />
    );

    expect(
      screen.queryByRole('button', { name: /resume/i })
    ).not.toBeInTheDocument();
  });
});
