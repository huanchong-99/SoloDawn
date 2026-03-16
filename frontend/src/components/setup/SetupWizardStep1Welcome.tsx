import { useTranslation } from 'react-i18next';
import { Globe, CaretDown } from '@phosphor-icons/react';

import { cn } from '@/lib/utils';

export interface SetupWizardStep1WelcomeProps {
  language: string;
  languageOptions: Array<{ value: string; label: string }>;
  onLanguageChange: (value: string) => void;
  onContinue: () => void;
  onSkip: () => void;
}

export function SetupWizardStep1Welcome({
  language,
  languageOptions,
  onLanguageChange,
  onContinue,
  onSkip,
}: Readonly<SetupWizardStep1WelcomeProps>) {
  const { t } = useTranslation(['setup']);

  return (
    <div className="flex flex-col items-center justify-center max-w-lg mx-auto space-y-double">
      <div className="text-center space-y-base">
        <h1 className="text-high text-xl font-medium">
          {t('setup:wizard.welcome.title')}
        </h1>
        <p className="text-low text-base leading-relaxed">
          {t('setup:wizard.welcome.subtitle')}
        </p>
      </div>

      <div className="w-full space-y-half">
        <label className="flex items-center gap-half text-normal text-base">
          <Globe className="size-icon-sm text-low" weight="regular" />
          {t('setup:wizard.welcome.languageLabel')}
        </label>
        <div className="relative w-full">
          <select
            value={language}
            onChange={(e) => onLanguageChange(e.target.value)}
            className={cn(
              'w-full appearance-none rounded border border-border bg-secondary',
              'px-base py-base pr-8 text-base text-normal',
              'focus:outline-none focus:ring-1 focus:ring-brand'
            )}
          >
            {languageOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
          <CaretDown
            className="size-icon-xs absolute right-2 top-1/2 -translate-y-1/2 text-low pointer-events-none"
            weight="bold"
          />
        </div>
      </div>

      <div className="w-full flex flex-col items-center gap-base pt-base">
        <button
          type="button"
          onClick={onContinue}
          className={cn(
            'w-full bg-brand text-white px-double py-base rounded font-medium text-base',
            'hover:opacity-90 transition-opacity',
            'focus:outline-none focus:ring-1 focus:ring-brand focus:ring-offset-1'
          )}
        >
          {t('setup:wizard.welcome.continueButton')}
        </button>
        <button
          type="button"
          onClick={onSkip}
          className="text-low text-sm hover:text-normal underline transition-colors"
        >
          {t('setup:wizard.welcome.skipButton')}
        </button>
      </div>
    </div>
  );
}
