import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { isEqual } from 'lodash';
import {
  PlusIcon,
  TrashIcon,
  SpinnerGapIcon,
  CheckCircleIcon,
} from '@phosphor-icons/react';

import { SettingsCard } from '@/components/ui-new/primitives/SettingsCard';
import { SettingsSection } from '@/components/ui-new/primitives/SettingsSection';
import { SettingsInput } from '@/components/ui-new/primitives/SettingsInput';
import { SettingsSelect } from '@/components/ui-new/primitives/SettingsSelect';
import { SettingsSaveBar } from '@/components/ui-new/primitives/SettingsSaveBar';
import { ErrorAlert } from '@/components/ui-new/primitives/ErrorAlert';
import { Button } from '@/components/ui-new/primitives/Button';
import { IconButton } from '@/components/ui-new/primitives/IconButton';
import { useProjects } from '@/hooks/useProjects';
import { useProjectMutations } from '@/hooks/useProjectMutations';
import { RepoPickerDialog } from '@/components/dialogs/shared/RepoPickerDialog';
import { CreateProjectDialog } from '@/components/ui-new/dialogs/CreateProjectDialog';
import { projectsApi } from '@/lib/api';
import { repoBranchKeys } from '@/hooks/useRepoBranches';
import type { Project, Repo, UpdateProject } from 'shared/types';

interface ProjectFormState {
  name: string;
}

function projectToFormState(project: Project): ProjectFormState {
  return {
    name: project.name,
  };
}

export function ProjectSettingsNew() {
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();
  const projectIdParam = searchParams.get('projectId') ?? '';
  const { t } = useTranslation(['settings', 'projects']);
  const queryClient = useQueryClient();

  // Fetch all projects
  const {
    projects,
    isLoading: projectsLoading,
    error: projectsError,
  } = useProjects();

  // Selected project state
  const [selectedProjectId, setSelectedProjectId] = useState<string>(
    searchParams.get('projectId') || ''
  );
  const [selectedProject, setSelectedProject] = useState<Project | null>(null);

  // Form state
  const [draft, setDraft] = useState<ProjectFormState | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  // Repositories state
  const [repositories, setRepositories] = useState<Repo[]>([]);
  const [loadingRepos, setLoadingRepos] = useState(false);
  const [repoError, setRepoError] = useState<string | null>(null);
  const [addingRepo, setAddingRepo] = useState(false);
  const [deletingRepoId, setDeletingRepoId] = useState<string | null>(null);

  // Check for unsaved changes (project name)
  const hasUnsavedChanges = useMemo(() => {
    if (!draft || !selectedProject) return false;
    return !isEqual(draft, projectToFormState(selectedProject));
  }, [draft, selectedProject]);

  // Build project options for SettingsSelect
  const projectOptions = useMemo(() => {
    if (!projects || projects.length === 0) {
      return [
        {
          value: 'no-projects',
          label: t('settings.projects.selector.noProjects'),
        },
      ];
    }
    return projects.map((project) => ({
      value: project.id,
      label: project.name,
    }));
  }, [projects, t]);

  // Handle project selection from dropdown
  const handleProjectSelect = useCallback(
    (id: string) => {
      if (id === 'no-projects') return;
      // No-op if same project
      if (id === selectedProjectId) return;

      // Confirm if there are unsaved changes
      if (hasUnsavedChanges) {
        const confirmed = globalThis.window.confirm(
          t('settings.projects.save.confirmSwitch')
        );
        if (!confirmed) return;

        // Clear local state before switching
        setDraft(null);
        setSelectedProject(null);
        setSuccess(false);
        setError(null);
      }

      // Update state and URL
      setSelectedProjectId(id);
      if (id) {
        setSearchParams({ projectId: id });
      } else {
        setSearchParams({});
      }
    },
    [hasUnsavedChanges, selectedProjectId, setSearchParams, t]
  );

  // Handle creating a new project
  const handleCreateProject = useCallback(async () => {
    const result = await CreateProjectDialog.show({});
    if (result.status === 'saved') {
      handleProjectSelect(result.project.id);
    }
  }, [handleProjectSelect]);

  // Sync selectedProjectId when URL changes (with unsaved changes prompt)
  useEffect(() => {
    if (projectIdParam === selectedProjectId) return;

    // Confirm if there are unsaved changes
    if (hasUnsavedChanges) {
      const confirmed = globalThis.window.confirm(
        t('settings.projects.save.confirmSwitch')
      );
      if (!confirmed) {
        // Revert URL to previous value
        if (selectedProjectId) {
          setSearchParams({ projectId: selectedProjectId });
        } else {
          setSearchParams({});
        }
        return;
      }

      // Clear local state before switching
      setDraft(null);
      setSelectedProject(null);
      setSuccess(false);
      setError(null);
    }

    setSelectedProjectId(projectIdParam);
  }, [
    projectIdParam,
    hasUnsavedChanges,
    selectedProjectId,
    setSearchParams,
    t,
  ]);

  // Populate draft from server data
  useEffect(() => {
    if (!projects) return;

    const nextProject = selectedProjectId
      ? projects.find((p) => p.id === selectedProjectId)
      : null;

    setSelectedProject((prev) =>
      prev?.id === nextProject?.id ? prev : (nextProject ?? null)
    );

    if (!nextProject) {
      if (!hasUnsavedChanges) setDraft(null);
      return;
    }

    if (hasUnsavedChanges) return;

    setDraft(projectToFormState(nextProject));
  }, [projects, selectedProjectId, hasUnsavedChanges]);

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

  // Fetch repositories when project changes
  useEffect(() => {
    if (!selectedProjectId) {
      setRepositories([]);
      return;
    }

    setLoadingRepos(true);
    setRepoError(null);
    projectsApi
      .getRepositories(selectedProjectId)
      .then(setRepositories)
      .catch((err) => {
        setRepoError(
          err instanceof Error ? err.message : 'Failed to load repositories'
        );
        setRepositories([]);
      })
      .finally(() => setLoadingRepos(false));
  }, [selectedProjectId]);

  const handleAddRepository = async () => {
    if (!selectedProjectId) return;

    const repo = await RepoPickerDialog.show({
      title: 'Select Git Repository',
      description: 'Choose a git repository to add to this project',
    });

    if (!repo) return;

    if (repositories.some((r) => r.id === repo.id)) {
      return;
    }

    setAddingRepo(true);
    setRepoError(null);
    try {
      const newRepo = await projectsApi.addRepository(selectedProjectId, {
        displayName: repo.displayName,
        gitRepoPath: repo.path,
      });
      setRepositories((prev) => [...prev, newRepo]);
      queryClient.invalidateQueries({
        queryKey: ['projectRepositories', selectedProjectId],
      });
      queryClient.invalidateQueries({
        queryKey: ['repos'],
      });
      queryClient.invalidateQueries({
        queryKey: repoBranchKeys.byRepo(newRepo.id),
      });
    } catch (err) {
      setRepoError(
        err instanceof Error ? err.message : 'Failed to add repository'
      );
    } finally {
      setAddingRepo(false);
    }
  };

  const handleDeleteRepository = async (repoId: string) => {
    if (!selectedProjectId) return;

    setDeletingRepoId(repoId);
    setRepoError(null);
    try {
      await projectsApi.deleteRepository(selectedProjectId, repoId);
      setRepositories((prev) => prev.filter((r) => r.id !== repoId));
      queryClient.invalidateQueries({
        queryKey: ['projectRepositories', selectedProjectId],
      });
      queryClient.invalidateQueries({
        queryKey: ['repos'],
      });
      queryClient.invalidateQueries({
        queryKey: repoBranchKeys.byRepo(repoId),
      });
    } catch (err) {
      setRepoError(
        err instanceof Error ? err.message : 'Failed to delete repository'
      );
    } finally {
      setDeletingRepoId(null);
    }
  };

  const { updateProject } = useProjectMutations({
    onUpdateSuccess: (updatedProject: Project) => {
      setSelectedProject(updatedProject);
      setDraft(projectToFormState(updatedProject));
      setSuccess(true);
      setTimeout(() => setSuccess(false), 3000);
      setSaving(false);
    },
    onUpdateError: (err) => {
      setError(
        err instanceof Error ? err.message : 'Failed to save project settings'
      );
      setSaving(false);
    },
  });

  const handleSave = async () => {
    if (!draft || !selectedProject) return;

    setSaving(true);
    setError(null);
    setSuccess(false);

    try {
      const updateData: UpdateProject = {
        name: draft.name.trim(),
        defaultAgentWorkingDir: selectedProject.defaultAgentWorkingDir,
      };

      updateProject.mutate({
        projectId: selectedProject.id,
        data: updateData,
      });
    } catch (err) {
      setError(t('settings.projects.save.error'));
      console.error('Error saving project settings:', err);
      setSaving(false);
    }
  };

  const handleDiscard = () => {
    if (!selectedProject) return;
    setDraft(projectToFormState(selectedProject));
  };

  if (projectsLoading) {
    return (
      <div className="flex items-center justify-center py-double">
        <SpinnerGapIcon className="size-icon-lg animate-spin text-low" weight="bold" />
        <span className="ml-base text-normal text-sm">
          {t('settings.projects.loading')}
        </span>
      </div>
    );
  }

  if (projectsError) {
    return (
      <div className="py-double">
        <ErrorAlert
          message={
            projectsError instanceof Error
              ? projectsError.message
              : t('settings.projects.loadError')
          }
        />
      </div>
    );
  }

  return (
    <div className="space-y-base">
      {error && <ErrorAlert message={error} />}

      {success && (
        <div className="relative w-full border border-success bg-success/10 p-base text-sm text-success flex items-center gap-half">
          <CheckCircleIcon className="size-icon-sm" weight="bold" />
          <span className="font-medium">
            {t('settings.projects.save.success')}
          </span>
        </div>
      )}

      {/* Project Selector Card */}
      <SettingsCard
        title={t('settings.projects.title')}
        description={t('settings.projects.description')}
      >
        <SettingsSection>
          <div className="flex items-start justify-between gap-double">
            <div className="flex-1 min-w-0">
              <SettingsSelect
                label={t('settings.projects.selector.label')}
                description={t('settings.projects.selector.helper')}
                value={selectedProjectId}
                onChange={handleProjectSelect}
                options={projectOptions}
                placeholder={t('settings.projects.selector.placeholder')}
              />
            </div>
            <div className="shrink-0 pt-0.5">
              <Button variant="outline" size="sm" onClick={handleCreateProject}>
                <PlusIcon className="size-icon-xs" weight="bold" />
                {t('projects:createProject')}
              </Button>
            </div>
          </div>
        </SettingsSection>
      </SettingsCard>

      {selectedProject && draft && (
        <>
          {/* General Settings Card */}
          <SettingsCard
            title={t('settings.projects.general.title')}
            description={t('settings.projects.general.description')}
          >
            <SettingsSection>
              <SettingsInput
                label={t('settings.projects.general.name.label')}
                description={t('settings.projects.general.name.helper')}
                value={draft.name}
                onChange={(value) =>
                  setDraft((prev) => (prev ? { ...prev, name: value } : prev))
                }
                placeholder={t('settings.projects.general.name.placeholder')}
              />

              {/* Inline save controls */}
              <div className="flex items-center justify-between pt-base border-t border-border">
                {hasUnsavedChanges ? (
                  <span className="text-sm text-low">
                    {t('settings.projects.save.unsavedChanges')}
                  </span>
                ) : (
                  <span />
                )}
                <div className="flex gap-half">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleDiscard}
                    disabled={saving || !hasUnsavedChanges}
                  >
                    {t('settings.projects.save.discard')}
                  </Button>
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={handleSave}
                    disabled={saving || !hasUnsavedChanges}
                  >
                    {saving && (
                      <SpinnerGapIcon
                        className="size-icon-xs animate-spin"
                        weight="bold"
                      />
                    )}
                    {saving
                      ? t('settings.projects.save.saving')
                      : t('settings.projects.save.button')}
                  </Button>
                </div>
              </div>
            </SettingsSection>
          </SettingsCard>

          {/* Repositories Card */}
          <SettingsCard
            title="Repositories"
            description="Manage the git repositories in this project"
          >
            <SettingsSection>
              {repoError && <ErrorAlert message={repoError} />}

              {loadingRepos ? (
                <div className="flex items-center justify-center py-base">
                  <SpinnerGapIcon
                    className="size-icon-sm animate-spin text-low"
                    weight="bold"
                  />
                  <span className="ml-half text-sm text-low">
                    Loading repositories...
                  </span>
                </div>
              ) : (
                <div className="space-y-half">
                  {repositories.map((repo) => (
                    <button
                      type="button"
                      key={repo.id}
                      className="flex items-center justify-between p-base border border-border rounded bg-secondary hover:bg-surface-2 cursor-pointer transition-colors duration-200 w-full text-left"
                      onClick={() =>
                        navigate(`/settings/repos?repoId=${repo.id}`)
                      }
                    >
                      <div className="min-w-0 flex-1">
                        <div className="text-normal text-base font-medium">
                          {repo.displayName}
                        </div>
                        <div className="text-sm text-low truncate">
                          {repo.path}
                        </div>
                      </div>
                      <div
                        role="none"
                        onClick={(e) => {
                          e.stopPropagation();
                        }}
                      >
                        {deletingRepoId === repo.id ? (
                          <SpinnerGapIcon
                            className="size-icon-sm animate-spin text-low"
                            weight="bold"
                          />
                        ) : (
                          <IconButton
                            icon={TrashIcon}
                            aria-label="Delete repository"
                            title="Delete repository"
                            onClick={() => {
                              handleDeleteRepository(repo.id);
                            }}
                          />
                        )}
                      </div>
                    </button>
                  ))}

                  {repositories.length === 0 && !loadingRepos && (
                    <div className="text-center py-base text-sm text-low">
                      No repositories configured
                    </div>
                  )}

                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleAddRepository}
                    disabled={addingRepo}
                    className="w-full"
                  >
                    {addingRepo ? (
                      <SpinnerGapIcon
                        className="size-icon-xs animate-spin"
                        weight="bold"
                      />
                    ) : (
                      <PlusIcon className="size-icon-xs" weight="bold" />
                    )}
                    Add Repository
                  </Button>
                </div>
              )}
            </SettingsSection>
          </SettingsCard>

          {/* Sticky save bar at bottom */}
          <SettingsSaveBar
            visible={hasUnsavedChanges}
            onSave={handleSave}
            onDiscard={handleDiscard}
            saving={saving}
          />
        </>
      )}
    </div>
  );
}
