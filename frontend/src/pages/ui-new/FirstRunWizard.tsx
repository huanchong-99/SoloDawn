import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  CheckCircle,
  Warning,
  ArrowRight,
  ArrowLeft,
  SkipForward,
  Package,
  Key,
  Plugs,
  Rocket,
} from '@phosphor-icons/react';

import { useFirstRun } from '@/hooks/useFirstRun';
import { handleApiResponse } from '@/lib/api';
import { SetupWizardStepIndicator } from '@/components/setup/SetupWizardStepIndicator';

// ============================================================================
// Types
// ============================================================================

interface CliStatus {
  name: string;
  display_name: string;
  installed: boolean;
  version?: string;
}

interface RuntimeStatus {
  node_version?: string;
  git_version?: string;
  gh_version?: string;
  cli_statuses: CliStatus[];
}

// ============================================================================
// Data fetching
// ============================================================================

function useRuntimeStatus() {
  return useQuery<RuntimeStatus>({
    queryKey: ['runtime', 'status'],
    queryFn: async () => {
      // Fetch CLI detection status and compose runtime info
      const response = await fetch('/api/cli-types/detect');
      const data = await handleApiResponse<Record<string, unknown>[]>(response);

      const cliStatuses: CliStatus[] = (data || []).map(
        (item: Record<string, unknown>) => ({
          name: (item.name as string) || '',
          display_name: (item.display_name as string) || (item.name as string) || '',
          installed: Boolean(item.detected),
          version: item.version as string | undefined,
        }),
      );

      return {
        cli_statuses: cliStatuses,
      };
    },
    staleTime: 30_000,
  });
}

// ============================================================================
// Step Components
// ============================================================================

function WelcomeStep({ onNext, onSkip }: Readonly<{ onNext: () => void; onSkip: () => void }>) {
  const { t } = useTranslation(['setup']);

  return (
    <div className="space-y-double">
      <div className="text-center">
        <Rocket className="mx-auto mb-base size-icon-xl text-brand" weight="duotone" />
        <h2 className="text-xl font-semibold text-high">
          {t('setup:firstRun.welcome.title', 'Welcome to GitCortex')}
        </h2>
        <p className="mt-half text-base text-normal">
          {t(
            'setup:firstRun.welcome.subtitle',
            'Your installation is ready. Let\'s verify everything is working.',
          )}
        </p>
      </div>

      <div className="rounded bg-panel p-base text-sm text-normal">
        <p>
          {t(
            'setup:firstRun.welcome.description',
            'GitCortex has been installed with all required tools bundled. This wizard will help you verify the installation and configure API keys for the AI CLIs you want to use.',
          )}
        </p>
      </div>

      <div className="flex justify-between">
        <button
          type="button"
          onClick={onSkip}
          className="flex items-center gap-half rounded border px-base py-half text-sm text-low hover:text-normal"
        >
          <SkipForward className="size-icon-sm" />
          {t('setup:firstRun.skipButton', 'Skip, configure later')}
        </button>
        <button
          type="button"
          onClick={onNext}
          className="flex items-center gap-half rounded bg-brand px-base py-half text-sm text-white"
        >
          {t('setup:firstRun.continueButton', 'Continue')}
          <ArrowRight className="size-icon-sm" />
        </button>
      </div>
    </div>
  );
}

function EnvironmentStep({ onNext, onBack }: Readonly<{ onNext: () => void; onBack: () => void }>) {
  const { t } = useTranslation(['setup']);
  const { data: runtime, isLoading } = useRuntimeStatus();

  return (
    <div className="space-y-double">
      <div>
        <Package className="mb-half size-icon-lg text-brand" weight="duotone" />
        <h2 className="text-xl font-semibold text-high">
          {t('setup:firstRun.environment.title', 'Environment Status')}
        </h2>
        <p className="mt-half text-sm text-normal">
          {t(
            'setup:firstRun.environment.subtitle',
            'Pre-installed CLI tools and their status.',
          )}
        </p>
      </div>

      {isLoading ? (
        <div className="py-double text-center text-sm text-low">
          {t('setup:firstRun.environment.loading', 'Checking environment...')}
        </div>
      ) : (
        <div className="space-y-half">
          {runtime?.cli_statuses.map((cli) => (
            <div
              key={cli.name}
              className="flex items-center justify-between rounded border px-base py-half"
            >
              <span className="text-sm text-high">{cli.display_name}</span>
              <div className="flex items-center gap-half">
                {cli.installed ? (
                  <>
                    <span className="text-xs text-low">{cli.version}</span>
                    <CheckCircle className="size-icon-sm text-success" weight="fill" />
                  </>
                ) : (
                  <Warning className="size-icon-sm text-low" />
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      <div className="flex justify-between">
        <button
          type="button"
          onClick={onBack}
          className="flex items-center gap-half rounded border px-base py-half text-sm text-low hover:text-normal"
        >
          <ArrowLeft className="size-icon-sm" />
          {t('setup:firstRun.backButton', 'Back')}
        </button>
        <button
          type="button"
          onClick={onNext}
          className="flex items-center gap-half rounded bg-brand px-base py-half text-sm text-white"
        >
          {t('setup:firstRun.continueButton', 'Continue')}
          <ArrowRight className="size-icon-sm" />
        </button>
      </div>
    </div>
  );
}

function ApiKeyStep({ onNext, onBack }: Readonly<{ onNext: () => void; onBack: () => void }>) {
  const { t } = useTranslation(['setup']);

  return (
    <div className="space-y-double">
      <div>
        <Key className="mb-half size-icon-lg text-brand" weight="duotone" />
        <h2 className="text-xl font-semibold text-high">
          {t('setup:firstRun.apiKey.title', 'Configure API Keys')}
        </h2>
        <p className="mt-half text-sm text-normal">
          {t(
            'setup:firstRun.apiKey.subtitle',
            'API keys are configured per-model in Settings > Models. You can set them up now or later.',
          )}
        </p>
      </div>

      <div className="rounded bg-panel p-base text-sm text-normal">
        <p>
          {t(
            'setup:firstRun.apiKey.hint',
            'Each AI CLI uses its own API key. Go to Settings > Models after setup to configure API keys for the providers you want to use (Anthropic, OpenAI, Google, etc.).',
          )}
        </p>
      </div>

      <div className="flex justify-between">
        <button
          type="button"
          onClick={onBack}
          className="flex items-center gap-half rounded border px-base py-half text-sm text-low hover:text-normal"
        >
          <ArrowLeft className="size-icon-sm" />
          {t('setup:firstRun.backButton', 'Back')}
        </button>
        <button
          type="button"
          onClick={onNext}
          className="flex items-center gap-half rounded bg-brand px-base py-half text-sm text-white"
        >
          {t('setup:firstRun.continueButton', 'Continue')}
          <ArrowRight className="size-icon-sm" />
        </button>
      </div>
    </div>
  );
}

function DoneStep({ onFinish }: Readonly<{ onFinish: () => void }>) {
  const { t } = useTranslation(['setup']);

  return (
    <div className="space-y-double text-center">
      <div>
        <Plugs className="mx-auto mb-base size-icon-xl text-success" weight="duotone" />
        <h2 className="text-xl font-semibold text-high">
          {t('setup:firstRun.done.title', 'All Set!')}
        </h2>
        <p className="mt-half text-sm text-normal">
          {t(
            'setup:firstRun.done.subtitle',
            'GitCortex is ready to use. You can always revisit settings to configure API keys, update CLI tools, or change the npm mirror.',
          )}
        </p>
      </div>

      <button
        type="button"
        onClick={onFinish}
        className="mx-auto flex items-center gap-half rounded bg-brand px-double py-half text-sm text-white"
      >
        <Rocket className="size-icon-sm" />
        {t('setup:firstRun.done.startButton', 'Get Started')}
      </button>
    </div>
  );
}

// ============================================================================
// Main Wizard
// ============================================================================

const TOTAL_STEPS = 4;

export function FirstRunWizard() {
  const { t } = useTranslation(['setup']);
  const navigate = useNavigate();
  const { completeFirstRun } = useFirstRun();

  const [currentStep, setCurrentStep] = useState(0);

  const steps = useMemo(
    () => [
      { key: 'welcome', label: t('setup:firstRun.steps.welcome', 'Welcome') },
      { key: 'environment', label: t('setup:firstRun.steps.environment', 'Environment') },
      { key: 'apikeys', label: t('setup:firstRun.steps.apikeys', 'API Keys') },
      { key: 'done', label: t('setup:firstRun.steps.done', 'Done') },
    ],
    [t],
  );

  const finish = useCallback(async () => {
    await completeFirstRun();
    navigate('/board');
  }, [completeFirstRun, navigate]);

  const goNext = useCallback(() => {
    setCurrentStep((prev) => Math.min(prev + 1, TOTAL_STEPS - 1));
  }, []);

  const goBack = useCallback(() => {
    setCurrentStep((prev) => Math.max(prev - 1, 0));
  }, []);

  const renderStep = () => {
    switch (currentStep) {
      case 0:
        return <WelcomeStep onNext={goNext} onSkip={finish} />;
      case 1:
        return <EnvironmentStep onNext={goNext} onBack={goBack} />;
      case 2:
        return <ApiKeyStep onNext={goNext} onBack={goBack} />;
      case 3:
        return <DoneStep onFinish={finish} />;
      default:
        return null;
    }
  };

  return (
    <div className="new-design h-screen overflow-y-auto bg-primary font-ibm-plex-sans">
      <div className="mx-auto my-double w-full max-w-2xl rounded-lg bg-secondary p-double shadow-lg">
        <div className="mb-double">
          <SetupWizardStepIndicator steps={steps} currentStep={currentStep} />
        </div>
        <div key={currentStep} className="animate-in fade-in duration-200">
          {renderStep()}
        </div>
      </div>
    </div>
  );
}
