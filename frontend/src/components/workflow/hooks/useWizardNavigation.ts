import { useCallback, useEffect, useMemo, useState } from 'react';
import { WIZARD_STEPS, WizardStep } from '../types';

const DEFAULT_STEP_ORDER = WIZARD_STEPS.map((step) => step.step);

export interface UseWizardNavigationOptions {
  initialStep?: WizardStep;
  steps?: WizardStep[];
}

export interface UseWizardNavigationReturn {
  currentStep: WizardStep;
  stepIndex: number;
  totalSteps: number;
  canGoNext: () => boolean;
  canGoPrevious: () => boolean;
  next: () => void;
  previous: () => void;
}

/**
 * Provides step navigation state and helpers for the workflow wizard.
 */
export function useWizardNavigation(
  options: UseWizardNavigationOptions = {}
): UseWizardNavigationReturn {
  const { initialStep = WizardStep.Project, steps } = options;
  const stepOrder = useMemo(() => {
    if (steps && steps.length > 0) {
      return [...steps];
    }

    return DEFAULT_STEP_ORDER;
  }, [steps]);

  const normalizedInitialStep = stepOrder.includes(initialStep)
    ? initialStep
    : (stepOrder[0] ?? WizardStep.Project);

  const [currentStep, setCurrentStep] = useState<WizardStep>(normalizedInitialStep);

  useEffect(() => {
    if (stepOrder.includes(currentStep)) {
      return;
    }

    setCurrentStep(stepOrder[0] ?? WizardStep.Project);
  }, [currentStep, stepOrder]);

  const stepIndex = useMemo(
    () => stepOrder.indexOf(currentStep),
    [currentStep, stepOrder]
  );

  const canGoNext = useCallback(() => {
    return stepIndex >= 0 && stepIndex < stepOrder.length - 1;
  }, [stepIndex, stepOrder.length]);

  const canGoPrevious = useCallback(() => {
    return stepIndex > 0;
  }, [stepIndex]);

  const next = useCallback(() => {
    if (canGoNext()) {
      setCurrentStep(stepOrder[stepIndex + 1]);
    }
  }, [canGoNext, stepIndex, stepOrder]);

  const previous = useCallback(() => {
    if (canGoPrevious()) {
      setCurrentStep(stepOrder[stepIndex - 1]);
    }
  }, [canGoPrevious, stepIndex, stepOrder]);

  return {
    currentStep,
    stepIndex,
    totalSteps: stepOrder.length,
    canGoNext,
    canGoPrevious,
    next,
    previous,
  };
}
