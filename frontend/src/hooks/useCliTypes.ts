import { useQuery, UseQueryResult } from '@tanstack/react-query';

import {
  useErrorNotification,
  type ErrorNotificationOptions,
} from './useErrorNotification';

// ============================================================================
// CLI Types
// ============================================================================

export interface CliType {
  id: string;
  name: string;
  displayName: string;
  description: string;
  executableCommand?: string;
  versionCheckCommand?: string;
  website?: string;
  documentationUrl?: string;
  isInstalled?: boolean;
  installedVersion?: string;
}

export interface CliModel {
  id: string;
  cliTypeId: string;
  modelId: string;
  displayName: string;
  provider: 'anthropic' | 'google' | 'openai' | 'other';
  apiType: 'anthropic' | 'anthropic-compatible' | 'google' | 'openai' | 'openai-compatible';
  requiresConfig: boolean;
  configSchema?: Record<string, unknown>;
}

export interface CliDetectionResult {
  cliTypeId: string;
  isInstalled: boolean;
  version?: string;
  path?: string;
  error?: string;
}

// ============================================================================
// Query Keys
// ============================================================================

export const cliTypesKeys = {
  all: ['cliTypes'] as const,
  models: (cliTypeId: string) => ['cliTypes', 'models', cliTypeId] as const,
  detection: ['cliTypes', 'detection'] as const,
};

// ============================================================================
// CLI Types API
// ============================================================================

const cliTypesApi = {
  /**
   * Get all available CLI types
   */
  getAll: async (): Promise<CliType[]> => {
    const response = await fetch('/api/cli_types');
    if (!response.ok) {
      throw new Error(`Failed to fetch CLI types: ${response.status}`);
    }
    return response.json();
  },

  /**
   * Detect which CLIs are installed on the system
   */
  detectInstallation: async (): Promise<CliDetectionResult[]> => {
    const response = await fetch('/api/cli_types/detect');
    if (!response.ok) {
      throw new Error(`Failed to detect CLI installations: ${response.status}`);
    }
    return response.json();
  },

  /**
   * Get available models for a specific CLI type
   */
  getModels: async (cliTypeId: string): Promise<CliModel[]> => {
    const response = await fetch(`/api/cli_types/${encodeURIComponent(cliTypeId)}/models`);
    if (!response.ok) {
      throw new Error(`Failed to fetch models: ${response.status}`);
    }
    return response.json();
  },
};

// ============================================================================
// Hooks
// ============================================================================

/**
 * Hook to fetch all available CLI types
 * @returns Query result with CLI types array
 */
export function useCliTypes(
  options: ErrorNotificationOptions = {}
): UseQueryResult<CliType[], Error> {
  const { notifyError } = useErrorNotification({
    ...options,
    context: options.context ?? 'CliTypes',
  });

  return useQuery({
    queryKey: cliTypesKeys.all,
    queryFn: async () => {
      try {
        return await cliTypesApi.getAll();
      } catch (error) {
        notifyError(error as Error);
        throw error;
      }
    },
    staleTime: 1000 * 60 * 60, // 1 hour - CLI types don't change often
  });
}

/**
 * Hook to detect CLI installation status
 * @returns Query result with CLI detection results
 */
export function useCliDetection(
  options: ErrorNotificationOptions = {}
): UseQueryResult<CliDetectionResult[], Error> {
  const { notifyError } = useErrorNotification({
    ...options,
    context: options.context ?? 'CliDetection',
  });

  return useQuery({
    queryKey: cliTypesKeys.detection,
    queryFn: async () => {
      try {
        return await cliTypesApi.detectInstallation();
      } catch (error) {
        notifyError(error as Error);
        throw error;
      }
    },
    staleTime: 1000 * 60 * 5, // 5 minutes - installation status can change
    refetchOnWindowFocus: true, // Re-check when user returns to tab
  });
}

/**
 * Hook to fetch models available for a specific CLI type
 * @param cliTypeId - The CLI type ID to fetch models for
 * @returns Query result with models array
 */
export function useModelsForCli(
  cliTypeId: string,
  options: ErrorNotificationOptions = {}
): UseQueryResult<CliModel[], Error> {
  const { notifyError } = useErrorNotification({
    ...options,
    context: options.context ?? 'CliModels',
  });

  return useQuery({
    queryKey: cliTypesKeys.models(cliTypeId),
    queryFn: async () => {
      try {
        return await cliTypesApi.getModels(cliTypeId);
      } catch (error) {
        notifyError(error as Error);
        throw error;
      }
    },
    enabled: !!cliTypeId,
    staleTime: 1000 * 60 * 30, // 30 minutes - available models don't change often
  });
}
