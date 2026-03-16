import { useState, useCallback, useRef, useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import { FolderPickerDialog } from '@/components/dialogs/shared/FolderPickerDialog';
import { useProjectMutations } from '@/hooks/useProjectMutations';
import { SetupWizardStep3Project } from './SetupWizardStep3Project';

interface SetupWizardStep3ProjectContainerProps {
  onNext: () => void;
  onSkip: () => void;
}

interface GitCheckResult {
  isGitRepo: boolean;
  errorMessage?: string;
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null;

async function checkGitRepo(path: string): Promise<GitCheckResult> {
  try {
    const response = await fetch('/api/git/status', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path }),
    });

    const data = (await response.json().catch(() => null)) as unknown;

    if (response.ok) {
      const apiResponse = data as { success?: boolean; data?: unknown };
      if (apiResponse?.success && isRecord(apiResponse.data)) {
        return { isGitRepo: apiResponse.data.isGitRepo === true };
      }
    }

    if (isRecord(data) && typeof data.error === 'string' && data.error.trim()) {
      return { isGitRepo: false, errorMessage: data.error };
    }

    return { isGitRepo: false };
  } catch {
    return { isGitRepo: false, errorMessage: 'networkError' };
  }
}

function deriveProjectName(directory: string): string {
  // Strip trailing slashes without regex (avoids S5852 backtracking hotspot)
  let trimmed = directory;
  while (trimmed.endsWith('/') || trimmed.endsWith('\\')) {
    trimmed = trimmed.slice(0, -1);
  }
  const sepIndex = Math.max(trimmed.lastIndexOf('/'), trimmed.lastIndexOf('\\'));
  const last = sepIndex >= 0 ? trimmed.slice(sepIndex + 1) : trimmed;
  return last || 'My Project';
}

export function SetupWizardStep3ProjectContainer({
  onNext,
  onSkip,
}: Readonly<SetupWizardStep3ProjectContainerProps>) {
  const { t } = useTranslation(['setup']);

  const [directory, setDirectory] = useState('');
  const [isValid, setIsValid] = useState(false);
  const [isChecking, setIsChecking] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [validationMessage, setValidationMessage] = useState<string | undefined>(
    undefined
  );

  const debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const { createProject } = useProjectMutations({
    onCreateSuccess: () => {
      setIsCreating(false);
      onNext();
    },
    onCreateError: () => {
      setIsCreating(false);
    },
  });

  useEffect(() => {
    return () => {
      if (debounceTimerRef.current) {
        clearTimeout(debounceTimerRef.current);
      }
    };
  }, []);

  const validateDirectory = useCallback(
    async (path: string) => {
      if (!path.trim()) {
        setIsValid(false);
        setValidationMessage(undefined);
        return;
      }

      setIsChecking(true);
      setValidationMessage(undefined);

      const result = await checkGitRepo(path);

      setIsChecking(false);

      if (result.isGitRepo) {
        setIsValid(true);
        setValidationMessage(t('setup:wizard.project.validGitRepo'));
      } else if (result.errorMessage === 'networkError') {
        setIsValid(false);
        setValidationMessage(t('setup:wizard.project.checkFailed'));
      } else if (result.errorMessage) {
        setIsValid(false);
        setValidationMessage(result.errorMessage);
      } else {
        setIsValid(false);
        setValidationMessage(t('setup:wizard.project.notGitRepo'));
      }
    },
    [t]
  );

  const handleDirectoryChange = useCallback(
    (path: string) => {
      setDirectory(path);
      setIsValid(false);
      setValidationMessage(undefined);

      if (debounceTimerRef.current) {
        clearTimeout(debounceTimerRef.current);
      }

      if (path.trim()) {
        debounceTimerRef.current = setTimeout(() => {
          validateDirectory(path).catch(() => { /* handled internally */ });
        }, 500);
      }
    },
    [validateDirectory]
  );

  const handleBrowse = useCallback(async () => {
    try {
      const selectedPath = await FolderPickerDialog.show({
        value: directory,
        title: t('setup:wizard.project.browseButton'),
      });

      if (!selectedPath) return;

      setDirectory(selectedPath);
      validateDirectory(selectedPath).catch(() => { /* handled internally */ });
    } catch {
      // User cancelled the dialog
    }
  }, [directory, t, validateDirectory]);

  const handleNext = useCallback(() => {
    if (!directory.trim() || !isValid || isCreating) return;

    setIsCreating(true);

    createProject.mutate({
      name: deriveProjectName(directory),
      repositories: [
        {
          displayName: deriveProjectName(directory),
          gitRepoPath: directory,
        },
      ],
    });
  }, [directory, isValid, isCreating, createProject]);

  return (
    <SetupWizardStep3Project
      directory={directory}
      onDirectoryChange={handleDirectoryChange}
      onBrowse={handleBrowse}
      isValid={isValid}
      isChecking={isChecking}
      isCreating={isCreating}
      validationMessage={validationMessage}
      onNext={handleNext}
      onSkip={onSkip}
    />
  );
}
