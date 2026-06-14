import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';

import type {
  QualityGateConfig,
  MetricKey,
  ProvidersConfig,
} from 'shared/types';
import { QualityGateRulesEditor } from '../QualityGateRulesEditor';

// ---- Fixtures ---------------------------------------------------------------

// Exactly the 11 ProvidersConfig toggles from crates/quality/src/config.rs.
const PROVIDER_KEYS: ReadonlyArray<keyof ProvidersConfig> = [
  'rust',
  'frontend',
  'repo',
  'security',
  'sonar',
  'builtin_rust',
  'builtin_frontend',
  'builtin_common',
  'coverage',
  'completeness',
  'delivery_readiness',
];

const METRIC_OPTIONS: MetricKey[] = [
  'cargo_check_errors',
  'clippy_warnings',
  'eslint_errors',
  'tsc_errors',
];

function makeConfig(): QualityGateConfig {
  return {
    mode: 'enforce',
    terminal_gate: {
      name: 'terminal',
      conditions: [
        { metric: 'cargo_check_errors', operator: 'GT', threshold: '0' },
      ],
    },
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

describe('QualityGateRulesEditor (spec §6 FE)', () => {
  it('renders exactly 11 provider checkboxes', () => {
    const onChange = vi.fn();
    render(
      <QualityGateRulesEditor
        value={makeConfig()}
        defaults={makeConfig()}
        metricOptions={METRIC_OPTIONS}
        onChange={onChange}
      />
    );

    const checkboxes = screen.getAllByRole('checkbox');
    expect(checkboxes).toHaveLength(11);

    // The 11 are exactly the ProvidersConfig keys (label text is the key).
    for (const key of PROVIDER_KEYS) {
      expect(screen.getByText(key)).toBeInTheDocument();
    }
  });

  it('renders a metric <select> (a picker, NOT a free-text input)', () => {
    const onChange = vi.fn();
    render(
      <QualityGateRulesEditor
        value={makeConfig()}
        defaults={makeConfig()}
        metricOptions={METRIC_OPTIONS}
        onChange={onChange}
      />
    );

    // One condition row exists (terminal_gate). Its metric cell is a <select>,
    // pre-set to the row's metric value — proving the picker, not free text.
    const metricSelect = screen.getByDisplayValue('cargo_check_errors');
    expect(metricSelect.tagName).toBe('SELECT');

    // No text/decimal input is used for the metric (only the threshold is text).
    // The metric select's options come ONLY from metricOptions ⇒ closed enum.
    const options = Array.from(
      (metricSelect as HTMLSelectElement).options
    ).map((o) => o.value);
    expect(options).toEqual(METRIC_OPTIONS);
  });

  it('metric select emits a valid MetricKey on change (closed-enum, no free text)', () => {
    const onChange = vi.fn();
    render(
      <QualityGateRulesEditor
        value={makeConfig()}
        defaults={makeConfig()}
        metricOptions={METRIC_OPTIONS}
        onChange={onChange}
      />
    );

    const metricSelect = screen.getByDisplayValue('cargo_check_errors');
    fireEvent.change(metricSelect, { target: { value: 'eslint_errors' } });

    expect(onChange).toHaveBeenCalledTimes(1);
    const next = onChange.mock.calls[0][0] as QualityGateConfig;
    expect(next.terminal_gate.conditions[0].metric).toBe('eslint_errors');
    // The emitted metric is one of the closed-enum options.
    expect(METRIC_OPTIONS).toContain(next.terminal_gate.conditions[0].metric);
  });
});
