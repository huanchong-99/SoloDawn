import type { WizardConfig } from '../types';

/**
 * Validates orchestrator and merge settings for step 6.
 */
export function validateStep6Advanced(config: WizardConfig): Record<string, string> {
  const errors: Record<string, string> = {};
  const isModelCompatibleWithCli = (
    modelConfigId: string | undefined,
    cliTypeId: string | undefined
  ): boolean => {
    if (!modelConfigId?.trim() || !cliTypeId?.trim()) {
      return false;
    }
    const model = config.models.find((candidate) => candidate.id === modelConfigId);
    if (!model) {
      return false;
    }
    const boundCliTypeId = model.cliTypeId?.trim();
    if (!boundCliTypeId) {
      return true;
    }
    return boundCliTypeId === cliTypeId;
  };

  if (!config.advanced.orchestrator.modelConfigId.trim()) {
    errors.orchestratorModel = 'validation.advanced.orchestratorModelRequired';
  } else {
    // G25-016: Verify orchestrator model actually exists in configured models
    const orchestratorModel = config.models.find(
      (m) => m.id === config.advanced.orchestrator.modelConfigId
    );
    if (!orchestratorModel) {
      errors.orchestratorModel = 'validation.advanced.orchestratorModelNotFound';
    }
  }

  if (config.advanced.errorTerminal.enabled) {
    if (!config.advanced.errorTerminal.cliTypeId?.trim()) {
      errors.errorTerminalCli = 'validation.terminals.cliRequired';
    }
    if (!config.advanced.errorTerminal.modelConfigId?.trim()) {
      errors.errorTerminalModel = 'validation.terminals.modelRequired';
    } else if (
      !isModelCompatibleWithCli(
        config.advanced.errorTerminal.modelConfigId,
        config.advanced.errorTerminal.cliTypeId
      )
    ) {
      errors.errorTerminalModel = 'validation.terminals.modelCliMismatch';
    }
  }

  if (!config.advanced.mergeTerminal.cliTypeId.trim()) {
    errors.mergeCli = 'validation.advanced.mergeCliRequired';
  }
  if (!config.advanced.mergeTerminal.modelConfigId.trim()) {
    errors.mergeModel = 'validation.advanced.mergeModelRequired';
  } else if (
    !isModelCompatibleWithCli(
      config.advanced.mergeTerminal.modelConfigId,
      config.advanced.mergeTerminal.cliTypeId
    )
  ) {
    errors.mergeModel = 'validation.terminals.modelCliMismatch';
  }

  return errors;
}
