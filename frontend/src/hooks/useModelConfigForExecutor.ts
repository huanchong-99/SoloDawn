import { useMemo, useState, useEffect, useRef } from 'react';
import type { BaseCodingAgent, ModelConfig } from 'shared/types';
import type { ModelConfig as WorkflowModelConfig } from '@/components/workflow/types';
import { useModelsForCli } from './useCliTypes';

/**
 * Derive cli_type_id from BaseCodingAgent enum value.
 * Convention: SCREAMING_SNAKE → kebab-case, prefixed with "cli-".
 */
function executorToCliTypeId(executor: BaseCodingAgent): string {
  switch (executor) {
    case 'GEMINI':
      return 'cli-gemini-cli';
    case 'COPILOT':
      return 'cli-copilot';
    default:
      return `cli-${executor.toLowerCase().replaceAll('_', '-')}`;
  }
}

/** Extended model info for the dropdown UI */
export interface ModelOption {
  id: string;
  displayName: string;
  subtitle: string | null;
  isCustom: boolean;
  hasApiKey: boolean;
}

const EMPTY_OPTIONS: ModelOption[] = [];

interface UseModelConfigForExecutorResult {
  /** Custom (user-configured) models */
  customModels: ModelOption[];
  /** Official models (only those usable — with API key or global auth) */
  officialModels: ModelOption[];
  /** All available models combined (custom first, then official) */
  allModels: ModelOption[];
  /** Currently selected model config ID */
  selectedModelConfigId: string | null;
  /** Set the selected model config ID */
  setSelectedModelConfigId: (id: string | null) => void;
  /** Whether models are loading */
  isLoading: boolean;
}

/**
 * Hook to get available model configs for a given executor and manage selection.
 * Merges models from two sources:
 * 1. model_config DB table (official models via API)
 * 2. workflow_model_library from config (user-configured third-party models)
 *
 * Official models only appear if they have an API key (i.e., user is logged in
 * or credentials were persisted). User-configured models always appear.
 */
export function useModelConfigForExecutor(
  executor: BaseCodingAgent | null,
  workflowModelLibrary?: WorkflowModelConfig[]
): UseModelConfigForExecutorResult {
  const cliTypeId = executor ? executorToCliTypeId(executor) : '';
  const { data: apiModels, isLoading } = useModelsForCli(cliTypeId);

  const { customModels, officialModels, allModels } = useMemo(() => {
    // User-configured models from workflow_model_library
    const custom: ModelOption[] = (workflowModelLibrary ?? [])
      .filter((wm) => wm.cliTypeId === cliTypeId && wm.apiKey)
      .map((wm) => ({
        id: wm.id,
        displayName: wm.displayName,
        subtitle: [wm.apiType, wm.modelId].filter(Boolean).join(' · ') || null,
        isCustom: true,
        hasApiKey: true,
      }));

    const customIds = new Set(custom.map((m) => m.id));

    // Official models from DB — only show if they have API key (usable)
    const dbModels = apiModels
      ? (apiModels as unknown as ModelConfig[])
      : [];
    // Only show truly official models (isOfficial=true) with API key/login.
    // Non-official DB entries are credential copies of custom models — skip them.
    const official: ModelOption[] = dbModels
      .filter((m) => !customIds.has(m.id) && m.isOfficial && m.hasApiKey)
      .map((m) => ({
        id: m.id,
        displayName: m.displayName,
        subtitle: null,
        isCustom: false,
        hasApiKey: m.hasApiKey,
      }));

    const all = [...custom, ...official];
    return {
      customModels: custom.length > 0 ? custom : EMPTY_OPTIONS,
      officialModels: official.length > 0 ? official : EMPTY_OPTIONS,
      allModels: all.length > 0 ? all : EMPTY_OPTIONS,
    };
  }, [apiModels, workflowModelLibrary, cliTypeId]);

  const [selectedModelConfigId, setSelectedModelConfigId] = useState<
    string | null
  >(null);

  // Auto-select: prefer first custom model, then first official.
  // Only auto-select when executor changes or no selection exists yet —
  // otherwise user's manual selection would be overwritten on every
  // allModels reference change.
  const prevExecutorRef = useRef(executor);
  // Primitive dep derived from model ids to avoid referential-identity churn.
  const allModelsKey = useMemo(
    () => allModels.map((m) => m.id).join('|'),
    [allModels]
  );
  const firstCustomId = customModels[0]?.id ?? null;
  const firstOfficialId = officialModels[0]?.id ?? null;
  const firstAnyId = allModels[0]?.id ?? null;
  const hasModels = allModels.length > 0;
  useEffect(() => {
    const executorChanged = prevExecutorRef.current !== executor;
    prevExecutorRef.current = executor;

    if (!hasModels) {
      setSelectedModelConfigId(null);
      return;
    }
    if (executorChanged || selectedModelConfigId === null) {
      const preferredId = firstCustomId ?? firstOfficialId ?? firstAnyId;
      setSelectedModelConfigId(preferredId);
    }
  }, [
    allModelsKey,
    hasModels,
    firstCustomId,
    firstOfficialId,
    firstAnyId,
    executor,
    selectedModelConfigId,
  ]);

  return {
    customModels,
    officialModels,
    allModels,
    selectedModelConfigId,
    setSelectedModelConfigId,
    isLoading,
  };
}
