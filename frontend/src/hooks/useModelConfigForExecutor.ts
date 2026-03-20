import { useMemo, useState, useEffect } from 'react';
import type { BaseCodingAgent, ModelConfig } from 'shared/types';
import { useModelsForCli } from './useCliTypes';

/**
 * Derive cli_type_id from BaseCodingAgent enum value.
 * Convention: SCREAMING_SNAKE → kebab-case, prefixed with "cli-".
 * Special cases handled explicitly.
 */
function executorToCliTypeId(executor: BaseCodingAgent): string {
  // Handle known exceptions to the naming convention
  switch (executor) {
    case 'GEMINI':
      return 'cli-gemini-cli';
    case 'COPILOT':
      return 'cli-copilot';
    default:
      // CLAUDE_CODE → claude-code → cli-claude-code
      return `cli-${executor.toLowerCase().replace(/_/g, '-')}`;
  }
}

const EMPTY_MODELS: ModelConfig[] = [];

interface UseModelConfigForExecutorResult {
  /** Available model configs with API keys for the current executor */
  availableModels: ModelConfig[];
  /** Currently selected model config ID */
  selectedModelConfigId: string | null;
  /** Set the selected model config ID */
  setSelectedModelConfigId: (id: string | null) => void;
  /** Whether models are loading */
  isLoading: boolean;
}

/**
 * Hook to get available model configs for a given executor and manage selection.
 * Only returns models that have API keys configured.
 * Auto-selects the default model when executor changes.
 */
export function useModelConfigForExecutor(
  executor: BaseCodingAgent | null
): UseModelConfigForExecutorResult {
  const cliTypeId = executor ? executorToCliTypeId(executor) : '';
  const { data: allModels, isLoading } = useModelsForCli(cliTypeId);

  // Filter to models with API keys — the API returns ModelConfig objects
  // (useModelsForCli types as CliModel but the endpoint returns ModelConfig)
  const availableModels = useMemo(() => {
    if (!allModels) return EMPTY_MODELS;
    const filtered = (allModels as unknown as ModelConfig[]).filter(
      (m) => m.hasApiKey
    );
    return filtered.length > 0 ? filtered : EMPTY_MODELS;
  }, [allModels]);

  const [selectedModelConfigId, setSelectedModelConfigId] = useState<
    string | null
  >(null);

  // Auto-select default model when executor changes or models load
  useEffect(() => {
    if (availableModels.length === 0) {
      setSelectedModelConfigId(null);
      return;
    }
    const defaultModel = availableModels.find((m) => m.isDefault);
    setSelectedModelConfigId(
      defaultModel?.id ?? availableModels[0]?.id ?? null
    );
  }, [availableModels]);

  return {
    availableModels,
    selectedModelConfigId,
    setSelectedModelConfigId,
    isLoading,
  };
}
