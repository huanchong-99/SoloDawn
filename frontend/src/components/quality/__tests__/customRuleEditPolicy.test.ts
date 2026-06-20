import { describe, it, expect } from 'vitest';

import type { CustomRule } from 'shared/types';
import type { ModelOption } from '@/hooks/useModelConfigForExecutor';
import {
  isBodyChange,
  isGoogleSource,
  type FormState,
} from '../CustomRuleEditDialog';

// D8 (PRD §11.3): a change to the rule BODY (pattern/severity/type/metric)
// re-runs validation and drops to shadow; a change to ONLY name/description does
// not. `isBodyChange` must mirror `body_changed` in the server route.

function baseRule(): CustomRule {
  return {
    id: 'r1',
    projectId: 'p1',
    name: 'rule',
    nlRequest: 'ask',
    ruleFormat: 'regex',
    ruleBody: 'foo',
    description: 'desc',
    ruleType: 'CodeSmell',
    severity: 'MAJOR',
    mappedMetric: null,
    enabled: true,
    status: 'shadow',
    createdBy: null,
    version: 1n,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
  };
}

function formFrom(rule: CustomRule, over: Partial<FormState> = {}): FormState {
  return {
    name: rule.name,
    description: rule.description ?? '',
    severity: rule.severity,
    ruleType: rule.ruleType,
    mappedMetric: rule.mappedMetric ?? '',
    ruleBody: rule.ruleBody,
    ...over,
  };
}

describe('CustomRuleEditDialog D8 body-change policy', () => {
  it('treats a name-only edit as metadata (no revalidate)', () => {
    const rule = baseRule();
    expect(isBodyChange(rule, formFrom(rule, { name: 'renamed' }))).toBe(false);
  });

  it('treats a description-only edit as metadata (no revalidate)', () => {
    const rule = baseRule();
    expect(
      isBodyChange(rule, formFrom(rule, { description: 'new description' }))
    ).toBe(false);
  });

  it('treats a rule-body change as a body change (revalidate)', () => {
    const rule = baseRule();
    expect(isBodyChange(rule, formFrom(rule, { ruleBody: 'bar' }))).toBe(true);
  });

  it('treats a severity change as a body change (revalidate)', () => {
    const rule = baseRule();
    expect(isBodyChange(rule, formFrom(rule, { severity: 'BLOCKER' }))).toBe(
      true
    );
  });

  it('treats a rule-type change as a body change (revalidate)', () => {
    const rule = baseRule();
    expect(isBodyChange(rule, formFrom(rule, { ruleType: 'Bug' }))).toBe(true);
  });

  it('treats a mapped-metric change as a body change (revalidate)', () => {
    const rule = baseRule();
    expect(
      isBodyChange(rule, formFrom(rule, { mappedMetric: 'clippy_warnings' }))
    ).toBe(true);
  });

  it('normalizes empty mappedMetric to null (no spurious body change)', () => {
    const rule = baseRule(); // mappedMetric: null
    // Form seeds '' for a null metric; that must NOT count as a change.
    expect(isBodyChange(rule, formFrom(rule, { mappedMetric: '' }))).toBe(
      false
    );
  });
});

// FIX B (audit): the edit dialog must refuse a google source for a body edit /
// revalidate (the metered path rejects api_type=google), mirroring
// RuleAuthoringDialog. `isGoogleSource` keys off the row subtitle's apiType.
function modelOption(over: Partial<ModelOption> = {}): ModelOption {
  return {
    id: 'm1',
    displayName: 'Model 1',
    subtitle: null,
    isCustom: false,
    hasApiKey: true,
    ...over,
  };
}

describe('CustomRuleEditDialog google-source guard', () => {
  it('flags a model whose subtitle advertises a google apiType', () => {
    expect(
      isGoogleSource(modelOption({ subtitle: 'google · gemini-2.5-pro' }))
    ).toBe(true);
  });

  it('is case-insensitive on the apiType token', () => {
    expect(isGoogleSource(modelOption({ subtitle: 'Google · gemini' }))).toBe(
      true
    );
  });

  it('does not flag non-google sources', () => {
    expect(
      isGoogleSource(modelOption({ subtitle: 'openai · gpt-4o' }))
    ).toBe(false);
    expect(
      isGoogleSource(modelOption({ subtitle: 'anthropic · claude' }))
    ).toBe(false);
  });

  it('does not flag a substring match (word boundary)', () => {
    // e.g. a hypothetical "googleish" provider name must not trip the guard.
    expect(isGoogleSource(modelOption({ subtitle: 'googleish · x' }))).toBe(
      false
    );
  });

  it('treats a null subtitle (official row) as non-google', () => {
    expect(isGoogleSource(modelOption({ subtitle: null }))).toBe(false);
  });
});
