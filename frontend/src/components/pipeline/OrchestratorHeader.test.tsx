import { render, screen } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'pipeline.orchestrator.statusLabel': 'Status:',
        'pipeline.orchestrator.modelLabel': 'Model:',
        'pipeline.orchestrator.tokensUsedLabel': 'Tokens Used',
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

  it('displays N/A when tokensUsed is not provided', () => {
    render(<OrchestratorHeader name="Test" status="idle" model={null} />);
    expect(screen.getByText('N/A')).toBeInTheDocument();
  });

  it('formats token count in thousands', () => {
    render(<OrchestratorHeader name="Test" status="idle" model={null} tokensUsed={12500} />);
    expect(screen.getByText('12.5k')).toBeInTheDocument();
  });

  it('formats token count in millions', () => {
    render(<OrchestratorHeader name="Test" status="idle" model={null} tokensUsed={1500000} />);
    expect(screen.getByText('1.5M')).toBeInTheDocument();
  });

  it('displays raw number for small token counts', () => {
    render(<OrchestratorHeader name="Test" status="idle" model={null} tokensUsed={500} />);
    expect(screen.getByText('500')).toBeInTheDocument();
  });
});
