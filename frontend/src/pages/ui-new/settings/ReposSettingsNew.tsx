import { useCallback, useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { isEqual } from 'lodash';
import { SpinnerGapIcon, CheckCircleIcon } from '@phosphor-icons/react';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { cn } from '@/lib/utils';
import { SettingsCard } from '@/components/ui-new/primitives/SettingsCard';
import { SettingsSection } from '@/components/ui-new/primitives/SettingsSection';
import { SettingsRow } from '@/components/ui-new/primitives/SettingsRow';
import { SettingsSelect } from '@/components/ui-new/primitives/SettingsSelect';
import { SettingsInput } from '@/components/ui-new/primitives/SettingsInput';
import { SettingsToggle } from '@/components/ui-new/primitives/SettingsToggle';
import { SettingsSaveBar } from '@/components/ui-new/primitives/SettingsSaveBar';
import { ErrorAlert } from '@/components/ui-new/primitives/ErrorAlert';
import { AutoExpandingTextarea } from '@/components/ui/auto-expanding-textarea';
import { MultiFileSearchTextarea } from '@/components/ui/multi-file-search-textarea';
import { useScriptPlaceholders } from '@/hooks/useScriptPlaceholders';
import { repoApi } from '@/lib/api';
import { repoBranchKeys } from '@/hooks/useRepoBranches';
import { ConfirmDialog } from '@/components/ui-new/dialogs/ConfirmDialog';
import type { Repo, UpdateRepo } from 'shared/types';

interface RepoScriptsFormState {
  displayName: string;
  setupScript: string;
  parallelSetupScript: boolean;
  cleanupScript: string;
  copyFiles: string;
  devServerScript: string;
}

function repoToFormState(repo: Repo): RepoScriptsFormState {
  return {
    displayName: repo.displayName,
    setupScript: repo.setupScript ?? '',
    parallelSetupScript: repo.parallelSetupScript,
    cleanupScript: repo.cleanupScript ?? '',
    copyFiles: repo.copyFiles ?? '',
    devServerScript: repo.devServerScript ?? '',
  };
}

export function ReposSettingsNew() {
  const [searchParams, setSearchParams] = useSearchParams();
  const repoIdParam = searchParams.get('repoId') ?? '';
  const { t } = useTranslation('settings');
  const queryClient = useQueryClient();

  // Fetch all repos
  const {
    data: repos,
    isLoading: reposLoading,
    error: reposError,
  } = useQuery({
    queryKey: ['repos'],
    queryFn: () => repoApi.list(),
  });

  // Selected repo state
  const [selectedRepoId, setSelectedRepoId] = useState<string>(repoIdParam);
  const [selectedRepo, setSelectedRepo] = useState<Repo | null>(null);

  // Form state
  const [draft, setDraft] = useState<RepoScriptsFormState | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  // Get OS-appropriate script placeholders
  const placeholders = useScriptPlaceholders();

  // Check for unsaved changes
  const hasUnsavedChanges = useMemo(() => {
    if (!draft || !selectedRepo) return false;
    return !isEqual(draft, repoToFormState(selectedRepo));
  }, [draft, selectedRepo]);

  // Handle repo selection from dropdown
  const handleRepoSelect = useCallback(
    async (id: string) => {
      if (id === selectedRepoId) return;

      if (hasUnsavedChanges) {
        const result = await ConfirmDialog.show({
          title: t('settings.repos.save.confirmSwitchTitle'),
          message: t('settings.repos.save.confirmSwitch'),
          variant: 'destructive',
        });
        if (result !== 'confirmed') return;
        setDraft(null);
        setSelectedRepo(null);
        setSuccess(false);
        setError(null);
      }

      setSelectedRepoId(id);
      if (id) {
        setSearchParams({ repoId: id });
      } else {
        setSearchParams({});
      }
    },
    [hasUnsavedChanges, selectedRepoId, setSearchParams, t]
  );

  // Sync selectedRepoId when URL changes
  useEffect(() => {
    if (repoIdParam === selectedRepoId) return;

    if (hasUnsavedChanges) {
      (async () => {
        const result = await ConfirmDialog.show({
          title: t('settings.repos.save.confirmSwitchTitle'),
          message: t('settings.repos.save.confirmSwitch'),
          variant: 'destructive',
        });
        if (result !== 'confirmed') {
          if (selectedRepoId) {
            setSearchParams({ repoId: selectedRepoId });
          } else {
            setSearchParams({});
          }
          return;
        }
        setDraft(null);
        setSelectedRepo(null);
        setSuccess(false);
        setError(null);
        setSelectedRepoId(repoIdParam);
      })();
      return;
    }

    setSelectedRepoId(repoIdParam);
  }, [repoIdParam, hasUnsavedChanges, selectedRepoId, setSearchParams, t]);

  // Populate draft from server data
  useEffect(() => {
    if (!repos) return;

    const nextRepo = selectedRepoId
      ? repos.find((r) => r.id === selectedRepoId)
      : null;

    setSelectedRepo((prev) =>
      prev?.id === nextRepo?.id ? prev : (nextRepo ?? null)
    );

    if (!nextRepo) {
      if (!hasUnsavedChanges) setDraft(null);
      return;
    }

    if (hasUnsavedChanges) return;

    setDraft(repoToFormState(nextRepo));
  }, [repos, selectedRepoId, hasUnsavedChanges]);

  // Warn on tab close/navigation with unsaved changes
  useEffect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      if (hasUnsavedChanges) {
        e.preventDefault();
      }
    };
    globalThis.addEventListener('beforeunload', handler);
    return () => globalThis.removeEventListener('beforeunload', handler);
  }, [hasUnsavedChanges]);

  const handleSave = async () => {
    if (!draft || !selectedRepo) return;

    setSaving(true);
    setError(null);
    setSuccess(false);

    try {
      // Normalize empty strings to null at save time
      const normalize = (v: string): string | null => {
        const trimmed = v.trim();
        return trimmed === '' ? null : trimmed;
      };
      const updateData: UpdateRepo = {
        displayName: normalize(draft.displayName),
        setupScript: normalize(draft.setupScript),
        cleanupScript: normalize(draft.cleanupScript),
        copyFiles: normalize(draft.copyFiles),
        parallelSetupScript: draft.parallelSetupScript,
        devServerScript: normalize(draft.devServerScript),
      };

      const updatedRepo = await repoApi.update(selectedRepo.id, updateData);
      setSelectedRepo(updatedRepo);
      setDraft(repoToFormState(updatedRepo));
      queryClient.setQueryData(['repos'], (old: Repo[] | undefined) =>
        old?.map((r) => (r.id === updatedRepo.id ? updatedRepo : r))
      );
      // Invalidate branch data since repo config may have changed
      queryClient.invalidateQueries({
        queryKey: repoBranchKeys.byRepo(updatedRepo.id),
      });
      setSuccess(true);
      setTimeout(() => setSuccess(false), 3000);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : t('settings.repos.save.error')
      );
    } finally {
      setSaving(false);
    }
  };

  const handleDiscard = () => {
    if (!selectedRepo) return;
    setDraft(repoToFormState(selectedRepo));
  };

  const updateDraft = (updates: Partial<RepoScriptsFormState>) => {
    setDraft((prev) => {
      if (!prev) return prev;
      return { ...prev, ...updates };
    });
  };

  // Build repo options for SettingsSelect
  const repoOptions = useMemo(() => {
    if (!repos || repos.length === 0) {
      return [{ value: '', label: t('settings.repos.selector.noRepos') }];
    }
    return repos.map((repo) => ({
      value: repo.id,
      label: repo.displayName,
    }));
  }, [repos, t]);

  if (reposLoading) {
    return (
      <div className="flex items-center justify-center py-double">
        <SpinnerGapIcon
          className="size-icon-lg animate-spin text-low"
          weight="bold"
        />
        <span className="ml-base text-low text-sm">
          {t('settings.repos.loading')}
        </span>
      </div>
    );
  }

  if (reposError) {
    return (
      <div className="py-double">
        <ErrorAlert
          message={
            reposError instanceof Error
              ? reposError.message
              : t('settings.repos.loadError')
          }
        />
      </div>
    );
  }

  return (
    <div className="space-y-base pb-16">
      {error && <ErrorAlert message={error} />}

      {success && (
        <div className="flex items-center gap-half border border-success bg-success/10 p-base text-sm text-success rounded">
          <CheckCircleIcon className="size-icon-sm" weight="bold" />
          <span className="font-medium">
            {t('settings.repos.save.success')}
          </span>
        </div>
      )}

      {/* Repo Selector */}
      <SettingsCard
        title={t('settings.repos.title')}
        description={t('settings.repos.description')}
      >
        <SettingsSelect
          label={t('settings.repos.selector.label')}
          description={t('settings.repos.selector.helper')}
          value={selectedRepoId}
          onChange={handleRepoSelect}
          options={repoOptions}
          placeholder={t('settings.repos.selector.placeholder')}
        />
      </SettingsCard>

      {selectedRepo && draft && (
        <>
          {/* General Settings */}
          <SettingsCard
            title={t('settings.repos.general.title')}
            description={t('settings.repos.general.description')}
          >
            <SettingsSection>
              <SettingsInput
                label={t('settings.repos.general.displayName.label')}
                description={t('settings.repos.general.displayName.helper')}
                value={draft.displayName}
                onChange={(val) => updateDraft({ displayName: val })}
                placeholder={t(
                  'settings.repos.general.displayName.placeholder'
                )}
              />
              <SettingsRow
                label={t('settings.repos.general.path.label')}
              >
                <span className="text-low text-sm font-ibm-plex-mono bg-secondary px-base py-1 rounded border border-border">
                  {selectedRepo.path}
                </span>
              </SettingsRow>
            </SettingsSection>
          </SettingsCard>

          {/* Scripts Settings */}
          <SettingsCard
            title={t('settings.repos.scripts.title')}
            description={t('settings.repos.scripts.description')}
          >
            <SettingsSection>
              {/* Dev Server Script */}
              <div className="space-y-1">
                <span className="text-normal text-base">
                  {t('settings.repos.scripts.devServer.label')}
                </span>
                <AutoExpandingTextarea
                  id="dev-server-script"
                  value={draft.devServerScript}
                  onChange={(e) =>
                    updateDraft({ devServerScript: e.target.value })
                  }
                  placeholder={placeholders.dev}
                  maxRows={12}
                  className={cn(
                    'w-full px-base py-1 border border-border bg-secondary text-normal rounded',
                    'focus:outline-none focus:ring-1 focus:ring-brand font-ibm-plex-mono text-base'
                  )}
                />
                <p className="text-low text-sm">
                  {t('settings.repos.scripts.devServer.helper')}
                </p>
              </div>

              {/* Setup Script */}
              <div className="space-y-1">
                <span className="text-normal text-base">
                  {t('settings.repos.scripts.setup.label')}
                </span>
                <AutoExpandingTextarea
                  id="setup-script"
                  value={draft.setupScript}
                  onChange={(e) =>
                    updateDraft({ setupScript: e.target.value })
                  }
                  placeholder={placeholders.setup}
                  maxRows={12}
                  className={cn(
                    'w-full px-base py-1 border border-border bg-secondary text-normal rounded',
                    'focus:outline-none focus:ring-1 focus:ring-brand font-ibm-plex-mono text-base'
                  )}
                />
                <p className="text-low text-sm">
                  {t('settings.repos.scripts.setup.helper')}
                </p>

                <SettingsToggle
                  label={t('settings.repos.scripts.setup.parallelLabel')}
                  description={t(
                    'settings.repos.scripts.setup.parallelHelper'
                  )}
                  checked={draft.parallelSetupScript}
                  onChange={(checked) =>
                    updateDraft({ parallelSetupScript: checked })
                  }
                  disabled={!draft.setupScript.trim()}
                  className="pt-half"
                />
              </div>

              {/* Cleanup Script */}
              <div className="space-y-1">
                <span className="text-normal text-base">
                  {t('settings.repos.scripts.cleanup.label')}
                </span>
                <AutoExpandingTextarea
                  id="cleanup-script"
                  value={draft.cleanupScript}
                  onChange={(e) =>
                    updateDraft({ cleanupScript: e.target.value })
                  }
                  placeholder={placeholders.cleanup}
                  maxRows={12}
                  className={cn(
                    'w-full px-base py-1 border border-border bg-secondary text-normal rounded',
                    'focus:outline-none focus:ring-1 focus:ring-brand font-ibm-plex-mono text-base'
                  )}
                />
                <p className="text-low text-sm">
                  {t('settings.repos.scripts.cleanup.helper')}
                </p>
              </div>

              {/* Copy Files */}
              <div className="space-y-1">
                <span className="text-normal text-base">
                  {t('settings.repos.scripts.copyFiles.label')}
                </span>
                <MultiFileSearchTextarea
                  value={draft.copyFiles}
                  onChange={(value) => updateDraft({ copyFiles: value })}
                  placeholder={t(
                    'settings.repos.scripts.copyFiles.placeholder'
                  )}
                  maxRows={6}
                  repoId={selectedRepo.id}
                  className={cn(
                    'w-full px-base py-1 border border-border bg-secondary text-normal rounded',
                    'focus:outline-none focus:ring-1 focus:ring-brand font-ibm-plex-mono text-base'
                  )}
                />
                <p className="text-low text-sm">
                  {t('settings.repos.scripts.copyFiles.helper')}
                </p>
              </div>
            </SettingsSection>
          </SettingsCard>
        </>
      )}

      {/* Sticky save bar */}
      <SettingsSaveBar
        visible={hasUnsavedChanges}
        onSave={handleSave}
        onDiscard={handleDiscard}
        saving={saving}
      />
    </div>
  );
}
