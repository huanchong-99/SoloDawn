import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'pipeline.merge': 'Merge',
      };
      return translations[key] ?? key;
    },
  }),
}));

import { MergeTerminalNode } from './MergeTerminalNode';

vi.mock('@/hooks/useWorkflows', () => ({
  useWorkflow: vi.fn(),
}));

import { useWorkflow } from '@/hooks/useWorkflows';

describe('MergeTerminalNode', () => {
  it('renders merge target branch', () => {
    vi.mocked(useWorkflow).mockReturnValue({
      data: { targetBranch: 'main' },
      isLoading: false,
      error: null,
    } as any);

    render(<MergeTerminalNode workflowId="wf-1" />);
    expect(screen.getByText('Merge')).toBeInTheDocument();
    expect(screen.getByText('main')).toBeInTheDocument();
  });
});
