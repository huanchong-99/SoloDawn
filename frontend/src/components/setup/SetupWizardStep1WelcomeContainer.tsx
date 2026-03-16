import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import type { UiLanguage } from 'shared/types';

import { useUserSystem } from '@/components/ConfigProvider';
import { getLanguageOptions, uiLanguageToI18nCode } from '@/i18n/languages';
import { SetupWizardStep1Welcome } from './SetupWizardStep1Welcome';

interface SetupWizardStep1WelcomeContainerProps {
  onNext: () => void;
  onSkip: () => void;
}

export function SetupWizardStep1WelcomeContainer({
  onNext,
  onSkip,
}: Readonly<SetupWizardStep1WelcomeContainerProps>) {
  const { t, i18n } = useTranslation(['setup']);
  const { config, updateAndSaveConfig } = useUserSystem();

  const currentLanguage = config?.language || 'BROWSER';

  const languageOptions = getLanguageOptions(
    t('setup:wizard.welcome.languageLabel')
  );

  const handleLanguageChange = useCallback(
    async (value: string) => {
      const uiLang = value as UiLanguage;
      const i18nCode = uiLanguageToI18nCode(uiLang);

      if (i18nCode) {
        await i18n.changeLanguage(i18nCode);
      }

      await updateAndSaveConfig({ language: uiLang });
    },
    [i18n, updateAndSaveConfig]
  );

  return (
    <SetupWizardStep1Welcome
      language={currentLanguage}
      languageOptions={languageOptions}
      onLanguageChange={handleLanguageChange}
      onContinue={onNext}
      onSkip={onSkip}
    />
  );
}
