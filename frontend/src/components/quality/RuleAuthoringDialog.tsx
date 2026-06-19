import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  CheckIcon,
  CircleNotchIcon,
  WarningCircleIcon,
  InfoIcon,
} from '@phosphor-icons/react';

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@/components/ui-new/primitives/Dialog';
import { Button } from '@/components/ui-new/primitives/Button';
import { ErrorAlert } from '@/components/ui-new/primitives/ErrorAlert';
import { Tooltip } from '@/components/ui-new/primitives/Tooltip';
import { ToolbarDropdown } from '@/components/ui-new/primitives/Toolbar';
import {
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from '@/components/ui-new/primitives/Dropdown';
import { cn } from '@/lib/utils';
import { customRulesApi } from '@/lib/api';
import { useGenerateRule } from '@/hooks/useQualityPolicy';
import {
  useModelConfigForExecutor,
  type ModelOption,
} from '@/hooks/useModelConfigForExecutor';
import { useUserSystem } from '@/components/ConfigProvider';
import { BaseCodingAgent } from 'shared/types';
import type { ModelConfig as WorkflowModelConfig } from '@/components/workflow/types';
import type {
  AuthorRuleResult,
  CustomRuleDraft,
  CustomRuleInput,
  ConditionConfig,
  MetricKey,
  RuleExample,
  JsonValue,
} from 'shared/types';

/**
 * The model-picker executor is fixed to Claude (the authoring pipeline drives
 * the `claude` transport for subscription and a generic LLMClient for metered).
 * `useModelConfigForExecutor` derives the cli_type id from this enum value.
 */
const AUTHORING_EXECUTOR = BaseCodingAgent.CLAUDE_CODE;
/**
 * Same id `useModelConfigForExecutor` derives from {@link AUTHORING_EXECUTOR}
 * (`CLAUDE_CODE` → `cli-claude-code`); the seed for the Claude CLI type in
 * `20260117000001_create_workflow_tables.sql`.
 */
const AUTHORING_CLI_TYPE_ID = 'cli-claude-code';

/** Pipeline stages shown in the running stepper (PRD §7.2 / §8.6). */
type Stage = 'proposer' | 'adversary' | 'empirical' | 'judge';
const STAGES: readonly Stage[] = ['proposer', 'adversary', 'empirical', 'judge'];

export interface RuleAuthoringDialogProps {
  /** Whether the dialog is open. */
  readonly open: boolean;
  /** Close the dialog without confirming. */
  readonly onClose: () => void;
  /** The project the authored rule belongs to (D4: non-null in v1 UI). */
  readonly projectId: string;
  /** The gate the parent is editing (informational context only in P1). */
  readonly gateKey?: string;
  /**
   * The live editor conditions, serialized as proposer context so the model
   * can avoid duplicating an existing rule.
   */
  readonly currentConditions?: ConditionConfig[];
  /**
   * Called after the candidate is persisted via `customRulesApi.create`. The
   * `mappedMetric` is the candidate's `MetricKey::as_str()` token (or undefined
   * when the rule maps to no metric) so the parent can splice a matching
   * `ConditionConfig` into the live gate config.
   */
  readonly onConfirmed: (
    draft: CustomRuleDraft,
    mappedMetric?: MetricKey
  ) => void;
}

/** A row is unauthorable when its (custom) subtitle advertises a google apiType. */
function isGoogleSource(model: ModelOption): boolean {
  return /\bgoogle\b/i.test(model.subtitle ?? '');
}

/**
 * Map a finished authoring candidate (+ its carried fixtures) onto the
 * `CustomRuleInput` the create endpoint expects. The candidate omits the
 * scoping/provenance arrays, so they default to empty; the NL ask is preserved
 * for round-trip/reproducibility.
 */
function draftToInput(
  draft: CustomRuleDraft,
  examples: RuleExample[],
  nlRequest: string
): CustomRuleInput {
  return {
    nlRequest: nlRequest.trim() ? nlRequest.trim() : null,
    ruleFormat: draft.ruleFormat,
    ruleBody: draft.ruleBody,
    name: draft.name,
    description: draft.description ?? null,
    message: draft.message,
    ruleType: draft.ruleType,
    severity: draft.severity,
    languages: [],
    extensions: [],
    includeGlobs: [],
    excludeGlobs: [],
    mappedMetric: draft.mappedMetric,
    examples,
  };
}

/**
 * AI rule-authoring dialog (PRD §7.2 / §11.2, decision D1).
 *
 * Drives the multi-agent authoring pipeline from a natural-language ask plus a
 * user-selected model source, renders the empirical/adversarial evidence, and
 * — on the mandatory human confirm — persists the candidate via
 * `customRulesApi.create` before handing the draft (and its mapped metric) back
 * to the parent gate editor. A `capped_out` outcome or a failed round-trip
 * never dead-ends: the dialog stays on the editable result step with the best
 * candidate prefilled.
 */
export function RuleAuthoringDialog({
  open,
  onClose,
  projectId,
  gateKey,
  currentConditions,
  onConfirmed,
}: Readonly<RuleAuthoringDialogProps>) {
  const { t } = useTranslation(['settings', 'common']);
  const { config } = useUserSystem();

  const workflowModelLibrary = (config as Record<string, unknown> | null)
    ?.workflow_model_library as WorkflowModelConfig[] | undefined;

  const {
    customModels,
    officialModels,
    selectedModelConfigId,
    setSelectedModelConfigId,
  } = useModelConfigForExecutor(AUTHORING_EXECUTOR, workflowModelLibrary);

  const generate = useGenerateRule();
  // `reset` is referentially stable across renders (TanStack v5); capturing it
  // lets the open-effect clear the last run without depending on the whole
  // mutation object (which is recreated every render).
  const resetGenerate = generate.reset;

  const [nlRequest, setNlRequest] = useState('');
  const [result, setResult] = useState<AuthorRuleResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  // Round-trip failures keep the dialog on the edit step with a surfaced warning.
  const [roundTripBlocked, setRoundTripBlocked] = useState(false);

  // Reset transient state whenever the dialog (re)opens.
  useEffect(() => {
    if (!open) return;
    setNlRequest('');
    setResult(null);
    setError(null);
    setSubmitting(false);
    setRoundTripBlocked(false);
    resetGenerate();
  }, [open, resetGenerate]);

  const selectedModel = useMemo<ModelOption | null>(() => {
    const all = [...customModels, ...officialModels];
    return all.find((m) => m.id === selectedModelConfigId) ?? null;
  }, [customModels, officialModels, selectedModelConfigId]);

  // The selected source decides the dispatched backend (mirrors the server's
  // InteractiveAuthMode::resolve): a key-less native/official row routes to the
  // subscription transport; google is rejected outright by the metered path.
  const selectedRoutesToSubscription = !!selectedModel && !selectedModel.hasApiKey;
  const selectedIsGoogle = !!selectedModel && isGoogleSource(selectedModel);

  const pickerLabel =
    selectedModel?.displayName ??
    t('settings:ruleAuthoring.selectModel', { defaultValue: 'Select model' });

  const renderModelItem = useCallback(
    (model: ModelOption) => {
      const google = isGoogleSource(model);
      const subscription = !model.hasApiKey;
      return (
        <DropdownMenuItem
          key={model.id}
          icon={selectedModelConfigId === model.id ? CheckIcon : undefined}
          disabled={google}
          onClick={
            google ? undefined : () => setSelectedModelConfigId(model.id)
          }
        >
          <span className="flex flex-col">
            <span>{model.displayName}</span>
            {model.subtitle && (
              <span className="text-low text-xs">{model.subtitle}</span>
            )}
            {google && (
              <span className="text-error text-xs">
                {t('settings:ruleAuthoring.googleUnsupported', {
                  defaultValue: 'Not supported for authoring',
                })}
              </span>
            )}
            {!google && subscription && (
              <span className="text-low text-xs">
                {t('settings:ruleAuthoring.subscriptionBackend', {
                  defaultValue: 'Uses Claude subscription',
                })}
              </span>
            )}
          </span>
        </DropdownMenuItem>
      );
    },
    [selectedModelConfigId, setSelectedModelConfigId, t]
  );

  const handleGenerate = useCallback(() => {
    if (!selectedModelConfigId || !nlRequest.trim() || selectedIsGoogle) return;

    setError(null);
    setRoundTripBlocked(false);
    setResult(null);

    const currentRulesContext: JsonValue | null = currentConditions?.length
      ? (currentConditions as unknown as JsonValue)
      : null;

    generate.mutate(
      {
        projectId,
        req: {
          nlRequest: nlRequest.trim(),
          modelConfigId: selectedModelConfigId,
          cliTypeId: AUTHORING_CLI_TYPE_ID,
          ruleFormatPreference: 'regex',
          currentRulesContext,
        },
      },
      {
        onSuccess: (res) => {
          setResult(res);
          setRoundTripBlocked(!res.roundTrip.matches);
        },
        onError: (err: unknown) => {
          setError(
            err instanceof Error
              ? err.message
              : t('settings:ruleAuthoring.generateError', {
                  defaultValue: 'Failed to author the rule.',
                })
          );
        },
      }
    );
  }, [
    selectedModelConfigId,
    nlRequest,
    selectedIsGoogle,
    currentConditions,
    generate,
    projectId,
    t,
  ]);

  const handleConfirm = useCallback(async () => {
    if (!result) return;

    setSubmitting(true);
    setError(null);

    try {
      const input = draftToInput(result.candidate, result.examples, nlRequest);
      await customRulesApi.create(projectId, input);
      const mapped = result.candidate.mappedMetric;
      onConfirmed(
        result.candidate,
        mapped ? (mapped as MetricKey) : undefined
      );
      onClose();
    } catch (err: unknown) {
      setError(
        err instanceof Error
          ? err.message
          : t('settings:ruleAuthoring.confirmError', {
              defaultValue: 'Failed to add the rule.',
            })
      );
    } finally {
      setSubmitting(false);
    }
  }, [result, nlRequest, projectId, onConfirmed, onClose, t]);

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next && !generate.isPending && !submitting) onClose();
    },
    [generate.isPending, submitting, onClose]
  );

  const cappedOut = result?.outcome === 'capped_out';
  const canGenerate =
    !!selectedModelConfigId &&
    !!nlRequest.trim() &&
    !selectedIsGoogle &&
    !generate.isPending;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {t('settings:ruleAuthoring.title', {
              defaultValue: 'Author rule with AI',
            })}
          </DialogTitle>
          <DialogDescription>
            {gateKey
              ? t('settings:ruleAuthoring.descriptionForGate', {
                  defaultValue:
                    'Describe a rule in plain language; AI drafts, attacks, ' +
                    'tests, and verifies it before you add it to {{gate}}.',
                  gate: gateKey,
                })
              : t('settings:ruleAuthoring.description', {
                  defaultValue:
                    'Describe a rule in plain language; AI drafts, attacks, ' +
                    'tests, and verifies it before you add it.',
                })}
          </DialogDescription>
        </DialogHeader>

        <div className="max-h-[60vh] overflow-y-auto py-base flex flex-col gap-base">
          {error && <ErrorAlert message={error} />}

          {/* ---- Model picker (lifted from CreateChatBox) ---- */}
          <div className="flex flex-col gap-half">
            <span className="text-low text-xs font-semibold uppercase tracking-wider">
              {t('settings:ruleAuthoring.modelLabel', {
                defaultValue: 'Authoring model',
              })}
            </span>
            <div className="flex items-center gap-base">
              <ToolbarDropdown label={pickerLabel}>
                {customModels.length > 0 && (
                  <>
                    <DropdownMenuLabel>
                      {t('settings:ruleAuthoring.customModels', {
                        defaultValue: 'Custom',
                      })}
                    </DropdownMenuLabel>
                    {customModels.map(renderModelItem)}
                  </>
                )}
                {officialModels.length > 0 && (
                  <>
                    {customModels.length > 0 && <DropdownMenuSeparator />}
                    <DropdownMenuLabel>
                      {t('settings:ruleAuthoring.officialModels', {
                        defaultValue: 'Official',
                      })}
                    </DropdownMenuLabel>
                    {officialModels.map(renderModelItem)}
                  </>
                )}
                {customModels.length === 0 && officialModels.length === 0 && (
                  <DropdownMenuLabel>
                    {t('settings:ruleAuthoring.noModels', {
                      defaultValue: 'No usable models configured',
                    })}
                  </DropdownMenuLabel>
                )}
              </ToolbarDropdown>
              {selectedRoutesToSubscription && (
                <span className="rounded-sm bg-panel px-base py-half text-xs text-normal">
                  {t('settings:ruleAuthoring.subscriptionBadge', {
                    defaultValue: 'Subscription',
                  })}
                </span>
              )}
            </div>
            {selectedIsGoogle && (
              <span className="text-error text-xs">
                {t('settings:ruleAuthoring.googleSelectedHint', {
                  defaultValue:
                    'Google models cannot author rules — pick an ' +
                    'OpenAI- or Anthropic-compatible source.',
                })}
              </span>
            )}
          </div>

          {/* ---- NL request ---- */}
          <div className="flex flex-col gap-half">
            <span className="text-low text-xs font-semibold uppercase tracking-wider">
              {t('settings:ruleAuthoring.requestLabel', {
                defaultValue: 'What should the rule do?',
              })}
            </span>
            <textarea
              className={cn(
                'min-h-[5rem] resize-y px-base py-half bg-secondary rounded border',
                'text-base text-normal placeholder:text-low',
                'focus:outline-none focus:ring-1 focus:ring-brand',
                'disabled:opacity-60 disabled:cursor-not-allowed'
              )}
              placeholder={t('settings:ruleAuthoring.requestPlaceholder', {
                defaultValue:
                  'e.g. Flag any use of unwrap() in non-test Rust code.',
              })}
              value={nlRequest}
              disabled={generate.isPending}
              onChange={(e) => setNlRequest(e.target.value)}
            />
          </div>

          {/* ---- Running stepper ---- */}
          {generate.isPending && (
            <RunningStepper t={t} />
          )}

          {/* ---- Result ---- */}
          {result && !generate.isPending && (
            <ResultPanels
              result={result}
              cappedOut={cappedOut}
              roundTripBlocked={roundTripBlocked}
              t={t}
            />
          )}
        </div>

        <DialogFooter>
          <Button
            variant="ghost"
            size="sm"
            onClick={onClose}
            disabled={generate.isPending || submitting}
          >
            {t('common:buttons.cancel', { defaultValue: 'Cancel' })}
          </Button>
          {!result ? (
            <Button
              variant="primary"
              size="sm"
              onClick={handleGenerate}
              disabled={!canGenerate}
            >
              {generate.isPending
                ? t('settings:ruleAuthoring.generating', {
                    defaultValue: 'Authoring...',
                  })
                : t('settings:ruleAuthoring.generate', {
                    defaultValue: 'Generate',
                  })}
            </Button>
          ) : (
            <>
              <Button
                variant="secondary"
                size="sm"
                onClick={handleGenerate}
                disabled={!canGenerate || submitting}
              >
                {t('settings:ruleAuthoring.regenerate', {
                  defaultValue: 'Regenerate',
                })}
              </Button>
              <Button
                variant="primary"
                size="sm"
                onClick={handleConfirm}
                disabled={submitting || roundTripBlocked}
              >
                {submitting
                  ? t('settings:ruleAuthoring.adding', {
                      defaultValue: 'Adding...',
                    })
                  : t('settings:ruleAuthoring.confirmAdd', {
                      defaultValue: 'Confirm & add',
                    })}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** The Proposer -> Adversary -> Empirical -> Judge progress strip. */
function RunningStepper({
  t,
}: Readonly<{ t: (key: string, opts?: Record<string, unknown>) => string }>) {
  return (
    <div className="flex items-center gap-base rounded border border-border bg-secondary p-base">
      <CircleNotchIcon className="size-icon-sm animate-spin text-brand" weight="bold" />
      <div className="flex items-center gap-half flex-wrap">
        {STAGES.map((stage, i) => (
          <span key={stage} className="flex items-center gap-half">
            <span className="text-base text-normal">
              {t(`settings:ruleAuthoring.stage.${stage}`, {
                defaultValue: stage,
              })}
            </span>
            {i < STAGES.length - 1 && (
              <span className="text-low" aria-hidden="true">
                {'→'}
              </span>
            )}
          </span>
        ))}
      </div>
    </div>
  );
}

/** Backend badge: "subscription" vs "metered" from the engine echo. */
function BackendBadge({
  backend,
  t,
}: Readonly<{
  backend: AuthorRuleResult['engine']['backend'];
  t: (key: string, opts?: Record<string, unknown>) => string;
}>) {
  const subscription = backend === 'subscription';
  return (
    <span
      className={cn(
        'rounded-sm px-base py-half text-xs',
        subscription ? 'bg-brand/15 text-brand' : 'bg-panel text-normal'
      )}
    >
      {subscription
        ? t('settings:ruleAuthoring.subscriptionBadge', {
            defaultValue: 'Subscription',
          })
        : t('settings:ruleAuthoring.meteredBadge', {
            defaultValue: 'Metered',
          })}
    </span>
  );
}

/** Round-trip verdict pill (green pass / red fail) + reconstructed request. */
function RoundTripBadge({
  result,
  t,
}: Readonly<{
  result: AuthorRuleResult;
  t: (key: string, opts?: Record<string, unknown>) => string;
}>) {
  const ok = result.roundTrip.matches;
  return (
    <div
      className={cn(
        'flex flex-col gap-half rounded border p-base',
        ok ? 'border-success/40 bg-success/10' : 'border-error/40 bg-error/10'
      )}
    >
      <div className="flex items-center gap-half">
        {ok ? (
          <CheckIcon className="size-icon-xs text-success" weight="bold" />
        ) : (
          <WarningCircleIcon className="size-icon-xs text-error" weight="bold" />
        )}
        <span
          className={cn(
            'text-base font-semibold',
            ok ? 'text-success' : 'text-error'
          )}
        >
          {ok
            ? t('settings:ruleAuthoring.roundTripOk', {
                defaultValue: 'Round-trip verified',
              })
            : t('settings:ruleAuthoring.roundTripFailed', {
                defaultValue: 'Round-trip failed — edit before adding',
              })}
        </span>
      </div>
      <span className="text-low text-xs">
        {t('settings:ruleAuthoring.reconstructed', {
          defaultValue: 'Reconstructed intent:',
        })}{' '}
        {result.roundTrip.reconstructedRequest}
      </span>
    </div>
  );
}

/** Candidate + empirical evidence + adversarial transcript panels. */
function ResultPanels({
  result,
  cappedOut,
  roundTripBlocked,
  t,
}: Readonly<{
  result: AuthorRuleResult;
  cappedOut: boolean;
  roundTripBlocked: boolean;
  t: (key: string, opts?: Record<string, unknown>) => string;
}>) {
  const { candidate, empirical, debate } = result;
  return (
    <div className="flex flex-col gap-base">
      {/* Engine + round-trip status row */}
      <div className="flex items-center gap-base flex-wrap">
        <BackendBadge backend={result.engine.backend} t={t} />
        <span className="text-low text-xs">
          {t('settings:ruleAuthoring.roundsUsed', {
            defaultValue: 'Rounds: {{n}}',
            n: result.roundsUsed,
          })}
        </span>
      </div>

      {/* capped_out: never a dead end — hand the best candidate back to edit. */}
      {cappedOut && (
        <div className="flex items-start gap-half rounded border border-error/40 bg-error/10 p-base">
          <WarningCircleIcon
            className="size-icon-sm text-error shrink-0"
            weight="bold"
          />
          <span className="text-base text-error">
            {t('settings:ruleAuthoring.cappedOut', {
              defaultValue:
                "AI couldn't converge — the best candidate is shown below. " +
                'Edit it manually before adding.',
            })}
          </span>
        </div>
      )}

      <RoundTripBadge result={result} t={t} />

      {/* (a) Candidate rule body + plain-language description */}
      <section className="flex flex-col gap-half rounded border border-border bg-secondary p-base">
        <div className="flex items-center gap-half">
          <span className="text-base font-semibold text-high">
            {candidate.name}
          </span>
          {candidate.description && (
            <Tooltip content={candidate.description}>
              <span
                className="inline-flex cursor-help text-low"
                aria-label={candidate.description}
              >
                <InfoIcon className="size-icon-xs" weight="bold" />
              </span>
            </Tooltip>
          )}
          <span className="ml-auto rounded-sm bg-panel px-base py-half text-xs text-normal">
            {candidate.severity}
          </span>
        </div>
        {candidate.description && (
          <p className="text-low text-xs">{candidate.description}</p>
        )}
        <pre className="overflow-x-auto rounded-sm bg-primary p-half text-xs font-ibm-plex-mono text-normal">
          {candidate.ruleBody}
        </pre>
        <span className="text-low text-xs">
          {t('settings:ruleAuthoring.format', { defaultValue: 'Format:' })}{' '}
          {candidate.ruleFormat}
          {candidate.mappedMetric ? ` · ${candidate.mappedMetric}` : ''}
        </span>
      </section>

      {/* (b) Empirical evidence table */}
      <section className="flex flex-col gap-half">
        <span className="text-low text-xs font-semibold uppercase tracking-wider">
          {t('settings:ruleAuthoring.evidence', {
            defaultValue: 'Empirical evidence',
          })}{' '}
          ({empirical.passed}/{empirical.total})
        </span>
        {!empirical.compiled && (
          <ErrorAlert
            message={
              empirical.compileError ??
              t('settings:ruleAuthoring.compileFailed', {
                defaultValue: 'The rule did not compile.',
              })
            }
          />
        )}
        {empirical.perExample.length === 0 ? (
          <p className="text-low text-xs italic">
            {t('settings:ruleAuthoring.noExamples', {
              defaultValue: 'No examples were run.',
            })}
          </p>
        ) : (
          <div className="overflow-x-auto rounded border border-border">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-left text-low">
                  <th className="px-half py-half font-semibold">
                    {t('settings:ruleAuthoring.colKind', {
                      defaultValue: 'Kind',
                    })}
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
                {empirical.perExample.map((ex, i) => (
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
                      {ex.matchCount > 0 ? ` (${ex.matchCount})` : ''}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* (c) Adversarial debate transcript */}
      <section className="flex flex-col gap-half">
        <span className="text-low text-xs font-semibold uppercase tracking-wider">
          {t('settings:ruleAuthoring.debate', {
            defaultValue: 'Adversarial review',
          })}{' '}
          ({debate.revisions}{' '}
          {t('settings:ruleAuthoring.revisions', {
            defaultValue: 'revisions',
          })}
          )
        </span>
        {debate.proposerNotes && (
          <div className="flex flex-col gap-half rounded border border-border bg-secondary p-base">
            <span className="text-low text-xs font-semibold">
              {t('settings:ruleAuthoring.proposerNotes', {
                defaultValue: 'Proposer',
              })}
            </span>
            <p className="text-xs text-normal whitespace-pre-wrap">
              {debate.proposerNotes}
            </p>
          </div>
        )}
        {debate.attackerFindings.length > 0 && (
          <div className="flex flex-col gap-half rounded border border-border bg-secondary p-base">
            <span className="text-low text-xs font-semibold">
              {t('settings:ruleAuthoring.attackerFindings', {
                defaultValue: 'Adversary findings',
              })}
            </span>
            <ul className="list-disc pl-base text-xs text-normal space-y-half">
              {debate.attackerFindings.map((finding, i) => (
                <li key={`finding-${i}-${finding.slice(0, 16)}`}>{finding}</li>
              ))}
            </ul>
          </div>
        )}
      </section>

      {roundTripBlocked && (
        <p className="text-error text-xs">
          {t('settings:ruleAuthoring.roundTripBlockedHint', {
            defaultValue:
              'Resolve the round-trip mismatch (regenerate or edit) before adding.',
          })}
        </p>
      )}
    </div>
  );
}

export default RuleAuthoringDialog;
