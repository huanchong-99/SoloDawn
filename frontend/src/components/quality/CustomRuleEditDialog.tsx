import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CheckIcon, CircleNotchIcon } from '@phosphor-icons/react';

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
import { ToolbarDropdown } from '@/components/ui-new/primitives/Toolbar';
import { DropdownMenuItem } from '@/components/ui-new/primitives/Dropdown';
import { cn } from '@/lib/utils';
import {
  useUpdateCustomRule,
  useRevalidateRule,
} from '@/hooks/useQualityPolicy';
import {
  useModelConfigForExecutor,
  type ModelOption,
} from '@/hooks/useModelConfigForExecutor';
import { useUserSystem } from '@/components/ConfigProvider';
import { BaseCodingAgent } from 'shared/types';
import type { ModelConfig as WorkflowModelConfig } from '@/components/workflow/types';
import type {
  CustomRule,
  CustomRuleInput,
  AuthorRuleRequest,
} from 'shared/types';

/** Mirrors RuleAuthoringDialog: the authoring/revalidation transport is Claude. */
const AUTHORING_EXECUTOR = BaseCodingAgent.CLAUDE_CODE;
const AUTHORING_CLI_TYPE_ID = 'cli-claude-code';

const SEVERITIES = ['INFO', 'MINOR', 'MAJOR', 'CRITICAL', 'BLOCKER'] as const;
const RULE_TYPES = [
  'Bug',
  'Vulnerability',
  'CodeSmell',
  'SecurityHotspot',
] as const;

export interface CustomRuleEditDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly projectId: string;
  readonly rule: CustomRule;
  /**
   * `edit` shows the full editable form; `revalidate` jumps straight to the
   * "re-run the AI pipeline" affordance (the row's Revalidate button). Both
   * paths share the model picker the revalidation backend requires.
   */
  readonly initialMode?: 'edit' | 'revalidate';
}

/** Local mutable form state seeded from a {@link CustomRule}. @internal */
export interface FormState {
  name: string;
  description: string;
  severity: string;
  ruleType: string;
  mappedMetric: string;
  ruleBody: string;
}

function seedForm(rule: CustomRule): FormState {
  return {
    name: rule.name,
    description: rule.description ?? '',
    severity: rule.severity,
    ruleType: rule.ruleType,
    mappedMetric: rule.mappedMetric ?? '',
    ruleBody: rule.ruleBody,
  };
}

/**
 * D8 fields whose change RE-RUNS the admission gate + drops the rule to shadow
 * (must mirror `body_changed` in crates/server/src/routes/custom_rules.rs:
 * rule_body / rule_format / severity / rule_type / mapped_metric). A change to
 * only name/description is metadata-only and does NOT revalidate.
 *
 * @internal Exported for unit tests of the D8 edit policy.
 */
export function isBodyChange(rule: CustomRule, form: FormState): boolean {
  return (
    form.ruleBody !== rule.ruleBody ||
    form.severity !== rule.severity ||
    form.ruleType !== rule.ruleType ||
    (form.mappedMetric || null) !== (rule.mappedMetric ?? null)
  );
}

/** Build the PUT payload. `message` is not persisted on update; default it. */
function formToInput(rule: CustomRule, form: FormState): CustomRuleInput {
  return {
    nlRequest: rule.nlRequest,
    ruleFormat: rule.ruleFormat,
    ruleBody: form.ruleBody,
    name: form.name,
    description: form.description.trim() ? form.description.trim() : null,
    message: form.name,
    ruleType: form.ruleType,
    severity: form.severity,
    languages: [],
    extensions: [],
    includeGlobs: [],
    excludeGlobs: [],
    mappedMetric: form.mappedMetric ? form.mappedMetric : null,
    // A body change re-derives fixtures via the subsequent revalidate; a
    // metadata-only change leaves the stored fixtures untouched (the route only
    // replaces examples on a body change).
    examples: [],
  };
}

const inputCls =
  'px-base py-half bg-secondary rounded border border-border text-base text-normal placeholder:text-low focus:outline-none focus:ring-1 focus:ring-brand disabled:opacity-60 disabled:cursor-not-allowed';
const labelCls = 'text-low text-xs font-semibold uppercase tracking-wider';

/**
 * Edit a custom rule (PRD §11.3, decision D8) and host its revalidate action.
 *
 * - A metadata-only edit (name/description) PUTs once and is done.
 * - A body edit (pattern/severity/type/metric) PUTs the new body — which the
 *   server drops to `shadow` — then re-runs the AI validation pipeline via
 *   `revalidate` so the regenerated fixtures + round-trip verdict are recorded.
 * - `revalidate` mode skips the form edits and just re-runs the pipeline.
 *
 * The revalidation backend requires an explicit model source (no default-billing
 * fallthrough), so the dialog reuses the same model picker as RuleAuthoringDialog.
 */
export function CustomRuleEditDialog({
  open,
  onClose,
  projectId,
  rule,
  initialMode = 'edit',
}: Readonly<CustomRuleEditDialogProps>) {
  const { t } = useTranslation(['settings', 'common', 'quality']);
  const { config } = useUserSystem();

  const workflowModelLibrary = (config as Record<string, unknown> | null)
    ?.workflow_model_library as WorkflowModelConfig[] | undefined;

  const {
    customModels,
    officialModels,
    selectedModelConfigId,
    setSelectedModelConfigId,
  } = useModelConfigForExecutor(AUTHORING_EXECUTOR, workflowModelLibrary);

  const update = useUpdateCustomRule();
  const revalidate = useRevalidateRule();

  const [form, setForm] = useState<FormState>(() => seedForm(rule));
  const [error, setError] = useState<string | null>(null);

  const revalidateOnly = initialMode === 'revalidate';
  const bodyChange = useMemo(() => isBodyChange(rule, form), [rule, form]);
  const dirty = useMemo(
    () =>
      form.name !== rule.name ||
      form.description !== (rule.description ?? '') ||
      bodyChange,
    [form, rule, bodyChange]
  );

  const submitting = update.isPending || revalidate.isPending;
  // A body edit AND the revalidate-only action both need a chosen model.
  const needsModel = revalidateOnly || bodyChange;

  const selectedModel = useMemo<ModelOption | null>(() => {
    const all = [...customModels, ...officialModels];
    return all.find((m) => m.id === selectedModelConfigId) ?? null;
  }, [customModels, officialModels, selectedModelConfigId]);

  const pickerLabel =
    selectedModel?.displayName ??
    t('settings:ruleAuthoring.selectModel', { defaultValue: 'Select model' });

  const patch = useCallback((partial: Partial<FormState>) => {
    setForm((prev) => ({ ...prev, ...partial }));
  }, []);

  const runRevalidate = useCallback(async () => {
    const req: AuthorRuleRequest = {
      nlRequest: rule.nlRequest,
      modelConfigId: selectedModelConfigId ?? '',
      cliTypeId: AUTHORING_CLI_TYPE_ID,
      ruleFormatPreference: rule.ruleFormat,
      currentRulesContext: null,
    };
    await revalidate.mutateAsync({ projectId, ruleId: rule.id, req });
  }, [rule, selectedModelConfigId, revalidate, projectId]);

  const handleSubmit = useCallback(async () => {
    setError(null);
    if (needsModel && !selectedModelConfigId) {
      setError(
        t('settings:ruleAuthoring.selectModel', {
          defaultValue: 'Select model',
        })
      );
      return;
    }

    try {
      if (revalidateOnly) {
        await runRevalidate();
        onClose();
        return;
      }

      // Persist the edit. The server drops a body change to shadow and records a
      // gate-pass validation; a metadata-only change just bumps the version.
      await update.mutateAsync({
        projectId,
        ruleId: rule.id,
        input: formToInput(rule, form),
      });

      // D8: a body change additionally re-runs the AI validation pipeline on the
      // now-stored body so the regenerated fixtures + round-trip are recorded.
      if (bodyChange) {
        await runRevalidate();
      }
      onClose();
    } catch (err: unknown) {
      setError(
        err instanceof Error
          ? err.message
          : t('quality:rulesEditor.customRules.saveError', {
              defaultValue: 'Failed to save the rule.',
            })
      );
    }
  }, [
    needsModel,
    selectedModelConfigId,
    revalidateOnly,
    runRevalidate,
    update,
    projectId,
    rule,
    form,
    bodyChange,
    onClose,
    t,
  ]);

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next && !submitting) onClose();
    },
    [submitting, onClose]
  );

  const primaryDisabled =
    submitting ||
    (needsModel && !selectedModelConfigId) ||
    (!revalidateOnly && !dirty);

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {revalidateOnly
              ? t('quality:rulesEditor.customRules.revalidateTitle', {
                  defaultValue: 'Revalidate rule',
                })
              : t('quality:rulesEditor.customRules.editTitle', {
                  defaultValue: 'Edit rule',
                })}
          </DialogTitle>
          <DialogDescription>
            {revalidateOnly
              ? t('quality:rulesEditor.customRules.revalidateDescription', {
                  defaultValue:
                    'Re-run the AI validation pipeline. The rule drops to ' +
                    'shadow until it passes again.',
                })
              : t('quality:rulesEditor.customRules.editDescription', {
                  defaultValue:
                    'Editing the rule body re-runs validation and drops the ' +
                    'rule to shadow; editing only the name or description does not.',
                })}
          </DialogDescription>
        </DialogHeader>

        <div className="max-h-[60vh] overflow-y-auto py-base flex flex-col gap-base">
          {error && <ErrorAlert message={error} />}

          {!revalidateOnly && (
            <>
              <label className="flex flex-col gap-half">
                <span className={labelCls}>
                  {t('quality:rulesEditor.customRules.fieldName', {
                    defaultValue: 'Name',
                  })}
                </span>
                <input
                  type="text"
                  className={inputCls}
                  value={form.name}
                  disabled={submitting}
                  onChange={(e) => patch({ name: e.target.value })}
                />
              </label>

              <label className="flex flex-col gap-half">
                <span className={labelCls}>
                  {t('quality:rulesEditor.customRules.fieldDescription', {
                    defaultValue: 'Description',
                  })}
                </span>
                <textarea
                  className={cn(inputCls, 'min-h-[3.5rem] resize-y')}
                  value={form.description}
                  disabled={submitting}
                  onChange={(e) => patch({ description: e.target.value })}
                />
              </label>

              <div className="grid grid-cols-1 md:grid-cols-3 gap-base">
                <label className="flex flex-col gap-half">
                  <span className={labelCls}>
                    {t('quality:rulesEditor.customRules.fieldSeverity', {
                      defaultValue: 'Severity',
                    })}
                  </span>
                  <select
                    className={inputCls}
                    value={form.severity}
                    disabled={submitting}
                    onChange={(e) => patch({ severity: e.target.value })}
                  >
                    {SEVERITIES.map((s) => (
                      <option key={s} value={s}>
                        {s}
                      </option>
                    ))}
                  </select>
                </label>

                <label className="flex flex-col gap-half">
                  <span className={labelCls}>
                    {t('quality:rulesEditor.customRules.fieldType', {
                      defaultValue: 'Type',
                    })}
                  </span>
                  <select
                    className={inputCls}
                    value={form.ruleType}
                    disabled={submitting}
                    onChange={(e) => patch({ ruleType: e.target.value })}
                  >
                    {RULE_TYPES.map((rt) => (
                      <option key={rt} value={rt}>
                        {rt}
                      </option>
                    ))}
                  </select>
                </label>

                <label className="flex flex-col gap-half">
                  <span className={labelCls}>
                    {t('quality:rulesEditor.customRules.fieldMetric', {
                      defaultValue: 'Mapped metric',
                    })}
                  </span>
                  <input
                    type="text"
                    className={inputCls}
                    placeholder={t(
                      'quality:rulesEditor.customRules.fieldMetricPlaceholder',
                      { defaultValue: 'none' }
                    )}
                    value={form.mappedMetric}
                    disabled={submitting}
                    onChange={(e) => patch({ mappedMetric: e.target.value })}
                  />
                </label>
              </div>

              <label className="flex flex-col gap-half">
                <span className={labelCls}>
                  {t('quality:rulesEditor.customRules.fieldBody', {
                    defaultValue: 'Rule body (regex)',
                  })}
                </span>
                <textarea
                  className={cn(
                    inputCls,
                    'min-h-[5rem] resize-y font-ibm-plex-mono'
                  )}
                  value={form.ruleBody}
                  disabled={submitting}
                  onChange={(e) => patch({ ruleBody: e.target.value })}
                />
              </label>
            </>
          )}

          {/* Model picker — shown whenever the action will re-run validation. */}
          {needsModel && (
            <div className="flex flex-col gap-half">
              <span className={labelCls}>
                {t('settings:ruleAuthoring.modelLabel', {
                  defaultValue: 'Authoring model',
                })}
              </span>
              <ToolbarDropdown label={pickerLabel}>
                {[...customModels, ...officialModels].map((model) => (
                  <DropdownMenuItem
                    key={model.id}
                    icon={
                      selectedModelConfigId === model.id ? CheckIcon : undefined
                    }
                    onClick={() => setSelectedModelConfigId(model.id)}
                  >
                    <span className="flex flex-col">
                      <span>{model.displayName}</span>
                      {model.subtitle && (
                        <span className="text-low text-xs">
                          {model.subtitle}
                        </span>
                      )}
                    </span>
                  </DropdownMenuItem>
                ))}
                {customModels.length === 0 && officialModels.length === 0 && (
                  <DropdownMenuItem disabled>
                    {t('settings:ruleAuthoring.noModels', {
                      defaultValue: 'No usable models configured',
                    })}
                  </DropdownMenuItem>
                )}
              </ToolbarDropdown>
              {bodyChange && !revalidateOnly && (
                <span className="text-low text-xs">
                  {t('quality:rulesEditor.customRules.bodyChangeHint', {
                    defaultValue:
                      'Body changed — saving re-runs validation and drops the rule to shadow.',
                  })}
                </span>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            variant="ghost"
            size="sm"
            onClick={onClose}
            disabled={submitting}
          >
            {t('common:buttons.cancel', { defaultValue: 'Cancel' })}
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={handleSubmit}
            disabled={primaryDisabled}
          >
            {submitting && (
              <CircleNotchIcon
                className="size-icon-xs mr-half animate-spin"
                weight="bold"
              />
            )}
            {revalidateOnly
              ? t('quality:rulesEditor.customRules.revalidateAction', {
                  defaultValue: 'Revalidate',
                })
              : t('common:buttons.save', { defaultValue: 'Save' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default CustomRuleEditDialog;
