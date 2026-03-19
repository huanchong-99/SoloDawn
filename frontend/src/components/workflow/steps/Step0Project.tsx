import React, { useEffect, useState } from 'react';
import {
  GitBranch as GitBranchIcon,
  Warning as WarningIcon,
  CheckCircle as CheckIcon,
  ArrowsClockwise as RefreshIcon,
  XCircle as XCircleIcon,
} from '@phosphor-icons/react';
import { Field, FieldLabel, FieldError } from '../../ui-new/primitives/Field';
import { InputField } from '../../ui-new/primitives/InputField';
import { PrimaryButton } from '../../ui-new/primitives/PrimaryButton';
import type { ProjectConfig, GitStatus } from '../types';
import { useTranslation } from 'react-i18next';
import { useErrorNotification } from '@/hooks/useErrorNotification';
import { FolderPickerDialog } from '@/components/dialogs/shared/FolderPickerDialog';
import { useUserSystem } from '@/components/ConfigProvider';
import { useProjectRepos } from '@/hooks/useProjectRepos';

interface Step0ProjectProps {
  config: ProjectConfig;
  projectId?: string;
  onChange: (updates: Partial<ProjectConfig>) => void;
  errors: Record<string, string>;
  onError?: (error: Error) => void;
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null;

const isGitStatus = (value: unknown): value is GitStatus => {
  if (!isRecord(value)) return false;
  return (
    typeof value.isGitRepo === 'boolean' &&
    typeof value.isDirty === 'boolean' &&
    (value.currentBranch == null || typeof value.currentBranch === 'string') &&
    (value.remoteUrl == null || typeof value.remoteUrl === 'string') &&
    (value.uncommittedChanges == null || typeof value.uncommittedChanges === 'number')
  );
};

const getErrorMessage = (value: unknown, fallback: string): string => {
  if (isRecord(value) && typeof value.error === 'string' && value.error.trim()) {
    return value.error;
  }
  return fallback;
};

const parseJson = async (response: Response): Promise<unknown> => {
  try {
    return (await response.json()) as unknown;
  } catch {
    return null;
  }
};

/**
 * Step 0: Selects the working directory and git status for the workflow.
 */
export const Step0Project: React.FC<Step0ProjectProps> = ({
  config,
  projectId,
  onChange,
  errors,
  onError,
}) => {
  const { t } = useTranslation(['workflow', 'common']);
  const { notifyError } = useErrorNotification({ onError, context: 'Step0Project' });
  const { environment } = useUserSystem();
  const [isLoading, setIsLoading] = useState(false);
  const [apiError, setApiError] = useState<string | null>(null);

  // Fetch project-bound repos for auto-fill
  const { data: projectRepos } = useProjectRepos(projectId);

  const folderPickerInitialPath =
    config.workingDirectory ||
    (environment?.is_containerized ? environment.workspace_root_hint ?? '' : '');

  const handleSelectFolder = async () => {
    try {
      const selectedPath = await FolderPickerDialog.show({
        value: folderPickerInitialPath,
        title: t('common:dialogs.selectGitRepository', {
          defaultValue: 'Select Git Repository',
        }),
        description: t('common:dialogs.chooseExistingRepo', {
          defaultValue: 'Choose an existing repository folder',
        }),
      });

      if (!selectedPath) {
        return;
      }

      onChange({ workingDirectory: selectedPath });
      setApiError(null);
      void checkGitStatus(selectedPath);
    } catch (error) {
      notifyError(error, 'selectFolder');
      setApiError(t('step0.errors.folderPicker'));
    }
  };

  const checkGitStatus = async (path: string) => {
    setIsLoading(true);
    setApiError(null);
    try {
      const response = await fetch('/api/git/status', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path }),
      });

      const data = await parseJson(response);
      if (response.ok) {
        const apiResponse = data as { success?: boolean; data?: unknown };
        if (apiResponse?.success && isGitStatus(apiResponse.data)) {
          const gitStatusData = apiResponse.data;
          onChange({ gitStatus: gitStatusData });
        } else {
          setApiError(getErrorMessage(data, t('step0.errors.gitStatus')));
        }
      } else {
        setApiError(getErrorMessage(data, t('step0.errors.gitStatus')));
      }
    } catch (error) {
      notifyError(error, 'checkGitStatus');
      setApiError(t('step0.errors.gitNetwork'));
    } finally {
      setIsLoading(false);
    }
  };

  // Auto-select when exactly one repo is bound and no directory is set
  const didAutoSelect = React.useRef(false);
  useEffect(() => {
    if (didAutoSelect.current) return;
    if (projectRepos?.length === 1 && !config.workingDirectory) {
      didAutoSelect.current = true;
      onChange({ workingDirectory: projectRepos[0].path });
    }
  }, [projectRepos, config.workingDirectory, onChange]);

  const handleInitGit = async () => {
    setIsLoading(true);
    setApiError(null);
    try {
      const response = await fetch('/api/git/init', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: config.workingDirectory }),
      });

      const data = await parseJson(response);
      if (response.ok) {
        await checkGitStatus(config.workingDirectory);
      } else {
        setApiError(getErrorMessage(data, t('step0.errors.gitInit')));
      }
    } catch (error) {
      notifyError(error, 'initGit');
      setApiError(t('step0.errors.gitNetwork'));
    } finally {
      setIsLoading(false);
    }
  };

  const gitStatus = config.gitStatus;

  return (
    <div className="flex flex-col gap-base">
      <Field>
        <FieldLabel>{t('step0.fieldLabel')}</FieldLabel>
        <div className="flex gap-base">
          <InputField
            value={config.workingDirectory}
            onChange={(value) => {
              onChange({ workingDirectory: value });
              setApiError(null);
              if (value) {
                void checkGitStatus(value);
              }
            }}
            placeholder={t('step0.placeholder')}
            className="flex-1"
            variant="search"
          />
          <PrimaryButton variant="secondary" onClick={handleSelectFolder}>
            {t('step0.browse')}
          </PrimaryButton>
        </div>
        {errors.workingDirectory && (
          <FieldError>{t(errors.workingDirectory)}</FieldError>
        )}
        {apiError && (
          <div className="flex items-center gap-half text-sm text-error mt-half">
            <XCircleIcon className="size-icon-xs" />
            <span>{apiError}</span>
          </div>
        )}
        {environment?.is_containerized && environment.workspace_root_hint && (
          <div className="mt-half text-xs text-low">
            {t('workflow:step0.containerHint', {
              path: environment.workspace_root_hint,
            })}
          </div>
        )}
      </Field>

      {/* Project-bound repositories for quick selection */}
      {projectRepos && projectRepos.length > 0 && !config.workingDirectory && (
        <div className="rounded-sm border border-border bg-secondary p-base">
          <p className="text-sm text-low mb-half">
            {t('workflow:step0.boundRepos', { defaultValue: 'Project repositories:' })}
          </p>
          <div className="flex flex-col gap-half">
            {projectRepos.map((repo) => (
              <button
                key={repo.id}
                type="button"
                className="flex items-center gap-base px-base py-half rounded text-left text-sm text-normal hover:bg-primary/50 border border-transparent hover:border-border transition-colors"
                onClick={() => {
                  onChange({ workingDirectory: repo.path });
                  setApiError(null);
                  void checkGitStatus(repo.path);
                }}
              >
                <GitBranchIcon className="size-icon-xs text-low shrink-0" />
                <span className="truncate font-mono">{repo.path}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {config.workingDirectory && (
        <div className="rounded-sm border border-border bg-secondary p-base">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-base">
              {(() => {
                if (isLoading) {
                  return <RefreshIcon className="size-icon-sm animate-spin text-low" />;
                }
                if (gitStatus.isGitRepo) {
                  return <CheckIcon className="size-icon-sm text-success" />;
                }
                return <WarningIcon className="size-icon-sm text-warning" />;
              })()}
              <span className="text-base font-medium text-normal">
                {gitStatus.isGitRepo
                  ? t('step0.status.gitDetected')
                  : t('step0.status.gitNotDetected')}
              </span>
            </div>
            <button
              onClick={() => {
                void checkGitStatus(config.workingDirectory);
              }}
              disabled={isLoading}
              className="text-low hover:text-normal disabled:opacity-50 disabled:cursor-not-allowed"
              aria-label={t('step0.refreshLabel')}
            >
              <RefreshIcon className="size-icon-sm" />
            </button>
          </div>

          {gitStatus.isGitRepo && (
            <div className="mt-base flex flex-col gap-half">
              <div className="flex items-center gap-half text-sm text-low">
                <GitBranchIcon className="size-icon-xs" />
                <span>
                  {t('step0.branchLabel')}:
                  <span className="text-normal font-mono">
                    {gitStatus.currentBranch?.trim()
                      ? gitStatus.currentBranch
                      : t('step0.notAvailable')}
                  </span>
                </span>
              </div>
              {gitStatus.remoteUrl && (
                <div className="text-sm text-low">
                  {t('step0.remoteLabel')}:
                  <span className="font-mono text-normal">
                    {gitStatus.remoteUrl}
                  </span>
                </div>
              )}
              {gitStatus.isDirty && (
                <div className="text-sm text-warning">
                  {t('step0.dirtyLabel')}
                  {gitStatus.uncommittedChanges
                    ? ` (${t('step0.dirtyFiles', { count: gitStatus.uncommittedChanges })})`
                    : ''}
                </div>
              )}
                  
            </div>
          )}

          {!gitStatus.isGitRepo && config.workingDirectory && (
            <div className="mt-base">
              <PrimaryButton
                variant="tertiary"
                onClick={() => {
                  void handleInitGit();
                }}
                disabled={isLoading}
              >
                {t('step0.initGit')}
              </PrimaryButton>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
