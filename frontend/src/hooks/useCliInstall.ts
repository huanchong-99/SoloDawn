import { useMutation, useQueryClient } from '@tanstack/react-query';
import { handleApiResponse } from '@/lib/api';
import { cliTypesKeys } from './useCliTypes';

// ============================================================================
// Types
// ============================================================================

export interface InstallResult {
  job_id: string;
  status: string;
}

// ============================================================================
// Hook
// ============================================================================

/**
 * Hook for installing/uninstalling a CLI.
 * Triggers cache invalidation of CLI detection queries on completion.
 */
export function useCliInstall() {
  const queryClient = useQueryClient();

  const installMutation = useMutation({
    mutationFn: async (cliTypeId: string): Promise<InstallResult> => {
      const response = await fetch(
        `/api/cli-types/${encodeURIComponent(cliTypeId)}/install`,
        { method: 'POST' }
      );
      return handleApiResponse<InstallResult>(response);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: cliTypesKeys.detection });
    },
  });

  const uninstallMutation = useMutation({
    mutationFn: async (cliTypeId: string): Promise<InstallResult> => {
      const response = await fetch(
        `/api/cli-types/${encodeURIComponent(cliTypeId)}/install`,
        { method: 'DELETE' }
      );
      return handleApiResponse<InstallResult>(response);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: cliTypesKeys.detection });
    },
  });

  return { installMutation, uninstallMutation };
}
