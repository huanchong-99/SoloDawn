import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { StepIndicator } from './StepIndicator';
import {
  WizardStep,
  WizardConfig,
  getDefaultWizardConfig,
  getVisibleWizardStepIds,
  getVisibleWizardSteps,
} from './types';
import type { ModelConfig } from './types';
import { useWizardNavigation } from './hooks/useWizardNavigation';
import { useWizardValidation } from './hooks/useWizardValidation';
import { useTranslation } from 'react-i18next';
import { useUserSystem } from '@/components/ConfigProvider';
import {
  Step0Project,
  Step1Basic,
  Step2Tasks,
  Step3Models,
  Step4Terminals,
  Step5Commands,
  Step6Advanced,
} from './steps';

interface WorkflowWizardProps {
  projectId?: string | null;
  onComplete: (config: WizardConfig) => void | Promise<void>;
  onCancel: () => void;
  onError?: (error: Error) => void;
}

/**
 * Renders the multi-step workflow wizard with navigation and validation.
 */
export function WorkflowWizard({
  projectId,
  onComplete,
  onCancel,
  onError,
}: Readonly<WorkflowWizardProps>) {
  const [state, setState] = useState<{
    config: WizardConfig;
    isSubmitting: boolean;
  }>({
    config: getDefaultWizardConfig(),
    isSubmitting: false,
  });
  const [completedSteps, setCompletedSteps] = useState<WizardStep[]>([]);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const { config, isSubmitting } = state;

  // Keep a ref to the latest config so async handlers never capture a stale snapshot
  const configRef = useRef(config);
  configRef.current = config;

  const visibleSteps = useMemo(
    () => getVisibleWizardSteps(config.basic.executionMode),
    [config.basic.executionMode]
  );
  const visibleStepIds = useMemo(
    () => getVisibleWizardStepIds(config.basic.executionMode),
    [config.basic.executionMode]
  );
  // G25-015: When switching from agent_planned back to diy, auto-initialize
  // skipped step data (tasks / terminals) if they are empty.
  useEffect(() => {
    if (config.basic.executionMode !== 'diy') {
      return;
    }

    setState((prev) => {
      const needsTasks =
        prev.config.tasks.length === 0 && prev.config.basic.taskCount > 0;
      const needsTerminals = prev.config.terminals.length === 0;

      if (!needsTasks && !needsTerminals) {
        return prev;
      }

      const defaults = getDefaultWizardConfig();
      return {
        ...prev,
        config: {
          ...prev.config,
          tasks: needsTasks
            ? Array.from({ length: prev.config.basic.taskCount }, (_, _i) => ({
                id: crypto.randomUUID(),
                name: '',
                description: '',
                branch: '',
                terminalCount: 1,
              }))
            : prev.config.tasks,
          terminals: needsTerminals ? defaults.terminals : prev.config.terminals,
        },
      };
    });
  }, [config.basic.executionMode]);

  const navigation = useWizardNavigation({ steps: visibleStepIds });
  const { currentStep } = navigation;
  const validation = useWizardValidation(currentStep);
  const { errors } = validation;
  const { t } = useTranslation('workflow');
  const { config: userConfig, updateAndSaveConfig } = useUserSystem();

  const globalModelLibrary = useMemo<ModelConfig[]>(() => {
    const rawLibrary = (userConfig as { workflow_model_library?: unknown } | null)
      ?.workflow_model_library;
    if (!Array.isArray(rawLibrary)) {
      return [];
    }

    return rawLibrary
      .filter((item): item is ModelConfig => {
        if (typeof item !== 'object' || item === null) {
          return false;
        }
        const candidate = item as Record<string, unknown>;
        const cliTypeId = candidate.cliTypeId;
        return (
          typeof candidate.id === 'string' &&
          typeof candidate.displayName === 'string' &&
          (cliTypeId === undefined || typeof cliTypeId === 'string') &&
          typeof candidate.apiType === 'string' &&
          typeof candidate.baseUrl === 'string' &&
          typeof candidate.apiKey === 'string' &&
          typeof candidate.modelId === 'string' &&
          typeof candidate.isVerified === 'boolean'
        );
      })
      .map((item) => ({ ...item }));
  }, [userConfig]);

  useEffect(() => {
    if (globalModelLibrary.length === 0) {
      return;
    }

    setState((prevState) => {
      if (prevState.config.models.length > 0) {
        return prevState;
      }

      return {
        ...prevState,
        config: {
          ...prevState.config,
          models: globalModelLibrary,
        },
      };
    });
  }, [globalModelLibrary]);

  const persistWorkflowModelLibrary = useCallback(
    async (models: ModelConfig[]) => {
      if (!userConfig) {
        return;
      }

      try {
        await updateAndSaveConfig({
          workflow_model_library: models,
        } as Partial<typeof userConfig>);
      } catch (error) {
        console.error('Failed to persist workflow model library', error);
      }
    },
    [userConfig, updateAndSaveConfig]
  );

  const handleNext = () => {
    const newErrors = validation.validate(configRef.current);
    if (Object.keys(newErrors).length > 0) {
      return;
    }

    // Mark current step as completed and move to next step
    const newCompletedSteps = [...completedSteps];
    if (!newCompletedSteps.includes(currentStep)) {
      newCompletedSteps.push(currentStep);
    }
    setCompletedSteps(newCompletedSteps);

    // G25-018: Clear errors BEFORE navigation to prevent stale error flash on the next step
    validation.clearErrors();
    if (navigation.canGoNext()) {
      navigation.next();
    }
  };

  const handleBack = () => {
    if (navigation.canGoPrevious()) {
      navigation.previous();
      validation.clearErrors();
    }
  };

  const handleSubmit = async () => {
    const latestConfig = configRef.current;
    const newErrors = validation.validate(latestConfig);
    if (Object.keys(newErrors).length > 0) {
      return;
    }

    setState((prev) => ({ ...prev, isSubmitting: true }));
    setSubmitError(null);

    try {
      await Promise.resolve(onComplete(latestConfig));
      // Reset submitting state after successful completion
      setState((prev) => ({ ...prev, isSubmitting: false }));
    } catch (error) {
      const errorObj =
        error instanceof Error
          ? error
          : new Error(t('wizard.errors.submitUnknown'));
      onError?.(errorObj);
      setSubmitError(errorObj.message);
      setState((prev) => ({ ...prev, isSubmitting: false }));
    }
  };

  const handleCancel = () => {
    onCancel();
  };

  // G25-012: Wrap in useCallback to stabilize reference for Step4Terminals useEffect deps
  const handleUpdateConfig = useCallback((updates: Partial<WizardConfig>) => {
    setState((prevState) => ({
      ...prevState,
      config: {
        ...prevState.config,
        ...updates,
      },
    }));
  }, []);

  const renderStep = () => {
    switch (currentStep) {
      case WizardStep.Project:
        return (
          <Step0Project
            config={config.project}
            projectId={projectId ?? undefined}
            errors={errors}
            onError={onError}
            onChange={(updates) => {
              setState((prevState) => ({
                ...prevState,
                config: {
                  ...prevState.config,
                  project: { ...prevState.config.project, ...updates },
                },
              }));
            }}
          />
        );
      case WizardStep.Basic:
        return (
          <Step1Basic
            config={config.basic}
            errors={errors}
            onChange={(updates) => {
              setState((prevState) => ({
                ...prevState,
                config: {
                  ...prevState.config,
                  basic: { ...prevState.config.basic, ...updates },
                },
              }));
            }}
          />
        );
      case WizardStep.Tasks:
        return (
          <Step2Tasks
            config={config.tasks}
            taskCount={config.basic.taskCount}
            onChange={(tasks) => {
              handleUpdateConfig({ tasks });
            }}
            errors={errors}
          />
        );
      case WizardStep.Models:
        return (
          <Step3Models
            config={config}
            onUpdate={(updates) => {
              handleUpdateConfig(updates);
              if (updates.models) {
                persistWorkflowModelLibrary(updates.models).catch((error) => {
                  console.error(
                    'Unexpected failure while persisting workflow model library',
                    error
                  );
                });
              }
            }}
          />
        );
      case WizardStep.Terminals:
        return (
          <Step4Terminals
            config={config}
            errors={errors}
            onUpdate={handleUpdateConfig}
            onError={onError}
          />
        );
      case WizardStep.Commands:
        return (
          <Step5Commands
            config={config.commands}
            errors={errors}
            onUpdate={(updates) => {
              setState((prevState) => ({
                ...prevState,
                config: {
                  ...prevState.config,
                  commands: { ...prevState.config.commands, ...updates },
                },
              }));
            }}
            onError={onError}
          />
        );
      case WizardStep.Advanced:
        return (
          <Step6Advanced
            config={config}
            errors={errors}
            onUpdate={handleUpdateConfig}
          />
        );
      default:
        return null;
    }
  };

  const getButtonText = () => {
    if (currentStep === WizardStep.Advanced) {
      return isSubmitting
        ? t('wizard.buttons.submitting')
        : t('wizard.buttons.submit');
    }
    return t('wizard.buttons.next');
  };

  const getBackButtonText = () => {
    if (currentStep === WizardStep.Project) {
      return t('wizard.buttons.cancel');
    }
    return t('wizard.buttons.back');
  };

  const handlePrimaryButtonClick = () => {
    if (currentStep === WizardStep.Advanced) {
      handleSubmit().catch((error) => {
        const errorObj =
          error instanceof Error
            ? error
            : new Error(t('wizard.errors.submitUnknown'));
        onError?.(errorObj);
        setSubmitError(errorObj.message);
        setState((prevState) => ({ ...prevState, isSubmitting: false }));
      });
    } else {
      handleNext();
    }
  };

  return (
    <Card className="w-full max-w-4xl mx-auto bg-panel text-high max-h-[calc(100vh-8rem)] min-h-0 flex flex-col">
      <CardHeader className="shrink-0">
        <CardTitle className="text-xl">{t('wizard.title')}</CardTitle>
      </CardHeader>
      <CardContent className="px-base flex min-h-0 flex-1 flex-col">
        <div className="shrink-0">
          <StepIndicator
            currentStep={currentStep}
            completedSteps={completedSteps.filter((step) => visibleStepIds.includes(step))}
            steps={visibleSteps}
          />
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto mb-6">
          {renderStep()}
        </div>

        {/* Navigation Buttons */}
        <div className="shrink-0 flex justify-between items-center pt-4 border-t border-border">
          <div>
            {currentStep > WizardStep.Project ? (
              <button
                type="button"
                onClick={handleBack}
                disabled={isSubmitting}
                className={cn(
                  'px-4 py-2 rounded border text-sm',
                  'bg-secondary text-low hover:text-normal',
                  'disabled:opacity-50 disabled:cursor-not-allowed'
                )}
              >
                {getBackButtonText()}
              </button>
            ) : null}
          </div>

          <div className="flex gap-3">
            {currentStep === WizardStep.Project ? (
              <button
                type="button"
                onClick={handleCancel}
                className={cn(
                  'px-4 py-2 rounded border text-sm',
                  'bg-secondary text-low hover:text-normal'
                )}
              >
                {t('wizard.buttons.cancel')}
              </button>
            ) : null}

            <button
              type="button"
              onClick={handlePrimaryButtonClick}
              disabled={isSubmitting}
              className={cn(
                'px-4 py-2 rounded border text-sm',
                'bg-brand text-white hover:opacity-90',
                'disabled:opacity-50 disabled:cursor-not-allowed'
              )}
            >
              {getButtonText()}
            </button>
          </div>
        </div>

        {/* Error Display */}
        {(Object.keys(errors).length > 0 || submitError) && (
          <div className="mt-4 p-3 bg-error/10 border border-error rounded">
            {submitError ? (
              <div>
                <p className="text-sm text-error font-medium">{t('wizard.errors.submitFailedTitle')}</p>
                <p className="mt-2 text-sm text-error">{submitError}</p>
              </div>
            ) : (
              <div>
                <p className="text-sm text-error font-medium">{t('wizard.errors.validationTitle')}</p>
                <ul className="mt-2 text-sm text-error list-disc list-inside">
                  {Object.entries(errors).map(([key, error]) => (
                    <li key={`error-${key}`}>{t(error)}</li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
