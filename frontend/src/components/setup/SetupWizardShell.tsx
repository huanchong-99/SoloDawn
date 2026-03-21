import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';

import { useUserSystem } from '@/components/ConfigProvider';
import { SetupWizardStep1WelcomeContainer } from './SetupWizardStep1WelcomeContainer';
import { SetupWizardStep2ModelContainer } from './SetupWizardStep2ModelContainer';
import { SetupWizardStep3ProjectContainer } from './SetupWizardStep3ProjectContainer';
import { SetupWizardStep4IntegrationsContainer } from './SetupWizardStep4IntegrationsContainer';
import { SetupWizardStep5Done } from './SetupWizardStep5Done';
import { SetupWizardStepIndicator } from './SetupWizardStepIndicator';

const TOTAL_STEPS = 5;

export function SetupWizardShell() {
  const { t } = useTranslation(['setup']);
  const navigate = useNavigate();
  const { updateAndSaveConfig } = useUserSystem();

  const [currentStep, setCurrentStep] = useState(0);

  const steps = useMemo(
    () => [
      { key: 'welcome', label: t('setup:wizard.steps.welcome') },
      { key: 'model', label: t('setup:wizard.steps.model') },
      { key: 'project', label: t('setup:wizard.steps.project') },
      { key: 'integrations', label: t('setup:wizard.steps.integrations') },
      { key: 'done', label: t('setup:wizard.steps.done') },
    ],
    [t],
  );

  const completeWizard = useCallback(async () => {
    await updateAndSaveConfig({ setup_wizard_completed: true } as Record<string, unknown>);
    navigate('/board');
  }, [updateAndSaveConfig, navigate]);

  const goNext = useCallback(() => {
    setCurrentStep((prev) => Math.min(prev + 1, TOTAL_STEPS - 1));
  }, []);

  const goPrevious = useCallback(() => {
    setCurrentStep((prev) => Math.max(prev - 1, 0));
  }, []);

  const renderStep = () => {
    switch (currentStep) {
      case 0:
        return (
          <SetupWizardStep1WelcomeContainer
            onNext={goNext}
            onSkip={completeWizard}
          />
        );
      case 1:
        return (
          <SetupWizardStep2ModelContainer
            onNext={goNext}
            onBack={goPrevious}
            onSkip={goNext}
          />
        );
      case 2:
        return (
          <SetupWizardStep3ProjectContainer
            onNext={goNext}
            onSkip={goNext}
          />
        );
      case 3:
        return (
          <SetupWizardStep4IntegrationsContainer
            onNext={goNext}
            onSkip={goNext}
          />
        );
      case 4:
        return <SetupWizardStep5Done onGetStarted={completeWizard} />;
      default:
        return null;
    }
  };

  return (
    <div className="new-design h-screen overflow-y-auto bg-primary font-ibm-plex-sans">
      <div className="mx-auto my-double w-full max-w-2xl rounded-lg bg-secondary p-double shadow-lg">
        {/* Step indicator */}
        <div className="mb-double">
          <SetupWizardStepIndicator steps={steps} currentStep={currentStep} />
        </div>

        {/* Step content with scroll for long forms */}
        <div
          key={currentStep}
          className="animate-in fade-in duration-200"
        >
          {renderStep()}
        </div>
      </div>
    </div>
  );
}
