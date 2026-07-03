import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ArrowsClockwiseIcon,
  PlusIcon,
  TrashIcon,
  SpinnerGapIcon,
  CheckCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react';

import {
  architectureApi,
  makeRequest,
  handleApiResponse,
  type ArchitectureSourceResponse,
} from '@/lib/api';
import { cn } from '@/lib/utils';
import { SettingsCard } from '@/components/ui-new/primitives/SettingsCard';
import { SettingsSection } from '@/components/ui-new/primitives/SettingsSection';
import { SettingsToggle } from '@/components/ui-new/primitives/SettingsToggle';
import { ErrorAlert } from '@/components/ui-new/primitives/ErrorAlert';
import { PrimaryButton } from '@/components/ui-new/primitives/PrimaryButton';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui-new/primitives/Dialog';

async function fetchSystemSettings(): Promise<Record<string, string>> {
  const response = await makeRequest('/api/system-settings');
  return handleApiResponse<Record<string, string>>(response);
}

async function saveGuidanceEnabled(enabled: boolean): Promise<void> {
  const response = await makeRequest('/api/system-settings', {
    method: 'PUT',
    body: JSON.stringify({ architectureGuidanceEnabled: enabled }),
  });
  await handleApiResponse(response);
}

/** Missing setting means "on": the feature ships enabled by default. */
function parseGuidanceEnabled(raw: string | undefined): boolean {
  if (raw === undefined || raw.trim() === '') return true;
  const v = raw.trim().toLowerCase();
  return v === 'true' || v === '1';
}

interface NewSourceState {
  name: string;
  owner: string;
  repo: string;
  branch: string;
  includePaths: string;
}

const EMPTY_SOURCE: NewSourceState = {
  name: '',
  owner: '',
  repo: '',
  branch: 'main',
  includePaths: 'templates/',
};

export function ArchitectureSettingsNew() {
  const { t, i18n } = useTranslation(['settings', 'common']);

  const [sources, setSources] = useState<ArchitectureSourceResponse[]>([]);
  const [guidanceEnabled, setGuidanceEnabled] = useState(true);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [togglingGuidance, setTogglingGuidance] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [syncingId, setSyncingId] = useState<string | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [addState, setAddState] = useState<NewSourceState>(EMPTY_SOURCE);
  const [addSaving, setAddSaving] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      setLoading(true);
      const [list, settings] = await Promise.all([
        architectureApi.listSources(),
        fetchSystemSettings(),
      ]);
      setSources(list);
      setGuidanceEnabled(
        parseGuidanceEnabled(settings['architecture_guidance_enabled'])
      );
      setError(null);
    } catch {
      setError(t('settings.architecture.loadError'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleToggleGuidance = async (enabled: boolean) => {
    const previous = guidanceEnabled;
    setGuidanceEnabled(enabled);
    try {
      setTogglingGuidance(true);
      await saveGuidanceEnabled(enabled);
    } catch {
      setGuidanceEnabled(previous);
      setError(t('settings.architecture.saveError'));
    } finally {
      setTogglingGuidance(false);
    }
  };

  const handleToggleSource = async (
    source: ArchitectureSourceResponse,
    enabled: boolean
  ) => {
    try {
      setBusyId(source.id);
      await architectureApi.updateSource(source.id, { enabled });
      await refresh();
    } catch {
      setError(t('settings.architecture.saveError'));
    } finally {
      setBusyId(null);
    }
  };

  const handleSync = async (source: ArchitectureSourceResponse) => {
    try {
      setSyncingId(source.id);
      await architectureApi.syncSource(source.id);
      await refresh();
    } catch {
      setError(t('settings.architecture.syncError'));
    } finally {
      setSyncingId(null);
    }
  };

  const handleDelete = async (source: ArchitectureSourceResponse) => {
    try {
      setBusyId(source.id);
      await architectureApi.removeSource(source.id);
      await refresh();
    } catch {
      setError(t('settings.architecture.deleteError'));
    } finally {
      setBusyId(null);
    }
  };

  const handleAdd = async () => {
    if (!addState.name.trim() || !addState.owner.trim() || !addState.repo.trim()) {
      setAddError(t('settings.architecture.addDialog.requiredFields'));
      return;
    }
    const includePaths = addState.includePaths
      .split(',')
      .map((p) => p.trim())
      .filter((p) => p.length > 0);
    try {
      setAddSaving(true);
      setAddError(null);
      await architectureApi.createSource({
        name: addState.name.trim(),
        owner: addState.owner.trim(),
        repo: addState.repo.trim(),
        branch: addState.branch.trim() || undefined,
        includePaths: includePaths.length > 0 ? includePaths : undefined,
      });
      setAddOpen(false);
      setAddState(EMPTY_SOURCE);
      await refresh();
    } catch {
      setAddError(t('settings.architecture.saveError'));
    } finally {
      setAddSaving(false);
    }
  };

  const formatSyncTime = (iso: string | null) => {
    if (!iso) return t('settings.architecture.source.neverSynced');
    try {
      return new Date(iso).toLocaleString(i18n.language);
    } catch {
      return iso;
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-double">
        <SpinnerGapIcon className="size-icon-sm animate-spin text-low" />
      </div>
    );
  }

  return (
    <div className="space-y-double pb-16">
      {error && <ErrorAlert message={error} />}

      <SettingsSection title={t('settings.architecture.title')}>
        <SettingsCard
          title={t('settings.architecture.guidanceCard.title')}
          description={t('settings.architecture.guidanceCard.description')}
        >
          <SettingsToggle
            label={t('settings.architecture.guidanceCard.toggleLabel')}
            description={t('settings.architecture.guidanceCard.toggleDescription')}
            checked={guidanceEnabled}
            disabled={togglingGuidance}
            onChange={handleToggleGuidance}
          />
        </SettingsCard>

        <SettingsCard
          title={t('settings.architecture.sourcesCard.title')}
          description={t('settings.architecture.sourcesCard.description')}
        >
          <div className="space-y-base">
            {sources.map((source) => {
              const syncOk = source.lastSyncStatus === 'ok';
              return (
                <div
                  key={source.id}
                  className="rounded border border-border bg-secondary p-base space-y-half"
                >
                  <div className="flex items-start justify-between gap-base">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-half flex-wrap">
                        <span className="text-high text-base font-medium">
                          {source.name}
                        </span>
                        {source.builtin && (
                          <span className="rounded-full border border-brand/40 px-half text-xs uppercase tracking-wider text-brand">
                            {t('settings.architecture.source.builtinBadge')}
                          </span>
                        )}
                      </div>
                      <p className="text-low text-sm mt-0.5 font-ibm-plex-mono">
                        {source.owner}/{source.repo}@{source.branch}
                      </p>
                      <p className="text-low text-xs">
                        {t('settings.architecture.source.paths', {
                          paths: source.includePaths.join(', '),
                        })}
                        {' · '}
                        {t('settings.architecture.source.entryCount', {
                          count: source.entryCount,
                        })}
                      </p>
                      <p className="flex items-center gap-0.5 text-xs">
                        {source.lastSyncStatus &&
                          (syncOk ? (
                            <CheckCircleIcon className="size-icon-xs text-success" />
                          ) : (
                            <WarningIcon className="size-icon-xs text-error" />
                          ))}
                        <span className={cn(syncOk ? 'text-low' : 'text-error')}>
                          {source.lastSyncStatus
                            ? t('settings.architecture.source.lastSync', {
                                time: formatSyncTime(source.lastSyncedAt),
                              })
                            : t('settings.architecture.source.neverSynced')}
                        </span>
                      </p>
                      {source.lastSyncStatus && !syncOk && (
                        <p className="text-error text-xs break-all">
                          {source.lastSyncStatus}
                        </p>
                      )}
                    </div>
                    <SettingsToggle
                      label=""
                      checked={source.enabled}
                      disabled={busyId === source.id}
                      onChange={(checked) => handleToggleSource(source, checked)}
                      className="shrink-0 w-auto"
                    />
                  </div>

                  <div className="flex items-center gap-half">
                    <button
                      type="button"
                      onClick={() => handleSync(source)}
                      disabled={syncingId === source.id}
                      className="inline-flex items-center gap-0.5 rounded border border-border bg-panel px-half py-0.5 text-xs text-low hover:text-normal disabled:opacity-60"
                    >
                      <ArrowsClockwiseIcon
                        className={cn(
                          'size-icon-xs',
                          syncingId === source.id && 'animate-spin'
                        )}
                      />
                      {t('settings.architecture.actions.syncNow')}
                    </button>
                    {!source.builtin && (
                      <button
                        type="button"
                        onClick={() => handleDelete(source)}
                        disabled={busyId === source.id}
                        className="inline-flex items-center gap-0.5 rounded border border-border bg-panel px-half py-0.5 text-xs text-error hover:bg-error/10 disabled:opacity-60"
                      >
                        <TrashIcon className="size-icon-xs" />
                        {t('settings.architecture.actions.delete')}
                      </button>
                    )}
                  </div>
                </div>
              );
            })}
          </div>

          <div className="mt-base">
            <PrimaryButton
              actionIcon={PlusIcon}
              value={t('settings.architecture.actions.addSource')}
              onClick={() => {
                setAddError(null);
                setAddState(EMPTY_SOURCE);
                setAddOpen(true);
              }}
            />
          </div>
        </SettingsCard>
      </SettingsSection>

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('settings.architecture.addDialog.title')}</DialogTitle>
          </DialogHeader>
          <div className="space-y-base p-double pt-0">
            {addError && <ErrorAlert message={addError} />}
            {(
              [
                ['name', 'nameLabel', 'text'],
                ['owner', 'ownerLabel', 'text'],
                ['repo', 'repoLabel', 'text'],
                ['branch', 'branchLabel', 'text'],
                ['includePaths', 'includePathsLabel', 'text'],
              ] as const
            ).map(([field, labelKey]) => (
              <div key={field} className="space-y-half">
                <label
                  className="text-normal text-sm"
                  htmlFor={`arch-source-${field}`}
                >
                  {t(`settings.architecture.addDialog.${labelKey}`)}
                </label>
                <input
                  id={`arch-source-${field}`}
                  type="text"
                  value={addState[field]}
                  onChange={(e) =>
                    setAddState((prev) => ({ ...prev, [field]: e.target.value }))
                  }
                  className="w-full rounded border border-border bg-secondary px-base py-1 text-base text-normal focus:outline-none focus:ring-1 focus:ring-brand"
                />
              </div>
            ))}
            <p className="text-low text-xs">
              {t('settings.architecture.addDialog.includePathsHint')}
            </p>
            <div className="flex justify-end gap-half">
              <PrimaryButton
                variant="tertiary"
                value={t('common:buttons.cancel')}
                onClick={() => setAddOpen(false)}
                disabled={addSaving}
              />
              <PrimaryButton
                actionIcon={addSaving ? 'spinner' : undefined}
                value={t('common:buttons.create')}
                onClick={handleAdd}
                disabled={addSaving}
              />
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
