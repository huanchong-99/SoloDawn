import { render, screen, fireEvent, within } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach, beforeAll } from 'vitest';

import i18n from '@/i18n/config';
import type { CustomRule, QualityGateConfig, MetricKey } from 'shared/types';

// ---- Mocks ------------------------------------------------------------------
// The custom-rules section drives the data hooks; the child dialogs pull in the
// model picker / ConfigProvider, so stub them to keep this test focused on the
// section's own list + lifecycle controls (PRD §11.3, D2/D8).

const mockSetStatusMutate = vi.fn();
const mockDeleteMutate = vi.fn();
let mockRules: CustomRule[] = [];

vi.mock('@/hooks/useQualityPolicy', () => ({
  useCustomRules: () => ({
    data: mockRules,
    isLoading: false,
    isFetching: false,
    isError: false,
  }),
  useSetCustomRuleStatus: () => ({
    mutate: mockSetStatusMutate,
    isPending: false,
    isError: false,
  }),
  useDeleteCustomRule: () => ({
    mutate: mockDeleteMutate,
    isPending: false,
    isError: false,
  }),
}));

vi.mock('../RuleAuthoringDialog', () => ({
  RuleAuthoringDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="authoring-dialog" /> : null,
}));

vi.mock('../CustomRuleEditDialog', () => ({
  CustomRuleEditDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="edit-dialog" /> : null,
}));

import { QualityGateRulesEditor } from '../QualityGateRulesEditor';

const METRIC_OPTIONS: MetricKey[] = ['cargo_check_errors', 'clippy_warnings'];

function makeConfig(): QualityGateConfig {
  return {
    mode: 'enforce',
    terminal_gate: { name: 'terminal', conditions: [] },
    branch_gate: { name: 'branch', conditions: [] },
    repo_gate: { name: 'repo', conditions: [] },
    providers: {
      rust: true,
      frontend: true,
      repo: true,
      security: false,
      sonar: false,
      builtin_rust: true,
      builtin_frontend: true,
      builtin_common: true,
      coverage: false,
      completeness: false,
      delivery_readiness: false,
    },
    sonar: { host_url: '', project_key: '', token: null },
  };
}

function makeRule(over: Partial<CustomRule> = {}): CustomRule {
  return {
    id: 'rule-1',
    projectId: 'proj-1',
    name: 'No unwrap in prod',
    nlRequest: 'flag unwrap() outside tests',
    ruleFormat: 'regex',
    ruleBody: '\\.unwrap\\(\\)',
    description: 'Flags unwrap() in non-test Rust.',
    ruleType: 'CodeSmell',
    severity: 'MAJOR',
    mappedMetric: null,
    enabled: true,
    status: 'shadow',
    createdBy: null,
    version: 1n,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...over,
  };
}

function renderEditor(projectId?: string) {
  return render(
    <QualityGateRulesEditor
      value={makeConfig()}
      defaults={makeConfig()}
      metricOptions={METRIC_OPTIONS}
      onChange={vi.fn()}
      projectId={projectId}
    />
  );
}

describe('QualityGateRulesEditor custom-rules section (PRD §11.3)', () => {
  // Use English resources so the interpolated labels (e.g. "→ warn") and copy
  // are deterministic; the app default is zh-Hans.
  beforeAll(async () => {
    await i18n.changeLanguage('en');
  });

  beforeEach(() => {
    mockRules = [];
    mockSetStatusMutate.mockClear();
    mockDeleteMutate.mockClear();
  });

  it('hides the section + AI button when no projectId (backward compatible)', () => {
    renderEditor(undefined);
    expect(screen.queryByText('Custom Rules')).not.toBeInTheDocument();
    expect(
      screen.queryByText('Generate rule with AI')
    ).not.toBeInTheDocument();
  });

  it('renders the section, AI buttons, and a rule row with its status badge', () => {
    mockRules = [makeRule()];
    renderEditor('proj-1');

    expect(screen.getByText('Custom Rules')).toBeInTheDocument();
    // One "Generate rule with AI" button per gate header (3 gates).
    expect(screen.getAllByText('Generate rule with AI')).toHaveLength(3);
    const row = screen.getByText('No unwrap in prod').closest('li');
    expect(row).not.toBeNull();
    // Status badge renders the rule's status verbatim (scoped to the row so the
    // Mode <select>'s "shadow" option doesn't collide).
    expect(within(row as HTMLElement).getByText('shadow')).toBeInTheDocument();
  });

  it('promotes shadow -> warn via the manual promotion control (never auto)', () => {
    mockRules = [makeRule({ status: 'shadow' })];
    renderEditor('proj-1');

    fireEvent.click(screen.getByText('→ warn'));
    expect(mockSetStatusMutate).toHaveBeenCalledWith({
      projectId: 'proj-1',
      ruleId: 'rule-1',
      status: 'warn',
    });
  });

  it('disable parks the rule at status=disabled', () => {
    mockRules = [makeRule({ status: 'warn' })];
    renderEditor('proj-1');

    fireEvent.click(screen.getByText('Disable'));
    expect(mockSetStatusMutate).toHaveBeenCalledWith({
      projectId: 'proj-1',
      ruleId: 'rule-1',
      status: 'disabled',
    });
  });

  it('re-enable returns a disabled rule to shadow (not an enforcing status)', () => {
    mockRules = [makeRule({ status: 'disabled' })];
    renderEditor('proj-1');

    fireEvent.click(screen.getByText('Enable'));
    expect(mockSetStatusMutate).toHaveBeenCalledWith({
      projectId: 'proj-1',
      ruleId: 'rule-1',
      status: 'shadow',
    });
  });

  it('delete calls the delete mutation for the row', () => {
    mockRules = [makeRule()];
    renderEditor('proj-1');

    fireEvent.click(screen.getByLabelText('Delete'));
    expect(mockDeleteMutate).toHaveBeenCalledWith({
      projectId: 'proj-1',
      ruleId: 'rule-1',
    });
  });

  it('opening "Generate rule with AI" mounts the authoring dialog', () => {
    renderEditor('proj-1');
    expect(screen.queryByTestId('authoring-dialog')).not.toBeInTheDocument();
    fireEvent.click(screen.getAllByText('Generate rule with AI')[0]);
    expect(screen.getByTestId('authoring-dialog')).toBeInTheDocument();
  });

  it('Edit opens the edit dialog; Revalidate also opens it', () => {
    mockRules = [makeRule()];
    renderEditor('proj-1');

    fireEvent.click(screen.getByLabelText('Edit'));
    expect(screen.getByTestId('edit-dialog')).toBeInTheDocument();
  });
});
