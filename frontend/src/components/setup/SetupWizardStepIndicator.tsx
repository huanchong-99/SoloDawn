import { Check } from '@phosphor-icons/react';

interface StepDefinition {
  key: string;
  label: string;
}

interface SetupWizardStepIndicatorProps {
  steps: Array<StepDefinition>;
  currentStep: number;
}

function getDotStyle(isCurrent: boolean, isCompleted: boolean): string {
  if (isCurrent) return 'bg-brand text-white';
  if (isCompleted) return 'border-2 border-brand text-brand';
  return 'border-2 border-low bg-transparent text-low';
}

function getLabelStyle(isCurrent: boolean, isCompleted: boolean): string {
  if (isCurrent) return 'text-high font-medium';
  if (isCompleted) return 'text-normal';
  return 'text-low';
}

export function SetupWizardStepIndicator({
  steps,
  currentStep,
}: Readonly<SetupWizardStepIndicatorProps>) {
  return (
    <nav aria-label="Setup progress" className="flex items-center justify-center gap-0 w-full">
      {steps.map((step, index) => {
        const isCompleted = index < currentStep;
        const isCurrent = index === currentStep;

        return (
          <div key={step.key} className="flex items-center">
            <div className="flex flex-col items-center gap-1">
              {/* Dot / circle */}
              <div
                className={[
                  'flex items-center justify-center rounded-full transition-colors',
                  'size-6',
                  getDotStyle(isCurrent, isCompleted),
                ].join(' ')}
              >
                {isCompleted ? (
                  <Check weight="bold" className="size-icon-xs" />
                ) : (
                  <span className="text-xs font-medium leading-none">
                    {index + 1}
                  </span>
                )}
              </div>

              {/* Label — hidden on small screens */}
              <span
                className={[
                  'hidden sm:block text-xs whitespace-nowrap transition-colors',
                  getLabelStyle(isCurrent, isCompleted),
                ].join(' ')}
              >
                {step.label}
              </span>
            </div>

            {/* Connector line between dots */}
            {index < steps.length - 1 && (
              <div
                className={[
                  'h-px w-8 sm:w-12 mx-1 sm:mx-2 mb-4 sm:mb-0 transition-colors',
                  index < currentStep ? 'bg-brand' : 'bg-low/30',
                ].join(' ')}
              />
            )}
          </div>
        );
      })}
    </nav>
  );
}
