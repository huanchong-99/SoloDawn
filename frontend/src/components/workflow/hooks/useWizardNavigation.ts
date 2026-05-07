import { useCallback, useEffect, useMemo, useState } from 'react';
import { WIZARD_STEPS, WizardStep } from '../types';

const DEFAULT_STEP_ORDER = WIZARD_STEPS.map((step) => step.step);

export interface UseWizardNavigationOptions {
  initialStep?: WizardStep;
  steps?: WizardStep[];
  /**
   * E11-10: Optional predicate used by goToStep to gate forward jumps. A step
   * is considered reachable only if every step between the current position
   * and the target (exclusive of the target) reports valid. Backward jumps
   * are always allowed. When omitted, forward jumps are unrestricted to
   * preserve legacy behavior for existing callers.
   */
  isStepValid?: (step: WizardStep) => boolean;
}

export interface UseWizardNavigationReturn {
  currentStep: WizardStep;
  stepIndex: number;
  totalSteps: number;
  canGoNext: () => boolean;
  canGoPrevious: () => boolean;
  next: () => void;
  previous: () => void;
  goToStep: (step: WizardStep) => void;
}

/**
 * Provides step navigation state and helpers for the workflow wizard.
 */
export function useWizardNavigation(
  options: UseWizardNavigationOptions = {}
): UseWizardNavigationReturn {
  const { initialStep = WizardStep.Project, steps, isStepValid } = options;
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

  const goToStep = useCallback(
    (step: WizardStep) => {
      const targetIndex = stepOrder.indexOf(step);
      if (targetIndex === -1) {
        return;
      }

      // Backward / same-step jumps are always allowed.
      if (targetIndex <= stepIndex) {
        setCurrentStep(step);
        return;
      }

      // E11-10: For forward jumps, require every intermediate step (from the
      // current position up to but not including the target) to be valid.
      // Without a validator provided, preserve legacy permissive behavior.
      if (isStepValid) {
        for (let i = stepIndex; i < targetIndex; i++) {
          const intermediate = stepOrder[i];
          if (intermediate !== undefined && !isStepValid(intermediate)) {
            return;
          }
        }
      }

      setCurrentStep(step);
    },
    [stepOrder, stepIndex, isStepValid]
  );

  return {
    currentStep,
    stepIndex,
    totalSteps: stepOrder.length,
    canGoNext,
    canGoPrevious,
    next,
    previous,
    goToStep,
  };
}
