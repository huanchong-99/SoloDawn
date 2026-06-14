import { render, screen } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { TerminalDetailPanel } from './TerminalDetailPanel';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'pipeline.orchestrator.statusLabel': 'Status:',
        'pipeline.orchestrator.modelLabel': 'Model:',
      };
      return translations[key] ?? key;
    },
  }),
}));

describe('TerminalDetailPanel', () => {
  it('renders terminal details', () => {
    render(<TerminalDetailPanel role="Planner" status="running" model="gpt-4o" />);

    expect(screen.getByText('Planner')).toBeInTheDocument();
    expect(screen.getByText('Status: running')).toBeInTheDocument();
    expect(screen.getByText('Model: gpt-4o')).toBeInTheDocument();
  });
});
