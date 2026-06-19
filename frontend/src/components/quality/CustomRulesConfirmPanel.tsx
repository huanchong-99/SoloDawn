import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  CheckIcon,
  WarningCircleIcon,
  CircleNotchIcon,
} from '@phosphor-icons/react';

import { cn } from '@/lib/utils';
import {
  useCustomRules,
  useCustomRuleValidations,
} from '@/hooks/useQualityPolicy';
import type {
  CustomRule,
  CustomRuleValidation,
  EmpiricalReport,
  RoundTripVerdict,
  ExampleResult,
} from 'shared/types';

/** The shape the authoring pipeline serializes into `validation.resultsJson`. */
interface PersistedResults {
  empirical?: EmpiricalReport;
  round_trip?: RoundTripVerdict;
  debate?: unknown;
}

function parseResults(json: string | null): PersistedResults | null {
  if (!json) return null;
  try {
    return JSON.parse(json) as PersistedResults;
  } catch {
    return null;
  }
}

/** A rule counts as "active" (enforced against the run) once past draft. */
function isActiveRule(rule: CustomRule): boolean {
  return (
    rule.enabled &&
    (rule.status === 'shadow' ||
      rule.status === 'warn' ||
      rule.status === 'enforce')
  );
}

const sectionCls =
  'flex flex-col gap-half rounded border border-border bg-secondary p-base';

/**
 * Read-only evidence panel for the G2 confirm dialog (PRD §11.4, decision D8).
 *
 * For each ACTIVE custom rule it surfaces the rule body, the AI-generated
 * description, the positive/negative example results, the empirical pass/fail
 * tallies and the round-trip verdict — sourced from {@link useCustomRules} plus
 * the latest validation artifact. It NEVER mutates: the human must still click
 * Save & Confirm in the parent dialog (the only path past the materialize block).
 */
export function CustomRulesConfirmPanel({
  projectId,
}: Readonly<{ projectId: string }>) {
  const { t } = useTranslation(['settings', 'quality']);
  const rulesQuery = useCustomRules(projectId);

  const activeRules = useMemo(
    () => (rulesQuery.data ?? []).filter(isActiveRule),
    [rulesQuery.data]
  );

  if (rulesQuery.isLoading) {
    return (
      <div className="flex items-center gap-half text-low text-base py-base">
        <CircleNotchIcon className="size-icon-xs animate-spin" weight="bold" />
        {t('quality:rulesEditor.customRules.confirmLoading', {
          defaultValue: 'Loading custom rules...',
        })}
      </div>
    );
  }

  if (activeRules.length === 0) {
    return null;
  }

  return (
    <div className="flex flex-col gap-base">
      <div className="flex flex-col gap-half">
        <span className="text-low text-xs font-semibold uppercase tracking-wider">
          {t('quality:rulesEditor.customRules.confirmTitle', {
            defaultValue: 'Custom rules enforced this run',
          })}
        </span>
        <p className="text-low text-xs">
          {t('quality:rulesEditor.customRules.confirmIntro', {
            defaultValue:
              'Review the AI-authored rules and their validation evidence before confirming.',
          })}
        </p>
      </div>

      {activeRules.map((rule) => (
        <CustomRuleEvidence key={rule.id} projectId={projectId} rule={rule} />
      ))}
    </div>
  );
}

/** One rule's body + description + latest validation evidence (all read-only). */
function CustomRuleEvidence({
  projectId,
  rule,
}: Readonly<{ projectId: string; rule: CustomRule }>) {
  const { t } = useTranslation(['settings', 'quality']);
  const validationsQuery = useCustomRuleValidations(projectId, rule.id);

  const latest: CustomRuleValidation | undefined = validationsQuery.data?.[0];
  const parsed = useMemo(
    () => parseResults(latest?.resultsJson ?? null),
    [latest?.resultsJson]
  );
  const perExample: ExampleResult[] = parsed?.empirical?.perExample ?? [];
  const roundTrip = parsed?.round_trip ?? null;
  const roundtripOk = latest?.roundtripOk ?? roundTrip?.matches ?? null;

  return (
    <section className={sectionCls}>
      {/* Header: name + status + round-trip verdict pill */}
      <div className="flex items-center gap-half flex-wrap">
        <span className="text-base font-semibold text-high">{rule.name}</span>
        <span className="rounded-sm bg-panel px-base py-half text-xs text-normal capitalize">
          {rule.status}
        </span>
        {roundtripOk !== null && (
          <span
            className={cn(
              'ml-auto inline-flex items-center gap-half rounded-sm px-base py-half text-xs',
              roundtripOk
                ? 'bg-success/15 text-success'
                : 'bg-error/15 text-error'
            )}
          >
            {roundtripOk ? (
              <CheckIcon className="size-icon-xs" weight="bold" />
            ) : (
              <WarningCircleIcon className="size-icon-xs" weight="bold" />
            )}
            {roundtripOk
              ? t('quality:rulesEditor.customRules.roundTripOk', {
                  defaultValue: 'Round-trip verified',
                })
              : t('quality:rulesEditor.customRules.roundTripFailed', {
                  defaultValue: 'Round-trip failed',
                })}
          </span>
        )}
      </div>

      {/* Generated description */}
      {rule.description && (
        <p className="text-low text-xs">{rule.description}</p>
      )}

      {/* Rule body */}
      <pre className="overflow-x-auto rounded-sm bg-primary p-half text-xs font-ibm-plex-mono text-normal">
        {rule.ruleBody}
      </pre>

      {/* Empirical tally */}
      {latest && (
        <span className="text-low text-xs">
          {t('quality:rulesEditor.customRules.evidenceTally', {
            defaultValue: 'Examples passed: {{passed}}/{{total}}',
            passed: String(latest.examplesPassed),
            total: String(latest.examplesTotal),
          })}
          {latest.judgeScore !== null
            ? ` · ${t('quality:rulesEditor.customRules.judgeScore', {
                defaultValue: 'Judge score {{score}}',
                score: String(latest.judgeScore),
              })}`
            : ''}
        </span>
      )}

      {/* Per-example results (positive/negative) */}
      {perExample.length > 0 && (
        <div className="overflow-x-auto rounded border border-border">
          <table className="w-full text-xs">
            <thead>
              <tr className="text-left text-low">
                <th className="px-half py-half font-semibold">
                  {t('settings:ruleAuthoring.colKind', { defaultValue: 'Kind' })}
                </th>
                <th className="px-half py-half font-semibold">
                  {t('settings:ruleAuthoring.colSnippet', {
                    defaultValue: 'Snippet',
                  })}
                </th>
                <th className="px-half py-half font-semibold">
                  {t('settings:ruleAuthoring.colExpected', {
                    defaultValue: 'Expected',
                  })}
                </th>
                <th className="px-half py-half font-semibold">
                  {t('settings:ruleAuthoring.colActual', {
                    defaultValue: 'Actual',
                  })}
                </th>
              </tr>
            </thead>
            <tbody>
              {perExample.map((ex, i) => (
                <tr
                  key={`${ex.kind}-${i}-${ex.snippet.slice(0, 16)}`}
                  className={cn(
                    'border-t border-border align-top',
                    !ex.passed && 'bg-error/10'
                  )}
                >
                  <td className="px-half py-half text-normal">{ex.kind}</td>
                  <td className="px-half py-half">
                    <pre className="max-w-[20rem] overflow-x-auto whitespace-pre-wrap font-ibm-plex-mono text-normal">
                      {ex.snippet}
                    </pre>
                  </td>
                  <td
                    className={cn(
                      'px-half py-half',
                      !ex.passed ? 'text-error font-semibold' : 'text-normal'
                    )}
                  >
                    {ex.expectedMatch
                      ? t('settings:ruleAuthoring.match', {
                          defaultValue: 'match',
                        })
                      : t('settings:ruleAuthoring.noMatch', {
                          defaultValue: 'no match',
                        })}
                  </td>
                  <td
                    className={cn(
                      'px-half py-half',
                      !ex.passed ? 'text-error font-semibold' : 'text-normal'
                    )}
                  >
                    {ex.actualMatch
                      ? t('settings:ruleAuthoring.match', {
                          defaultValue: 'match',
                        })
                      : t('settings:ruleAuthoring.noMatch', {
                          defaultValue: 'no match',
                        })}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Reconstructed intent (round-trip evidence) */}
      {roundTrip?.reconstructedRequest && (
        <span className="text-low text-xs">
          {t('settings:ruleAuthoring.reconstructed', {
            defaultValue: 'Reconstructed intent:',
          })}{' '}
          {roundTrip.reconstructedRequest}
        </span>
      )}

      {validationsQuery.isLoading && (
        <span className="text-low text-xs italic">
          {t('quality:rulesEditor.customRules.evidenceLoading', {
            defaultValue: 'Loading validation evidence...',
          })}
        </span>
      )}
    </section>
  );
}

export default CustomRulesConfirmPanel;
