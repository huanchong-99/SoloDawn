import type { WizardConfig } from '../types';

/**
 * Validates basic workflow metadata and task count.
 */
export function validateStep1Basic(config: WizardConfig): Record<string, string> {
  const errors: Record<string, string> = {};
  const isAgentPlanned = config.basic.executionMode === 'agent_planned';

  if (!config.basic.name.trim()) {
    errors.name = 'validation.basic.nameRequired';
  }

  if (isAgentPlanned && !config.basic.initialGoal?.trim()) {
    errors.initialGoal = 'validation.basic.initialGoalRequired';
  }

  if (!isAgentPlanned && (config.basic.taskCount <= 0 || config.basic.taskCount > 10)) {
    errors.taskCount = 'validation.basic.taskCountMin';
  }

  return errors;
}
