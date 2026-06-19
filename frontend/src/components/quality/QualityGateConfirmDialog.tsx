import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { isEqual } from 'lodash';

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
import { QualityGateRulesEditor } from '@/components/quality/QualityGateRulesEditor';
import { CustomRulesConfirmPanel } from '@/components/quality/CustomRulesConfirmPanel';
import {
  qualityPolicyApi,
  planningDraftsApi,
  type PlanningDraftResponse,
} from '@/lib/api';
import { useProjectMetricsLatest } from '@/hooks/useQualityPolicy';
import type { QualityGateConfig, MetricKey, MetricInfo } from 'shared/types';

interface QualityGateConfirmDialogProps {
  /** Whether the dialog is open. */
  readonly open: boolean;
  /** Close the dialog without confirming (materialize stays blocked). */
  readonly onClose: () => void;
  /** The project whose quality policy is resolved/edited. */
  readonly projectId: string;
  /** The planning draft whose gates are being confirmed. */
  readonly draftId: string;
  /**
   * Called after the gates are confirmed (and any DIY edit persisted). The
   * parent is responsible for refetching the draft and triggering materialize.
   */
  readonly onConfirmed?: (draft: PlanningDraftResponse) => void;
}

/**
 * G2 mandatory quality-gate confirmation popup (spec §3.3).
 *
 * Fetches the effective project policy (DB-first), the bundled default, and the
 * metric catalog, then holds editor state. The primary action "Save & Confirm":
 *   1. if the rules were edited, PUT the project policy via qualityPolicyApi
 *   2. POST /planning-drafts/{id}/confirm-gates (with the edited config if any)
 *   3. invoke onConfirmed so the parent can proceed to materialize
 * Cancel closes without confirming, leaving materialize backend-blocked.
 */
export function QualityGateConfirmDialog({
  open,
  onClose,
  projectId,
  draftId,
  onConfirmed,
}: QualityGateConfirmDialogProps) {
  const { t } = useTranslation(['settings', 'common']);

  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [config, setConfig] = useState<QualityGateConfig | null>(null);
  const [original, setOriginal] = useState<QualityGateConfig | null>(null);
  const [defaults, setDefaults] = useState<QualityGateConfig | null>(null);
  const [metricOptions, setMetricOptions] = useState<MetricKey[]>([]);
  const [metricInfo, setMetricInfo] = useState<MetricInfo[]>([]);

  // Latest persisted snapshot for the metric tooltips (pure display, no recompute).
  const metricsLatest = useProjectMetricsLatest(open ? projectId : null);

  // Load policy + default + metrics whenever the dialog opens for a project.
  useEffect(() => {
    if (!open || !projectId) return;

    let cancelled = false;
    setLoading(true);
    setError(null);

    Promise.all([
      qualityPolicyApi.getProject(projectId),
      qualityPolicyApi.getDefault(),
      qualityPolicyApi.getMetrics(),
    ])
      .then(([project, def, metrics]) => {
        if (cancelled) return;
        setConfig(project.config);
        setOriginal(project.config);
        setDefaults(def.config);
        setMetricOptions(metrics.metrics);
        setMetricInfo(metrics.info);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(
          err instanceof Error
            ? err.message
            : t('settings:qualityGates.loadError', {
                defaultValue: 'Failed to load quality policy.',
              })
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [open, projectId, t]);

  const edited = useMemo(
    () => !!config && !!original && !isEqual(config, original),
    [config, original]
  );

  const handleSaveAndConfirm = useCallback(async () => {
    if (!config) return;

    setSubmitting(true);
    setError(null);

    try {
      // Persist DIY edits as the project policy first (priority-0 source).
      if (edited) {
        await qualityPolicyApi.putProject(projectId, config);
      }
      // Stamp gates_confirmed_at (sending the edited config so a direct
      // backend upsert path stays consistent with the popup).
      const updated = await planningDraftsApi.confirmGates(
        draftId,
        edited ? config : undefined
      );
      onConfirmed?.(updated);
      onClose();
    } catch (err: unknown) {
      setError(
        err instanceof Error
          ? err.message
          : t('settings:qualityGates.confirmError', {
              defaultValue: 'Failed to confirm quality gates.',
            })
      );
    } finally {
      setSubmitting(false);
    }
  }, [config, edited, projectId, draftId, onConfirmed, onClose, t]);

  const handleOpenChange = useCallback(
    (next: boolean) => {
      // Radix fires onOpenChange(false) on overlay click / Esc / close button.
      if (!next && !submitting) onClose();
    },
    [submitting, onClose]
  );

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {t('settings:qualityGates.confirmTitle', {
              defaultValue: 'Confirm Quality Gates',
            })}
          </DialogTitle>
          <DialogDescription>
            {t('settings:qualityGates.confirmDescription', {
              defaultValue:
                'Review the quality-gate rules and AI-authored custom rules ' +
                'enforced for this run. Confirmation is mandatory — ' +
                'materialization cannot begin until you Save & Confirm.',
            })}
          </DialogDescription>
        </DialogHeader>

        <div className="max-h-[60vh] overflow-y-auto py-base">
          {error && <ErrorAlert message={error} className="mb-base" />}

          {loading || !config || !defaults ? (
            <p className="text-low text-base py-double text-center">
              {t('common:loading', { defaultValue: 'Loading...' })}
            </p>
          ) : (
            <div className="flex flex-col gap-base">
              <QualityGateRulesEditor
                value={config}
                defaults={defaults}
                metricOptions={metricOptions}
                metricInfo={metricInfo}
                currentValues={metricsLatest.data}
                projectId={projectId}
                onChange={setConfig}
                readOnly={submitting}
              />
              {/* Read-only evidence for every active AI-authored custom rule. */}
              <CustomRulesConfirmPanel projectId={projectId} />
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
            onClick={handleSaveAndConfirm}
            disabled={loading || submitting || !config}
          >
            {t('settings:qualityGates.saveAndConfirm', {
              defaultValue: 'Save & Confirm',
            })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default QualityGateConfirmDialog;
