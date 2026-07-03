import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) => {
      if (key === 'conversation.planning.auditPlan.cardTitle') {
        return 'Acceptance Rubric';
      }
      if (key === 'conversation.planning.auditPlan.cardSummary') {
        return `${opts?.points} scoring points · pass at ${opts?.threshold}`;
      }
      if (key === 'conversation.planning.auditPlan.maxScore') {
        return `${opts?.max} pts`;
      }
      return key;
    },
  }),
}));

import { AuditPlanCard, parseAuditPlan } from '../AuditPlanCard';

const PLAN = JSON.stringify({
  mode: 'builtin',
  dimensions: [
    {
      name: 'functional_completeness',
      name_zh: '功能完整性',
      max_score: 25,
      criteria: ['[RP-001] memo CRUD works', '[RP-002] reminders fire on time'],
      sub_dimensions: null,
    },
    {
      name: 'code_quality',
      name_zh: '代码质量',
      max_score: 30,
      criteria: ['no dead code'],
      sub_dimensions: [
        {
          name: 'security',
          name_zh: '安全',
          max_score: 10,
          criteria: [],
          sub_dimensions: null,
        },
      ],
    },
  ],
  pass_threshold: 90,
  generated_at: '2026-07-03T00:00:00Z',
  raw_principles: '',
});

describe('parseAuditPlan', () => {
  it('returns null for null / malformed / shape-mismatched input', () => {
    expect(parseAuditPlan(null)).toBeNull();
    expect(parseAuditPlan('not json')).toBeNull();
    expect(parseAuditPlan('{"pass_threshold":90}')).toBeNull();
  });

  it('parses a valid plan', () => {
    const plan = parseAuditPlan(PLAN);
    expect(plan?.pass_threshold).toBe(90);
    expect(plan?.dimensions).toHaveLength(2);
  });
});

describe('AuditPlanCard', () => {
  it('renders nothing when the plan is missing or invalid', () => {
    const { container } = render(<AuditPlanCard auditPlanJson={null} />);
    expect(container).toBeEmptyDOMElement();
    const { container: c2 } = render(<AuditPlanCard auditPlanJson="broken" />);
    expect(c2).toBeEmptyDOMElement();
  });

  it('summarizes functional points and threshold in the header', () => {
    render(<AuditPlanCard auditPlanJson={PLAN} />);
    expect(screen.getByText('Acceptance Rubric')).toBeInTheDocument();
    // 2 functional_completeness criteria; code_quality criteria don't count.
    expect(
      screen.getByText('2 scoring points · pass at 90')
    ).toBeInTheDocument();
    // Collapsed by default: criteria hidden.
    expect(screen.queryByText('memo CRUD works')).not.toBeInTheDocument();
  });

  it('expands to show dimensions, RP point tags, and sub-dimensions', () => {
    render(<AuditPlanCard auditPlanJson={PLAN} />);
    fireEvent.click(screen.getByRole('button'));

    expect(screen.getByText('功能完整性')).toBeInTheDocument();
    expect(screen.getByText('25 pts')).toBeInTheDocument();
    // "[RP-001] text" is split into a code tag + plain text.
    expect(screen.getByText('RP-001')).toBeInTheDocument();
    expect(screen.getByText('memo CRUD works')).toBeInTheDocument();
    // Untagged criterion renders without a code tag.
    expect(screen.getByText('no dead code')).toBeInTheDocument();
    // Sub-dimension line.
    expect(screen.getByText(/安全/)).toBeInTheDocument();
  });

  it('honors defaultExpanded', () => {
    render(<AuditPlanCard auditPlanJson={PLAN} defaultExpanded />);
    expect(screen.getByText('memo CRUD works')).toBeInTheDocument();
  });
});
