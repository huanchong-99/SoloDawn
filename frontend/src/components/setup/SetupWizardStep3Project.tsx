import { useTranslation } from 'react-i18next';
import {
  FolderOpen,
  CheckCircle,
  XCircle,
  CircleNotch,
} from '@phosphor-icons/react';

import { cn } from '@/lib/utils';

export interface SetupWizardStep3ProjectProps {
  directory: string;
  onDirectoryChange: (path: string) => void;
  onBrowse: () => void;
  isValid: boolean;
  isChecking: boolean;
  isCreating: boolean;
  validationMessage?: string;
  onNext: () => void;
  onSkip: () => void;
}

export function SetupWizardStep3Project({
  directory,
  onDirectoryChange,
  onBrowse,
  isValid,
  isChecking,
  isCreating,
  validationMessage,
  onNext,
  onSkip,
}: Readonly<SetupWizardStep3ProjectProps>) {
  const { t } = useTranslation(['setup']);

  const canContinue = directory.trim() !== '' && isValid && !isChecking && !isCreating;

  const validationIndicator = (() => {
    if (isChecking) {
      return (
        <>
          <CircleNotch className="size-icon-xs text-low animate-spin" />
          <span className="text-sm text-low">
            {t('setup:wizard.project.checking')}
          </span>
        </>
      );
    }
    if (isValid) {
      return (
        <>
          <CheckCircle className="size-icon-xs text-success" weight="fill" />
          <span className="text-sm text-success">
            {validationMessage ?? t('setup:wizard.project.validGitRepo')}
          </span>
        </>
      );
    }
    return (
      <>
        <XCircle className="size-icon-xs text-error" weight="fill" />
        <span className="text-sm text-error">
          {validationMessage ?? t('setup:wizard.project.notGitRepo')}
        </span>
      </>
    );
  })();

  return (
    <div className="flex flex-col items-center justify-center max-w-lg mx-auto space-y-double">
      <div className="text-center space-y-base">
        <div className="flex items-center justify-center gap-base">
          <h1 className="text-high text-xl font-medium">
            {t('setup:wizard.project.title')}
          </h1>
          <span
            className={cn(
              'rounded border border-border bg-secondary',
              'px-half py-[2px] text-xs text-low font-medium uppercase tracking-wide'
            )}
          >
            {t('setup:wizard.project.optionalBadge')}
          </span>
        </div>
        <p className="text-low text-base leading-relaxed">
          {t('setup:wizard.project.subtitle')}
        </p>
      </div>

      <div className="w-full space-y-half">
        <label className="text-normal text-base">
          {t('setup:wizard.project.directoryLabel')}
        </label>
        <div className="flex gap-half">
          <input
            type="text"
            value={directory}
            onChange={(e) => onDirectoryChange(e.target.value)}
            placeholder={t('setup:wizard.project.directoryPlaceholder')}
            className={cn(
              'flex-1 rounded border border-border bg-secondary',
              'px-base py-base text-base text-normal font-ibm-plex-mono',
              'placeholder:text-low',
              'focus:outline-none focus:ring-1 focus:ring-brand'
            )}
          />
          <button
            type="button"
            onClick={onBrowse}
            className={cn(
              'flex items-center justify-center rounded border border-border bg-secondary',
              'px-base py-base text-low hover:text-normal transition-colors',
              'focus:outline-none focus:ring-1 focus:ring-brand'
            )}
            aria-label={t('setup:wizard.project.browseButton')}
            title={t('setup:wizard.project.browseButton')}
          >
            <FolderOpen className="size-icon-sm" weight="regular" />
          </button>
        </div>

        {directory.trim() !== '' && (
          <div className="flex items-center gap-half mt-half">
            {validationIndicator}
          </div>
        )}
      </div>

      <p className="text-xs text-low text-center leading-relaxed">
        {t('setup:wizard.project.skipHint')}
      </p>

      <div className="w-full flex flex-col items-center gap-base pt-base">
        <button
          type="button"
          onClick={onNext}
          disabled={!canContinue}
          className={cn(
            'w-full bg-brand text-white px-double py-base rounded font-medium text-base',
            'hover:opacity-90 transition-opacity',
            'focus:outline-none focus:ring-1 focus:ring-brand focus:ring-offset-1',
            'disabled:opacity-50 disabled:cursor-not-allowed'
          )}
        >
          {isCreating
            ? t('setup:wizard.project.creating')
            : t('setup:wizard.project.continueButton')}
        </button>
        <button
          type="button"
          onClick={onSkip}
          disabled={isCreating}
          className="text-low text-sm hover:text-normal underline transition-colors disabled:opacity-50"
        >
          {t('setup:wizard.project.skipButton')}
        </button>
      </div>
    </div>
  );
}
