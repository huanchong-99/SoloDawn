import type { WizardConfig } from '../types';

/**
 * Validates terminal assignments for step 4.
 */
export function validateStep4Terminals(config: WizardConfig): Record<string, string> {
  const errors: Record<string, string> = {};

  if (config.basic.executionMode === 'agent_planned') {
    return errors;
  }

  if (config.terminals.length === 0) {
    errors.terminals = 'validation.terminals.required';
    return errors;
  }

  config.terminals.forEach((terminal, index) => {
    const terminalKey = terminal.id.trim() || String(index);
    const cliTypeId = terminal.cliTypeId.trim();
    const modelConfigId = terminal.modelConfigId.trim();

    if (!cliTypeId) {
      errors[`terminal-${terminalKey}-cli`] = 'validation.terminals.cliRequired';
    }
    if (!modelConfigId) {
      errors[`terminal-${terminalKey}-model`] = 'validation.terminals.modelRequired';
      return;
    }

    const model = config.models.find((candidate) => candidate.id === modelConfigId);
    if (!model) {
      errors[`terminal-${terminalKey}-model`] = 'validation.terminals.modelRequired';
      return;
    }

    const boundCliTypeId = model.cliTypeId?.trim();
    if (boundCliTypeId && cliTypeId && boundCliTypeId !== cliTypeId) {
      errors[`terminal-${terminalKey}-model`] = 'validation.terminals.modelCliMismatch';
    }
  });

  return errors;
}
