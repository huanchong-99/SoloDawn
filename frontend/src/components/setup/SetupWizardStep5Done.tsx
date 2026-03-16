import { Sparkle } from '@phosphor-icons/react';
import { useTranslation } from 'react-i18next';

interface SetupWizardStep5DoneProps {
  onGetStarted: () => void;
}

export function SetupWizardStep5Done({
  onGetStarted,
}: Readonly<SetupWizardStep5DoneProps>) {
  const { t } = useTranslation(['setup']);

  return (
    <div className="flex flex-col items-center justify-center gap-6 py-double text-center">
      {/* Celebratory icon */}
      <div className="flex items-center justify-center rounded-full bg-brand/10 p-double">
        <Sparkle weight="duotone" className="size-icon-xl text-brand" />
      </div>

      {/* Title */}
      <h2 className="text-xl font-semibold text-high">
        {t('setup:wizard.done.title')}
      </h2>

      {/* Subtitle */}
      <p className="text-base text-normal max-w-sm">
        {t('setup:wizard.done.subtitle')}
      </p>

      {/* Get Started button */}
      <button
        type="button"
        onClick={onGetStarted}
        className="mt-base rounded-md bg-brand px-double py-base text-base font-medium text-white transition-opacity hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-brand focus:ring-offset-2"
      >
        {t('setup:wizard.done.getStartedButton')}
      </button>

      {/* Hint about re-running */}
      <p className="text-sm text-low">
        {t('setup:wizard.done.rerunHint')}
      </p>
    </div>
  );
}
