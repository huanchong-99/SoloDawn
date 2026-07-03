import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import type { RequirementItemResponse } from '@/lib/api';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) => {
      if (key === 'conversation.planning.ledger.progress') {
        return `${opts?.delivered}/${opts?.total} delivered`;
      }
      // Leaf-name identity keeps assertions readable: last path segment.
      return key.split('.').pop() as string;
    },
  }),
}));

const items: RequirementItemResponse[] = [
  {
    id: 'item-1',
    projectId: 'proj-1',
    pointCode: 'RP-001',
    text: 'memo CRUD works',
    status: 'delivered',
    originDraftId: 'draft-1',
    contextCapsule: JSON.stringify({
      built: 'CRUD endpoints',
      livesWhere: 'src/memo/',
      decisions: 'sqlite over file store',
      extensionNotes: 'add handlers next to the existing routes',
    }),
    provenanceWorkflowId: 'wf-1',
    provenanceCommits: '3 files changed',
    createdAt: '2026-07-03T00:00:00Z',
    updatedAt: '2026-07-03T00:00:00Z',
    deliveredAt: '2026-07-03T01:00:00Z',
  },
  {
    id: 'item-2',
    projectId: 'proj-1',
    pointCode: 'RP-002',
    text: 'reminders fire on time',
    status: 'pending',
    originDraftId: 'draft-2',
    contextCapsule: null,
    provenanceWorkflowId: null,
    provenanceCommits: null,
    createdAt: '2026-07-03T00:00:00Z',
    updatedAt: '2026-07-03T00:00:00Z',
    deliveredAt: null,
  },
];

vi.mock('@/lib/api', () => ({
  requirementItemsApi: {
    list: vi.fn().mockImplementation(() => Promise.resolve(items)),
    update: vi.fn(),
    remove: vi.fn(),
  },
}));

import { parseCapsule } from '@/hooks/useRequirementItems';
import { RequirementLedgerPanel } from '../RequirementLedgerPanel';

function renderPanel(projectId: string | null = 'proj-1') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <RequirementLedgerPanel projectId={projectId} />
    </QueryClientProvider>
  );
}

describe('parseCapsule', () => {
  it('returns null when absent or malformed', () => {
    expect(parseCapsule(items[1])).toBeNull();
    expect(
      parseCapsule({ ...items[1], contextCapsule: 'not json' })
    ).toBeNull();
  });

  it('parses a capsule and defaults missing fields to empty strings', () => {
    expect(parseCapsule(items[0])?.livesWhere).toBe('src/memo/');
    const partial = parseCapsule({
      ...items[0],
      contextCapsule: '{"built":"only this"}',
    });
    expect(partial?.built).toBe('only this');
    expect(partial?.decisions).toBe('');
  });
});

describe('RequirementLedgerPanel', () => {
  it('stays invisible without a project', () => {
    const { container } = renderPanel(null);
    expect(container).toBeEmptyDOMElement();
  });

  it('starts as a collapsed tab and expands to the point list', async () => {
    renderPanel();
    // Collapsed tab (vertical label) once items load.
    fireEvent.click(await screen.findByTitle('panelTab'));

    expect(screen.getByText('panelTitle')).toBeInTheDocument();
    expect(screen.getByText('1/2 delivered')).toBeInTheDocument();
    expect(screen.getByText('RP-001')).toBeInTheDocument();
    expect(screen.getByText('reminders fire on time')).toBeInTheDocument();
  });

  it('shows the compressed capsule of a delivered point on demand', async () => {
    renderPanel();
    fireEvent.click(await screen.findByTitle('panelTab'));

    // Delivered point exposes its capsule toggle; pending one does not.
    const toggles = screen.getAllByText('showCapsule');
    expect(toggles).toHaveLength(1);
    fireEvent.click(toggles[0]);

    expect(screen.getByText(/src\/memo\//)).toBeInTheDocument();
    expect(screen.getByText(/sqlite over file store/)).toBeInTheDocument();
    expect(screen.getByText(/3 files changed/)).toBeInTheDocument();
  });

  it('offers edit/delete only on pending points', async () => {
    renderPanel();
    fireEvent.click(await screen.findByTitle('panelTab'));

    // One pending point → exactly one edit and one delete affordance.
    expect(screen.getAllByText('edit')).toHaveLength(1);
    expect(screen.getAllByText('delete')).toHaveLength(1);
  });
});
