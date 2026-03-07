import { useMemo, useCallback, useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useCreateMode } from '@/contexts/CreateModeContext';
import { useUserSystem } from '@/components/ConfigProvider';
import { useProjects } from '@/hooks/useProjects';
import { useCreateWorkspace } from '@/hooks/useCreateWorkspace';
import { useCreateAttachments } from '@/hooks/useCreateAttachments';
import { getVariantOptions, areProfilesEqual } from '@/utils/executor';
import { splitMessageToTitleDescription } from '@/utils/string';
import type { ExecutorProfileId, BaseCodingAgent } from 'shared/types';
import { CreateChatBox } from '../primitives/CreateChatBox';
import { ProjectSelectorContainer } from './ProjectSelectorContainer';
import { CreateProjectDialog } from '@/components/ui-new/dialogs/CreateProjectDialog';

export function CreateChatBoxContainer() {
  const { t } = useTranslation('common');
  const { profiles, config, updateAndSaveConfig } = useUserSystem();
  const {
    repos,
    targetBranches,
    selectedProfile,
    setSelectedProfile,
    message,
    setMessage,
    selectedProjectId,
    setSelectedProjectId,
    clearDraft,
    hasInitialValue,
  } = useCreateMode();

  const { createWorkspace } = useCreateWorkspace();
  const [hasAttemptedSubmit, setHasAttemptedSubmit] = useState(false);
  const [saveAsDefault, setSaveAsDefault] = useState(false);

  // Attachment handling - insert markdown and track image IDs
  const handleInsertMarkdown = useCallback(
    (markdown: string) => {
      const newMessage = message.trim()
        ? `${message}\n\n${markdown}`
        : markdown;
      setMessage(newMessage);
    },
    [message, setMessage]
  );

  const { uploadFiles, getImageIds, clearAttachments, localImages } =
    useCreateAttachments(handleInsertMarkdown);

  // Default to user's config profile or first available executor
  const effectiveProfile = useMemo<ExecutorProfileId | null>(() => {
    if (selectedProfile) return selectedProfile;
    if (config?.executor_profile) return config.executor_profile;
    if (profiles) {
      const firstExecutor = Object.keys(profiles)[0] as BaseCodingAgent;
      if (firstExecutor) {
        const variants = Object.keys(profiles[firstExecutor]);
        return {
          executor: firstExecutor,
          variant: variants[0] ?? null,
        };
      }
    }
    return null;
  }, [selectedProfile, config?.executor_profile, profiles]);

  // Get variant options for the current executor
  const variantOptions = useMemo(
    () => getVariantOptions(effectiveProfile?.executor, profiles),
    [effectiveProfile?.executor, profiles]
  );

  // Detect if user has changed from their saved default
  const hasChangedFromDefault = useMemo(() => {
    if (!config?.executor_profile || !effectiveProfile) return false;
    return !areProfilesEqual(effectiveProfile, config.executor_profile);
  }, [effectiveProfile, config?.executor_profile]);

  // Reset toggle when profile matches default again
  useEffect(() => {
    if (!hasChangedFromDefault) {
      setSaveAsDefault(false);
    }
  }, [hasChangedFromDefault]);

  const { projects } = useProjects();
  const projectId = selectedProjectId;
  const selectedProject = projects.find((p) => p.id === projectId);

  const handleCreateProjectNoAutoSelect = useCallback(async () => {
    await CreateProjectDialog.show({});
  }, []);

  const canSubmit =
    repos.length > 0 &&
    message.trim().length > 0 &&
    effectiveProfile !== null &&
    projectId !== undefined;

  // Handle variant change
  const handleVariantChange = useCallback(
    (variant: string | null) => {
      if (!effectiveProfile) return;
      setSelectedProfile({
        executor: effectiveProfile.executor,
        variant,
      });
    },
    [effectiveProfile, setSelectedProfile]
  );

  // Handle executor change - use saved variant if switching to default executor
  const handleExecutorChange = useCallback(
    (executor: BaseCodingAgent) => {
      const executorConfig = profiles?.[executor];
      if (!executorConfig) {
        setSelectedProfile({ executor, variant: null });
        return;
      }

      const variants = Object.keys(executorConfig);
      let targetVariant: string | null = null;

      // If switching to user's default executor, use their saved variant
      if (
        config?.executor_profile?.executor === executor &&
        config?.executor_profile?.variant
      ) {
        const savedVariant = config.executor_profile.variant;
        if (variants.includes(savedVariant)) {
          targetVariant = savedVariant;
        }
      }

      // Fallback to DEFAULT or first available
      if (!targetVariant) {
        if (variants.includes('DEFAULT')) {
          targetVariant = 'DEFAULT';
        } else {
          targetVariant = variants[0] ?? null;
        }
      }

      setSelectedProfile({ executor, variant: targetVariant });
    },
    [profiles, setSelectedProfile, config?.executor_profile]
  );

  // Handle submit
  const handleSubmit = useCallback(async () => {
    setHasAttemptedSubmit(true);
    if (!canSubmit || !effectiveProfile || !projectId) return;

    // Save profile as default if toggle is checked
    if (saveAsDefault && hasChangedFromDefault) {
      await updateAndSaveConfig({ executor_profile: effectiveProfile });
    }

    const { title, description } = splitMessageToTitleDescription(message);

    await createWorkspace.mutateAsync({
      task: {
        projectId: projectId,
        title,
        description,
        status: null,
        parentWorkspaceId: null,
        imageIds: getImageIds(),
        sharedTaskId: null,
      },
      executor_profile_id: effectiveProfile,
      repos: repos.map((r) => ({
        repo_id: r.id,
        target_branch: targetBranches[r.id] ?? 'main',
      })),
    });

    // Clear attachments and draft after successful creation
    clearAttachments();
    await clearDraft();
  }, [
    canSubmit,
    effectiveProfile,
    projectId,
    message,
    repos,
    targetBranches,
    createWorkspace,
    getImageIds,
    clearAttachments,
    clearDraft,
    saveAsDefault,
    hasChangedFromDefault,
    updateAndSaveConfig,
  ]);

  // Determine error to display
  const displayError = (() => {
    if (hasAttemptedSubmit && repos.length === 0) {
      return 'Add at least one repository to create a workspace';
    } else if (createWorkspace.error) {
      if (createWorkspace.error instanceof Error) {
        return createWorkspace.error.message;
      } else {
        return 'Failed to create workspace';
      }
    } else {
      return null;
    }
  })();

  // Wait for initial value to be applied before rendering
  // This ensures the editor mounts with content ready, so autoFocus works correctly
  if (!hasInitialValue) {
    return null;
  }

  if (!projectId) {
    return (
      <div className="flex h-full w-full items-center justify-center">
        <div className="text-center max-w-sm">
          <h2 className="text-lg font-medium text-high mb-2">
            {t('workspace.selectProjectTitle')}
          </h2>
          <p className="text-sm text-low mb-4">
            {t('workspace.selectProjectHint')}
          </p>
          <ProjectSelectorContainer
            projects={projects}
            selectedProjectId={null}
            onProjectSelect={(p) => setSelectedProjectId(p.id)}
            onCreateProject={handleCreateProjectNoAutoSelect}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="relative flex flex-1 flex-col bg-primary h-full">
      <div className="flex-1" />
      <div className="flex justify-center @container">
        <div className="w-full max-w-3xl px-4 mb-2">
          <div className="flex items-center gap-2 text-xs text-low">
            <span>{t('workspace.currentProject')}</span>
            <ProjectSelectorContainer
              projects={projects}
              selectedProjectId={projectId}
              selectedProjectName={selectedProject?.name}
              onProjectSelect={(p) => setSelectedProjectId(p.id)}
              onCreateProject={handleCreateProjectNoAutoSelect}
            />
          </div>
        </div>
        <CreateChatBox
          editor={{
            value: message,
            onChange: setMessage,
          }}
          onSend={handleSubmit}
          isSending={createWorkspace.isPending}
          executor={{
            selected: effectiveProfile?.executor ?? null,
            options: Object.keys(profiles || {}) as BaseCodingAgent[],
            onChange: handleExecutorChange,
          }}
          variant={
            effectiveProfile
              ? {
                  selected: effectiveProfile.variant ?? 'DEFAULT',
                  options: variantOptions,
                  onChange: handleVariantChange,
                }
              : undefined
          }
          saveAsDefault={{
            checked: saveAsDefault,
            onChange: setSaveAsDefault,
            visible: hasChangedFromDefault,
          }}
          error={displayError}
          projectId={projectId}
          agent={effectiveProfile?.executor ?? null}
          onPasteFiles={uploadFiles}
          localImages={localImages}
        />
      </div>
    </div>
  );
}
