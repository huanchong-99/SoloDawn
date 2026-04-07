import type { WizardConfig } from '../types';

/**
 * Validates model configuration presence for step 3.
 */
export function validateStep3Models(config: WizardConfig): Record<string, string> {
  const errors: Record<string, string> = {};

  if (config.models.length === 0) {
    errors.models = 'validation.models.required';
    return errors;
  }

  config.models.forEach((model, index) => {
    // Native models are auto-detected and pre-validated — skip field checks
    if (model.isNative) {
      return;
    }
    const modelKey = model.id.trim() || String(index);
    if (!model.cliTypeId?.trim()) {
      errors[`model-${modelKey}-cli`] = 'validation.terminals.cliRequired';
    }
    if (!model.apiKey?.trim()) {
      errors[`model-${modelKey}-apiKey`] = 'validation.models.apiKeyRequired';
    }
    if (!model.baseUrl?.trim()) {
      errors[`model-${modelKey}-baseUrl`] = 'validation.models.baseUrlRequired';
    }
    if (!model.modelId?.trim()) {
      errors[`model-${modelKey}-modelId`] = 'validation.models.modelIdRequired';
    }
  });

  return errors;
}
