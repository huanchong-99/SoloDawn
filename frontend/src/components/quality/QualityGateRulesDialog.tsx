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
import { cn } from '@/lib/utils';
import {
  useProjectQualityPolicy,
  useDefaultQualityPolicy,
  useQualityMetricKeys,
  useSaveQualityPolicy,
  useResetQualityPolicy,
} from '@/hooks/useQualityPolicy';
import type { QualityGateConfig } from 'shared/types';

interface QualityGateRulesDialogProps {
  /** Whether the dialog is open. */
  readonly open: boolean;
  /** The project whose quality policy is resolved/edited. */
  readonly projectId: string;
  /** Close the dialog. */
  readonly onClose: () => void;
}

/* ------------------------------------------------------------------ */
/*  Source badge (project | file | bundled)                           */
/* ------------------------------------------------------------------ */
function SourceBadge({ source }: Readonly<{ source: string }>) {
  const { t } = useTranslation('tasks');

  const styles: Record<string, string> = {
    project: 'border-brand text-brand bg-brand/10',
    file: 'border-success text-success bg-success/10',
    bundled: 'border-border text-low bg-secondary',
  };

  const labels: Record<string, string> = {
    project: t('conversation.createLanding.qualityGates.sourceProject'),
    file: t('conversation.createLanding.qualityGates.sourceFile'),
    bundled: t('conversation.createLanding.qualityGates.sourceBundled'),
  };

  return (
    <span
      className={cn(
        'inline-flex items-center rounded border px-2 py-0.5 text-xs font-medium',
        styles[source] ?? styles.bundled
      )}
    >
      {labels[source] ?? source}
    </span>
  );
}

/**
 * Standalone manage-mode quality-gate rules editor dialog.
 *
 * Decoupled from the draft/materialize flow: it lets the user review and edit
 * the per-project quality-gate rules (add/edit/delete gate conditions), SAVE
 * (PUT) and reset-to-default (DELETE) for the SELECTED project, WITHOUT starting
 * a task. The G2 QualityGateConfirmDialog still governs the materialize gate.
 */
export function QualityGateRulesDialog({
  open,
  projectId,
  onClose,
}: QualityGateRulesDialogProps) {
  const { t } = useTranslation('tasks');

  const policyQuery = useProjectQualityPolicy(open ? projectId : null);
  const defaultQuery = useDefaultQualityPolicy();
  const metricsQuery = useQualityMetricKeys();
  const saveMutation = useSaveQualityPolicy();
  const resetMutation = useResetQualityPolicy();

  const [config, setConfig] = useState<QualityGateConfig | null>(null);
  const [original, setOriginal] = useState<QualityGateConfig | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const serverConfig = policyQuery.data?.config;
  const source = policyQuery.data?.source ?? 'bundled';
  const defaults = defaultQuery.data?.config;
  const metricOptions = metricsQuery.data?.metrics ?? [];

  // Sync local editor state to the server config when (re)loaded and not dirty.
  useEffect(() => {
    if (serverConfig && (!original || isEqual(original, serverConfig))) {
      setConfig(serverConfig);
      setOriginal(serverConfig);
    }
  }, [serverConfig, original]);

  // Reset local editor state when the dialog is reopened or project changes.
  useEffect(() => {
    if (!open) {
      setConfig(null);
      setOriginal(null);
      setActionError(null);
    }
  }, [open, projectId]);

  const dirty = useMemo(
    () => !!config && !!original && !isEqual(config, original),
    [config, original]
  );

  const pending = saveMutation.isPending || resetMutation.isPending;

  const loading =
    policyQuery.isLoading || defaultQuery.isLoading || metricsQuery.isLoading;

  const loadError =
    policyQuery.error ?? defaultQuery.error ?? metricsQuery.error ?? null;

  const handleSave = useCallback(async () => {
    if (!config) return;
    setActionError(null);
    try {
      const result = await saveMutation.mutateAsync({ projectId, config });
      setConfig(result.config);
      setOriginal(result.config);
      onClose();
    } catch (err: unknown) {
      setActionError(
        err instanceof Error
          ? err.message
          : t('conversation.createLanding.qualityGates.saveError')
      );
    }
  }, [config, projectId, saveMutation, onClose, t]);

  const handleReset = useCallback(async () => {
    setActionError(null);
    try {
      await resetMutation.mutateAsync(projectId);
      setConfig(null);
      setOriginal(null);
      await policyQuery.refetch();
    } catch (err: unknown) {
      setActionError(
        err instanceof Error
          ? err.message
          : t('conversation.createLanding.qualityGates.resetError')
      );
    }
  }, [projectId, resetMutation, policyQuery, t]);

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next && !pending) onClose();
    },
    [pending, onClose]
  );

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <div className="flex items-center justify-between gap-base pr-8">
            <DialogTitle>
              {t('conversation.createLanding.qualityGates.title')}
            </DialogTitle>
            {!policyQuery.isLoading && <SourceBadge source={source} />}
          </div>
          <DialogDescription>
            {t('conversation.createLanding.qualityGates.description')}
          </DialogDescription>
        </DialogHeader>

        <div className="max-h-[60vh] overflow-y-auto py-base">
          {actionError && <ErrorAlert message={actionError} className="mb-base" />}
          {loadError && (
            <ErrorAlert
              message={
                loadError instanceof Error
                  ? loadError.message
                  : t('conversation.createLanding.qualityGates.loadError')
              }
              className="mb-base"
            />
          )}

          {loading || !config || !defaults ? (
            <p className="text-low text-base py-double text-center">
              {t('conversation.createLanding.qualityGates.loading')}
            </p>
          ) : (
            <QualityGateRulesEditor
              value={config}
              defaults={defaults}
              metricOptions={metricOptions}
              onChange={setConfig}
              readOnly={pending}
            />
          )}
        </div>

        <DialogFooter className="sm:justify-between">
          <Button
            variant="destructive"
            size="sm"
            onClick={handleReset}
            disabled={source !== 'project' || pending}
            title={t('conversation.createLanding.qualityGates.resetTitle')}
          >
            {t('conversation.createLanding.qualityGates.reset')}
          </Button>
          <div className="flex gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={onClose}
              disabled={pending}
            >
              {t('conversation.createLanding.qualityGates.cancel')}
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={handleSave}
              disabled={!dirty || pending || !config}
            >
              {t('conversation.createLanding.qualityGates.save')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default QualityGateRulesDialog;
