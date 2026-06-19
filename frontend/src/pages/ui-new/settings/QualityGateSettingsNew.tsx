import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { isEqual } from 'lodash';
import { CaretDownIcon } from '@phosphor-icons/react';

import { cn } from '@/lib/utils';
import { SettingsCard } from '@/components/ui-new/primitives/SettingsCard';
import { Button } from '@/components/ui-new/primitives/Button';
import { Label } from '@/components/ui-new/primitives/Label';
import { ErrorAlert } from '@/components/ui-new/primitives/ErrorAlert';
import { SettingsSaveBar } from '@/components/ui-new/primitives/SettingsSaveBar';
import { QualityGateRulesEditor } from '@/components/quality/QualityGateRulesEditor';
import { useProjects } from '@/hooks/useProjects';
import {
  useProjectQualityPolicy,
  useDefaultQualityPolicy,
  useQualityMetricKeys,
  useSaveQualityPolicy,
  useResetQualityPolicy,
  useProjectMetricsLatest,
} from '@/hooks/useQualityPolicy';
import type { QualityGateConfig } from 'shared/types';

/* ------------------------------------------------------------------ */
/*  Source badge (project | file | bundled)                           */
/* ------------------------------------------------------------------ */
function SourceBadge({ source }: Readonly<{ source: string }>) {
  const { t } = useTranslation('settings');

  const styles: Record<string, string> = {
    project: 'border-brand text-brand bg-brand/10',
    file: 'border-success text-success bg-success/10',
    bundled: 'border-border text-low bg-secondary',
  };

  const labels: Record<string, string> = {
    project: t('qualityGates.source.project', { defaultValue: 'Project override' }),
    file: t('qualityGates.source.file', { defaultValue: 'Repo file' }),
    bundled: t('qualityGates.source.bundled', { defaultValue: 'Bundled default' }),
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
 * G3 standalone quality-gate settings page (spec §3.4).
 *
 * Provides project context (selector), wraps the shared QualityGateRulesEditor,
 * persists via useSaveQualityPolicy (PUT) and resets to the resolved default via
 * useResetQualityPolicy (DELETE), and surfaces the resolved source badge.
 */
export function QualityGateSettingsNew() {
  const { t } = useTranslation(['settings', 'common']);

  const { projects, isLoading: projectsLoading } = useProjects();
  const [projectId, setProjectId] = useState<string>('');

  // Default the selector to the first available project.
  useEffect(() => {
    if (!projectId && projects.length > 0) {
      setProjectId(projects[0].id);
    }
  }, [projects, projectId]);

  const policyQuery = useProjectQualityPolicy(projectId || null);
  const defaultQuery = useDefaultQualityPolicy();
  const metricsQuery = useQualityMetricKeys();
  const metricsLatest = useProjectMetricsLatest(projectId || null);
  const saveMutation = useSaveQualityPolicy();
  const resetMutation = useResetQualityPolicy();

  const [config, setConfig] = useState<QualityGateConfig | null>(null);
  const [original, setOriginal] = useState<QualityGateConfig | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const serverConfig = policyQuery.data?.config;
  const source = policyQuery.data?.source ?? 'bundled';
  const defaults = defaultQuery.data?.config;
  const metricOptions = metricsQuery.data?.metrics ?? [];
  const metricInfo = metricsQuery.data?.info;

  // Sync local editor state to the server config when (re)loaded and not dirty.
  useEffect(() => {
    if (serverConfig && (!original || isEqual(original, serverConfig))) {
      setConfig(serverConfig);
      setOriginal(serverConfig);
    }
    // Re-seed when the selected project changes (server config identity changes).
  }, [serverConfig, original]);

  // Reset local editor state when the project selection changes.
  useEffect(() => {
    setConfig(null);
    setOriginal(null);
    setActionError(null);
  }, [projectId]);

  const dirty = useMemo(
    () => !!config && !!original && !isEqual(config, original),
    [config, original]
  );

  const handleSave = async () => {
    if (!config || !projectId) return;
    setActionError(null);
    try {
      const result = await saveMutation.mutateAsync({ projectId, config });
      setConfig(result.config);
      setOriginal(result.config);
    } catch (err: unknown) {
      setActionError(
        err instanceof Error
          ? err.message
          : t('settings:qualityGates.saveError', {
              defaultValue: 'Failed to save quality policy.',
            })
      );
    }
  };

  const handleDiscard = () => {
    if (original) setConfig(original);
    setActionError(null);
  };

  const handleReset = async () => {
    if (!projectId) return;
    setActionError(null);
    try {
      await resetMutation.mutateAsync(projectId);
      // After deletion the GET resolves to file/bundled default; let the query
      // refetch re-seed the editor via the sync effect.
      setOriginal(null);
      setConfig(null);
      await policyQuery.refetch();
    } catch (err: unknown) {
      setActionError(
        err instanceof Error
          ? err.message
          : t('settings:qualityGates.resetError', {
              defaultValue: 'Failed to reset quality policy.',
            })
      );
    }
  };

  const loading =
    projectsLoading ||
    policyQuery.isLoading ||
    defaultQuery.isLoading ||
    metricsQuery.isLoading;

  const loadError =
    policyQuery.error ?? defaultQuery.error ?? metricsQuery.error ?? null;

  return (
    <div className="space-y-base">
      {actionError && <ErrorAlert message={actionError} />}
      {loadError && (
        <ErrorAlert
          message={
            loadError instanceof Error
              ? loadError.message
              : String(loadError)
          }
        />
      )}

      <SettingsCard
        title={t('settings:qualityGates.title', {
          defaultValue: 'Quality Gates',
        })}
        description={t('settings:qualityGates.description', {
          defaultValue:
            'Configure the per-project quality-gate rules enforced during ' +
            'workflow runs. Saved rules take priority over the repo file and ' +
            'the bundled default.',
        })}
      >
        <div className="space-y-base">
          {/* Project selector + source badge */}
          <div className="flex items-end justify-between gap-base">
            <div className="space-y-half flex-1 max-w-sm">
              <Label htmlFor="qg-project" className="text-normal text-base">
                {t('settings:qualityGates.projectLabel', {
                  defaultValue: 'Project',
                })}
              </Label>
              <div className="relative">
                <select
                  id="qg-project"
                  value={projectId}
                  onChange={(e) => setProjectId(e.target.value)}
                  disabled={projectsLoading || projects.length === 0}
                  className={cn(
                    'w-full appearance-none rounded border border-border bg-secondary px-base py-1 pr-7 text-base text-normal',
                    'focus:outline-none focus:ring-1 focus:ring-brand',
                    (projectsLoading || projects.length === 0) &&
                      'opacity-60 cursor-not-allowed'
                  )}
                >
                  {projects.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
                <CaretDownIcon
                  className="size-icon-xs absolute right-1.5 top-1/2 -translate-y-1/2 text-low pointer-events-none"
                  weight="bold"
                />
              </div>
            </div>
            {!!projectId && !policyQuery.isLoading && (
              <SourceBadge source={source} />
            )}
          </div>

          {(() => {
            if (!projectId) {
              return (
                <p className="text-low text-base py-double text-center">
                  {t('settings:qualityGates.noProject', {
                    defaultValue: 'Select a project to edit its quality gates.',
                  })}
                </p>
              );
            }
            if (loading || !config || !defaults) {
              return (
                <p className="text-low text-base py-double text-center">
                  {t('common:loading', { defaultValue: 'Loading...' })}
                </p>
              );
            }
            return (
              <QualityGateRulesEditor
                value={config}
                defaults={defaults}
                metricOptions={metricOptions}
                metricInfo={metricInfo}
                currentValues={metricsLatest.data}
                projectId={projectId}
                onChange={setConfig}
                readOnly={saveMutation.isPending || resetMutation.isPending}
              />
            );
          })()}

          {/* Reset-to-default action */}
          {!!projectId && !!config && (
            <div className="flex justify-start pt-base border-t border-border">
              <Button
                variant="destructive"
                size="sm"
                onClick={handleReset}
                disabled={
                  source !== 'project' ||
                  resetMutation.isPending ||
                  saveMutation.isPending
                }
                title={t('settings:qualityGates.resetTitle', {
                  defaultValue:
                    'Delete this project override and fall back to the default.',
                })}
              >
                {t('settings:qualityGates.reset', {
                  defaultValue: 'Reset to default',
                })}
              </Button>
            </div>
          )}
        </div>
      </SettingsCard>

      <SettingsSaveBar
        visible={dirty}
        onSave={handleSave}
        onDiscard={handleDiscard}
        saving={saveMutation.isPending}
      />
    </div>
  );
}

export default QualityGateSettingsNew;
