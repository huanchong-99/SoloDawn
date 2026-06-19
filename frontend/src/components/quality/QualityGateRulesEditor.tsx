import { useCallback, useState } from 'react';
import {
  Plus,
  Trash2,
  AlertTriangle,
  Info,
  Sparkles,
  RefreshCw,
  Pencil,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type {
  QualityGateConfig,
  QualityGateMode,
  GateDefinition,
  ConditionConfig,
  ProvidersConfig,
  SonarConfig,
  MetricKey,
  MetricInfo,
  MeasureValue,
  ProjectMetricSnapshot,
  CustomRule,
  CustomRuleDraft,
} from 'shared/types';
import { Button } from '@/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import {
  useCustomRules,
  useDeleteCustomRule,
  useSetCustomRuleStatus,
} from '@/hooks/useQualityPolicy';
import { RuleAuthoringDialog } from '@/components/quality/RuleAuthoringDialog';
import { CustomRuleEditDialog } from '@/components/quality/CustomRuleEditDialog';

export interface QualityGateRulesEditorProps {
  value: QualityGateConfig;
  defaults: QualityGateConfig;
  /** Picker source from GET /quality/policy/metrics — the closed MetricKey enum (sentinel excluded). */
  metricOptions: MetricKey[];
  onChange: (next: QualityGateConfig) => void;
  readOnly?: boolean;
  /** Field-level validation errors to surface (one human-readable string per offending condition). */
  errors?: string[];
  /**
   * Self-documenting tooltip catalog (D7 / PRD §7.1): one entry per selectable
   * metric. Optional + backward-compatible — when absent the indicator hides.
   */
  metricInfo?: MetricInfo[];
  /**
   * Latest persisted-run metric snapshot. Pure display only — rendering a
   * tooltip NEVER triggers a recompute (the snapshot is passed in, not fetched).
   */
  currentValues?: ProjectMetricSnapshot;
  /**
   * The project whose AI-authored custom rules are managed (PRD §11.3, D2).
   * Optional + backward-compatible: when absent, the custom-rules management
   * section and the per-gate "Generate rule with AI" buttons are hidden (the
   * editor stays a pure gate-condition editor for project-less consumers).
   */
  projectId?: string;
}

const MODES: QualityGateMode[] = ['off', 'shadow', 'warn', 'enforce'];
const OPERATORS = ['GT', 'LT'] as const;

/** The exact 11 provider toggles from crates/quality/src/config.rs (ProvidersConfig). */
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

type GateKey = 'terminal_gate' | 'branch_gate' | 'repo_gate';

const GATE_SECTIONS: ReadonlyArray<{ key: GateKey; title: string }> = [
  { key: 'terminal_gate', title: 'Terminal Gate' },
  { key: 'branch_gate', title: 'Branch Gate' },
  { key: 'repo_gate', title: 'Repo Gate' },
];

const inputCls =
  'px-2 py-1 bg-white dark:bg-slate-900 rounded border border-slate-200 dark:border-slate-700 text-sm text-slate-900 dark:text-slate-100 placeholder:text-slate-400 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-60 disabled:cursor-not-allowed';

const labelCls =
  'text-xs font-semibold text-slate-500 uppercase tracking-wider';

/**
 * Render a {@link MeasureValue} (a tagged union from the backend) as a plain
 * string. Returns `null` for the absent/`"None"` cases so callers can fall back
 * to a "no run yet" label.
 */
function formatMeasureValue(value: MeasureValue | undefined): string | null {
  if (value === undefined || value === 'None') return null;
  if ('Int' in value) return value.Int.toString();
  if ('Float' in value) return String(value.Float);
  if ('String' in value) return value.String;
  return null;
}

export interface MetricInfoTooltipProps {
  /** The metric to document. */
  metric: MetricKey;
  /** The tooltip catalog; the entry whose `key === metric` is shown. */
  metricInfo?: MetricInfo[];
  /** Latest snapshot for the current value; pure display, never recomputed. */
  currentValues?: ProjectMetricSnapshot;
  /** Extra classes for the trigger button (e.g. spacing in the custom-rules UI). */
  className?: string;
}

/**
 * A circled-"!" indicator (D7 / PRD §7.1, §11.1) that, on hover/focus, reveals a
 * metric's display name, description, example and its CURRENT project value from
 * the passed snapshot ("as of <ranAt>" or "no run yet"). It is pure display:
 * it reads only the props and never triggers a metric recompute. Reusable by the
 * gate-condition rows here and by the custom-rules section.
 */
export function MetricInfoTooltip({
  metric,
  metricInfo,
  currentValues,
  className,
}: Readonly<MetricInfoTooltipProps>) {
  const { t } = useTranslation('quality');
  const info = metricInfo?.find((m) => m.key === metric);

  // Nothing to document ⇒ render nothing (keeps the row clean for stale metrics).
  if (!info) return null;

  const current = formatMeasureValue(currentValues?.values?.[metric]);
  const ranAt = currentValues?.ranAt ?? null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={t(
            'rulesEditor.metricInfo.indicatorLabel',
            'Metric details'
          )}
          className="inline-flex shrink-0 items-center justify-center text-slate-400 hover:text-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 rounded-full"
        >
          <Info className="w-4 h-4" />
        </button>
      </TooltipTrigger>
      <TooltipContent
        side="top"
        className={cn(
          'max-w-xs bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-200 shadow-md px-3 py-2',
          className
        )}
      >
        <div className="flex flex-col gap-1.5 text-xs">
          <div className="font-semibold text-slate-900 dark:text-slate-100">
            {info.displayName}
          </div>
          <p className="text-slate-600 dark:text-slate-300">
            {info.description}
          </p>
          <p className="text-slate-500 dark:text-slate-400">
            <span className="font-semibold">
              {t('rulesEditor.metricInfo.example', 'Example')}:
            </span>{' '}
            {info.example}
          </p>
          <div className="pt-1 mt-1 border-t border-slate-200 dark:border-slate-700">
            {current === null ? (
              <span className="text-slate-400 italic">
                {t('rulesEditor.metricInfo.noRun', 'No run yet')}
              </span>
            ) : (
              <span className="text-slate-700 dark:text-slate-200">
                <span className="font-semibold">
                  {t('rulesEditor.metricInfo.current', 'Current')}:
                </span>{' '}
                <span className="font-mono">{current}</span>
                {ranAt && (
                  <span className="text-slate-400">
                    {' '}
                    {t('rulesEditor.metricInfo.asOf', 'as of {{ranAt}}', {
                      ranAt,
                    })}
                  </span>
                )}
              </span>
            )}
          </div>
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

export function QualityGateRulesEditor({
  value,
  defaults,
  metricOptions,
  onChange,
  readOnly = false,
  errors = [],
  metricInfo,
  currentValues,
  projectId,
}: Readonly<QualityGateRulesEditorProps>) {
  const { t } = useTranslation('quality');

  // The gate currently targeted by the AI authoring dialog (null = closed).
  const [authoringGate, setAuthoringGate] = useState<GateKey | null>(null);

  // ---- pure helpers: build the next config and bubble it up via onChange ----
  const patch = (partial: Partial<QualityGateConfig>) => {
    onChange({ ...value, ...partial });
  };

  const patchGate = (key: GateKey, next: GateDefinition) => {
    patch({ [key]: next } as Partial<QualityGateConfig>);
  };

  const addCondition = (key: GateKey) => {
    const gate = value[key];
    // Default the metric to the first available picker option so it never holds a free-text value.
    const metric = (metricOptions[0] ?? '') as MetricKey;
    const nextCond: ConditionConfig = {
      metric,
      operator: 'GT',
      threshold: '0',
    };
    patchGate(key, { ...gate, conditions: [...gate.conditions, nextCond] });
  };

  const updateCondition = (
    key: GateKey,
    index: number,
    partial: Partial<ConditionConfig>
  ) => {
    const gate = value[key];
    const conditions = gate.conditions.map((c, i) =>
      i === index ? { ...c, ...partial } : c
    );
    patchGate(key, { ...gate, conditions });
  };

  const removeCondition = (key: GateKey, index: number) => {
    const gate = value[key];
    patchGate(key, {
      ...gate,
      conditions: gate.conditions.filter((_, i) => i !== index),
    });
  };

  /**
   * Splice an AI-authored rule's mapped metric into a gate (PRD §11.3, D2).
   * Reuses the add/update semantics: when the gate already carries a condition
   * for `metric`, it is updated in place; otherwise a new `metric > 0` condition
   * is appended via the same helper path the manual editor uses. A rule that
   * maps to no gate metric (`mappedMetric` undefined) splices nothing.
   */
  // Plain (non-memoized) handler: it reads `value`/`authoringGate` and reuses the
  // same add/update helpers the manual editor uses, so it must NOT be frozen by
  // a stale closure — a fresh function per render keeps it correct.
  const handleAuthoringConfirmed = (
    _draft: CustomRuleDraft,
    mappedMetric?: MetricKey
  ) => {
    const key = authoringGate;
    setAuthoringGate(null);
    if (!key || !mappedMetric) return;

    const gate = value[key];
    const existingIndex = gate.conditions.findIndex(
      (c) => c.metric === mappedMetric
    );
    if (existingIndex >= 0) {
      updateCondition(key, existingIndex, { metric: mappedMetric });
      return;
    }
    const nextCond: ConditionConfig = {
      metric: mappedMetric,
      operator: 'GT',
      threshold: '0',
    };
    patchGate(key, { ...gate, conditions: [...gate.conditions, nextCond] });
  };

  const patchProviders = (partial: Partial<ProvidersConfig>) => {
    patch({ providers: { ...value.providers, ...partial } });
  };

  const patchSonar = (partial: Partial<SonarConfig>) => {
    patch({ sonar: { ...value.sonar, ...partial } });
  };

  return (
    <TooltipProvider>
      <div className="flex flex-col gap-6 text-slate-900 dark:text-slate-100">
        {/* Validation errors */}
        {errors.length > 0 && (
          <div className="p-3 bg-red-50 dark:bg-red-950/20 border border-red-100 dark:border-red-900/30 rounded-md">
            <div className="flex items-center gap-2 text-sm font-semibold text-red-700 dark:text-red-400 mb-1">
              <AlertTriangle className="w-4 h-4" />
              <span>Invalid quality policy</span>
            </div>
            <ul className="list-disc pl-6 text-xs text-red-600 dark:text-red-400 space-y-0.5">
              {errors.map((err) => (
                <li key={err}>{err}</li>
              ))}
            </ul>
          </div>
        )}

        {/* Mode */}
        <section className="flex flex-col gap-2">
          <span className={labelCls}>Mode</span>
          <select
            className={cn(inputCls, 'w-48')}
            value={value.mode}
            disabled={readOnly}
            onChange={(e) => patch({ mode: e.target.value as QualityGateMode })}
          >
            {MODES.map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
        </section>

        {/* Gate sections */}
        {GATE_SECTIONS.map(({ key, title }) => {
          const gate = value[key];
          return (
            <section
              key={key}
              className="border border-slate-200 dark:border-slate-800 rounded-lg p-4 bg-slate-50 dark:bg-slate-900/40"
            >
              <div className="flex items-center justify-between mb-3">
                <h4 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                  {title}
                </h4>
                <div className="flex items-center gap-2">
                  {projectId && (
                    <Button
                      type="button"
                      variant="outline"
                      size="xs"
                      disabled={readOnly}
                      onClick={() => setAuthoringGate(key)}
                    >
                      <Sparkles className="w-3.5 h-3.5 mr-1" />
                      {t('rulesEditor.customRules.generateWithAi', {
                        defaultValue: 'Generate rule with AI',
                      })}
                    </Button>
                  )}
                  <Button
                    type="button"
                    variant="outline"
                    size="xs"
                    disabled={readOnly}
                    onClick={() => addCondition(key)}
                  >
                    <Plus className="w-3.5 h-3.5 mr-1" />
                    Add condition
                  </Button>
                </div>
              </div>

              {gate.conditions.length === 0 ? (
                <p className="text-xs text-slate-400 italic py-2">
                  No conditions.
                </p>
              ) : (
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-left">
                      <th className={cn(labelCls, 'pb-2 pr-2 font-semibold')}>
                        Metric
                      </th>
                      <th
                        className={cn(labelCls, 'pb-2 pr-2 font-semibold w-24')}
                      >
                        Operator
                      </th>
                      <th
                        className={cn(labelCls, 'pb-2 pr-2 font-semibold w-32')}
                      >
                        Threshold
                      </th>
                      <th className="w-10" />
                    </tr>
                  </thead>
                  <tbody>
                    {gate.conditions.map((cond, index) => (
                      <tr
                        key={`${key}-${index}-${cond.metric}-${cond.operator}`}
                        className="border-t border-slate-200 dark:border-slate-800"
                      >
                        <td className="py-2 pr-2">
                          <div className="flex items-center gap-1.5">
                            <select
                              className={cn(inputCls, 'w-full')}
                              value={cond.metric}
                              disabled={readOnly}
                              onChange={(e) =>
                                updateCondition(key, index, {
                                  metric: e.target.value as MetricKey,
                                })
                              }
                            >
                              {/* Options come ONLY from metricOptions ⇒ enforces the closed enum; no free text. */}
                              {metricOptions.map((m) => (
                                <option key={m} value={m}>
                                  {m}
                                </option>
                              ))}
                              {/* Surface a stale/out-of-range metric so the select still shows it. */}
                              {!metricOptions.includes(cond.metric) &&
                                cond.metric && (
                                  <option value={cond.metric}>
                                    {cond.metric} (unknown)
                                  </option>
                                )}
                            </select>
                            <MetricInfoTooltip
                              metric={cond.metric}
                              metricInfo={metricInfo}
                              currentValues={currentValues}
                            />
                          </div>
                        </td>
                        <td className="py-2 pr-2">
                          <select
                            className={cn(inputCls, 'w-full')}
                            value={cond.operator}
                            disabled={readOnly}
                            onChange={(e) =>
                              updateCondition(key, index, {
                                operator: e.target.value,
                              })
                            }
                          >
                            {OPERATORS.map((op) => (
                              <option key={op} value={op}>
                                {op}
                              </option>
                            ))}
                          </select>
                        </td>
                        <td className="py-2 pr-2">
                          <input
                            type="text"
                            inputMode="decimal"
                            className={cn(inputCls, 'w-full')}
                            value={cond.threshold}
                            disabled={readOnly}
                            onChange={(e) =>
                              updateCondition(key, index, {
                                threshold: e.target.value,
                              })
                            }
                          />
                        </td>
                        <td className="py-2 text-right">
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            className="w-7 h-7 text-slate-400 hover:text-red-500"
                            disabled={readOnly}
                            aria-label="Delete condition"
                            onClick={() => removeCondition(key, index)}
                          >
                            <Trash2 className="w-4 h-4" />
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </section>
          );
        })}

        {/* Providers — exactly the 11 ProvidersConfig toggles */}
        <section className="flex flex-col gap-2">
          <span className={labelCls}>Providers</span>
          <div className="grid grid-cols-2 md:grid-cols-3 gap-2">
            {PROVIDER_KEYS.map((pk) => (
              <label
                key={pk}
                className={cn(
                  'flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 select-none',
                  readOnly ? 'cursor-not-allowed' : 'cursor-pointer'
                )}
              >
                <input
                  type="checkbox"
                  className="w-4 h-4 rounded border-slate-300 dark:border-slate-600 text-blue-600 focus:ring-blue-500 disabled:opacity-60"
                  checked={value.providers[pk]}
                  disabled={readOnly}
                  onChange={(e) =>
                    patchProviders({
                      [pk]: e.target.checked,
                    } as Partial<ProvidersConfig>)
                  }
                />
                <span className="font-mono">{pk}</span>
              </label>
            ))}
          </div>
        </section>

        {/* SonarQube */}
        <section className="flex flex-col gap-3 border border-slate-200 dark:border-slate-800 rounded-lg p-4">
          <span className={labelCls}>SonarQube</span>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <label className="flex flex-col gap-1">
              <span className="text-xs text-slate-500">Host URL</span>
              <input
                type="text"
                className={inputCls}
                placeholder="https://sonar.example.com"
                value={value.sonar.host_url}
                disabled={readOnly}
                onChange={(e) => patchSonar({ host_url: e.target.value })}
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-xs text-slate-500">Project Key</span>
              <input
                type="text"
                className={inputCls}
                placeholder="my-project"
                value={value.sonar.project_key}
                disabled={readOnly}
                onChange={(e) => patchSonar({ project_key: e.target.value })}
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-xs text-slate-500">Token</span>
              <input
                type="password"
                autoComplete="off"
                className={inputCls}
                placeholder="••••••••"
                value={value.sonar.token ?? ''}
                disabled={readOnly}
                onChange={(e) =>
                  patchSonar({
                    token: e.target.value === '' ? null : e.target.value,
                  })
                }
              />
            </label>
          </div>
        </section>

        {/* Custom rules (AI-editable) — only when a project is in scope (D2/D4). */}
        {projectId && (
          <CustomRulesSection
            projectId={projectId}
            metricInfo={metricInfo}
            currentValues={currentValues}
            readOnly={readOnly}
          />
        )}

        {/* defaults is accepted per the shared editor contract (parents use it for reset affordances). */}
        <span
          className="sr-only"
          aria-hidden="true"
          data-defaults-mode={defaults.mode}
        />
      </div>

      {/* AI authoring dialog — persists the candidate, then splices its mapped
          metric into the targeted gate via {@link handleAuthoringConfirmed}. */}
      {projectId && (
        <RuleAuthoringDialog
          open={authoringGate !== null}
          onClose={() => setAuthoringGate(null)}
          projectId={projectId}
          gateKey={authoringGate ?? undefined}
          currentConditions={
            authoringGate ? value[authoringGate].conditions : undefined
          }
          onConfirmed={handleAuthoringConfirmed}
        />
      )}
    </TooltipProvider>
  );
}

/** Status → StatusPill-style slate tone for the custom-rule status badge. */
const STATUS_BADGE_CLASS: Record<string, string> = {
  draft:
    'bg-slate-100 text-slate-600 border-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:border-slate-700',
  shadow:
    'bg-blue-50 text-blue-700 border-blue-200 dark:bg-blue-950/30 dark:text-blue-300 dark:border-blue-900/40',
  warn: 'bg-amber-50 text-amber-700 border-amber-200 dark:bg-amber-950/30 dark:text-amber-300 dark:border-amber-900/40',
  enforce:
    'bg-red-50 text-red-700 border-red-200 dark:bg-red-950/30 dark:text-red-300 dark:border-red-900/40',
  disabled:
    'bg-slate-100 text-slate-400 border-slate-200 dark:bg-slate-800 dark:text-slate-500 dark:border-slate-700',
};

/** The next status in the shadow → warn → enforce promotion ladder (never auto). */
const PROMOTION_NEXT: Record<string, string | undefined> = {
  shadow: 'warn',
  warn: 'enforce',
};

/**
 * The per-project custom-rules manager (PRD §11.3, D2). Lists each rule with its
 * description "!" tooltip, status badge and the lifecycle controls (enable /
 * disable, manual promotion shadow→warn→enforce, edit, revalidate, delete).
 * Promotion is always explicit — a rule is NEVER auto-enforced.
 */
function CustomRulesSection({
  projectId,
  metricInfo,
  currentValues,
  readOnly,
}: Readonly<{
  projectId: string;
  metricInfo?: MetricInfo[];
  currentValues?: ProjectMetricSnapshot;
  readOnly: boolean;
}>) {
  const { t } = useTranslation('quality');
  const rulesQuery = useCustomRules(projectId);
  const setStatus = useSetCustomRuleStatus();
  const remove = useDeleteCustomRule();

  // The dialog edits a rule's body/metadata (D8) AND hosts the revalidate
  // action (both need the AI model picker), so the row "Revalidate" button just
  // opens it in 'revalidate' mode.
  const [editing, setEditing] = useState<{
    rule: CustomRule;
    mode: 'edit' | 'revalidate';
  } | null>(null);

  const busy = readOnly || setStatus.isPending || remove.isPending;
  const rules = rulesQuery.data ?? [];

  const handleToggleEnabled = useCallback(
    (rule: CustomRule) => {
      // Disable = park at status 'disabled'; re-enable returns it to 'shadow'
      // (never straight to an enforcing status — promotion stays manual).
      const next = rule.status === 'disabled' ? 'shadow' : 'disabled';
      setStatus.mutate({ projectId, ruleId: rule.id, status: next });
    },
    [projectId, setStatus]
  );

  const handlePromote = useCallback(
    (rule: CustomRule) => {
      const next = PROMOTION_NEXT[rule.status];
      if (!next) return;
      setStatus.mutate({ projectId, ruleId: rule.id, status: next });
    },
    [projectId, setStatus]
  );

  const handleDelete = useCallback(
    (rule: CustomRule) => {
      remove.mutate({ projectId, ruleId: rule.id });
    },
    [projectId, remove]
  );

  return (
    <section className="flex flex-col gap-3 border border-slate-200 dark:border-slate-800 rounded-lg p-4">
      <div className="flex items-center justify-between">
        <span className={labelCls}>
          {t('rulesEditor.customRules.title', { defaultValue: 'Custom Rules' })}
        </span>
        {rulesQuery.isFetching && (
          <RefreshCw className="w-3.5 h-3.5 animate-spin text-slate-400" />
        )}
      </div>

      {rulesQuery.isError && (
        <p className="text-xs text-red-500">
          {t('rulesEditor.customRules.loadError', {
            defaultValue: 'Failed to load custom rules.',
          })}
        </p>
      )}

      {rules.length === 0 && !rulesQuery.isLoading ? (
        <p className="text-xs text-slate-400 italic py-1">
          {t('rulesEditor.customRules.empty', {
            defaultValue:
              'No custom rules yet. Use "Generate rule with AI" above to author one.',
          })}
        </p>
      ) : (
        <ul className="flex flex-col divide-y divide-slate-200 dark:divide-slate-800">
          {rules.map((rule) => {
            const promoteTo = PROMOTION_NEXT[rule.status];
            const isDisabled = rule.status === 'disabled';
            const rowBusy = busy;
            return (
              <li
                key={rule.id}
                className="flex items-center gap-2 py-2 first:pt-0 last:pb-0"
              >
                <div className="flex min-w-0 flex-1 items-center gap-1.5">
                  <span className="truncate text-sm font-medium text-slate-900 dark:text-slate-100">
                    {rule.name}
                  </span>
                  {rule.mappedMetric ? (
                    <MetricInfoTooltip
                      metric={rule.mappedMetric as MetricKey}
                      metricInfo={metricInfo}
                      currentValues={currentValues}
                    />
                  ) : (
                    rule.description && (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <button
                            type="button"
                            aria-label={t('rulesEditor.customRules.ruleInfo', {
                              defaultValue: 'Rule details',
                            })}
                            className="inline-flex shrink-0 items-center justify-center text-slate-400 hover:text-blue-500 focus:outline-none rounded-full"
                          >
                            <Info className="w-4 h-4" />
                          </button>
                        </TooltipTrigger>
                        <TooltipContent
                          side="top"
                          className="max-w-xs bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-200 shadow-md px-3 py-2 text-xs"
                        >
                          {rule.description}
                        </TooltipContent>
                      </Tooltip>
                    )
                  )}
                </div>

                <span
                  className={cn(
                    'inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium capitalize',
                    STATUS_BADGE_CLASS[rule.status] ?? STATUS_BADGE_CLASS.draft
                  )}
                >
                  {rule.status}
                </span>

                <div className="flex items-center gap-1">
                  {/* Enable/disable toggle (re-enable lands at shadow). */}
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    disabled={rowBusy}
                    onClick={() => handleToggleEnabled(rule)}
                  >
                    {isDisabled
                      ? t('rulesEditor.customRules.enable', {
                          defaultValue: 'Enable',
                        })
                      : t('rulesEditor.customRules.disable', {
                          defaultValue: 'Disable',
                        })}
                  </Button>

                  {/* Manual promotion shadow→warn→enforce (never automatic). */}
                  {promoteTo && !isDisabled && (
                    <Button
                      type="button"
                      variant="outline"
                      size="xs"
                      disabled={rowBusy}
                      onClick={() => handlePromote(rule)}
                      title={t('rulesEditor.customRules.promoteTitle', {
                        defaultValue: 'Promote to {{status}}',
                        status: promoteTo,
                      })}
                    >
                      {t('rulesEditor.customRules.promoteTo', {
                        defaultValue: '→ {{status}}',
                        status: promoteTo,
                      })}
                    </Button>
                  )}

                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="w-7 h-7 text-slate-400 hover:text-blue-500"
                    disabled={rowBusy}
                    aria-label={t('rulesEditor.customRules.revalidate', {
                      defaultValue: 'Revalidate',
                    })}
                    title={t('rulesEditor.customRules.revalidate', {
                      defaultValue: 'Revalidate',
                    })}
                    onClick={() => setEditing({ rule, mode: 'revalidate' })}
                  >
                    <RefreshCw className="w-4 h-4" />
                  </Button>

                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="w-7 h-7 text-slate-400 hover:text-blue-500"
                    disabled={rowBusy}
                    aria-label={t('rulesEditor.customRules.edit', {
                      defaultValue: 'Edit',
                    })}
                    title={t('rulesEditor.customRules.edit', {
                      defaultValue: 'Edit',
                    })}
                    onClick={() => setEditing({ rule, mode: 'edit' })}
                  >
                    <Pencil className="w-4 h-4" />
                  </Button>

                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="w-7 h-7 text-slate-400 hover:text-red-500"
                    disabled={rowBusy}
                    aria-label={t('rulesEditor.customRules.delete', {
                      defaultValue: 'Delete',
                    })}
                    title={t('rulesEditor.customRules.delete', {
                      defaultValue: 'Delete',
                    })}
                    onClick={() => handleDelete(rule)}
                  >
                    <Trash2 className="w-4 h-4" />
                  </Button>
                </div>
              </li>
            );
          })}
        </ul>
      )}

      {(setStatus.isError || remove.isError) && (
        <p className="text-xs text-red-500">
          {t('rulesEditor.customRules.actionError', {
            defaultValue: 'The last action failed. Please retry.',
          })}
        </p>
      )}

      {/* D8 edit dialog: a body change revalidates (drops to shadow); a
          name/description-only change does not. Also hosts the revalidate flow. */}
      {editing && (
        <CustomRuleEditDialog
          open
          projectId={projectId}
          rule={editing.rule}
          initialMode={editing.mode}
          onClose={() => setEditing(null)}
        />
      )}
    </section>
  );
}
