import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { qualityPolicyApi } from '@/lib/api';
import type { QualityGateConfig } from 'shared/types';

// ============================================================================
// Query Keys
// ============================================================================

export const qualityPolicyKeys = {
  all: ['qualityPolicy'] as const,
  default: ['qualityPolicy', 'default'] as const,
  metrics: ['qualityPolicy', 'metrics'] as const,
  project: (projectId: string) => ['qualityPolicy', 'project', projectId] as const,
};

// ============================================================================
// Hooks
// ============================================================================

/** Fetch the resolved quality policy for a specific project (DB-first). */
export function useProjectQualityPolicy(projectId: string | null) {
  return useQuery({
    queryKey: qualityPolicyKeys.project(projectId ?? ''),
    queryFn: () => qualityPolicyApi.getProject(projectId!),
    enabled: !!projectId,
    staleTime: 30_000,
  });
}

/** Fetch the default quality policy (parsed from BUNDLED_CENTRAL_POLICY). */
export function useDefaultQualityPolicy() {
  return useQuery({
    queryKey: qualityPolicyKeys.default,
    queryFn: () => qualityPolicyApi.getDefault(),
    staleTime: 5 * 60 * 1000,
  });
}

/** Fetch the closed-enum metric keys and supported operators for the editor picker. */
export function useQualityMetricKeys() {
  return useQuery({
    queryKey: qualityPolicyKeys.metrics,
    queryFn: () => qualityPolicyApi.getMetrics(),
    staleTime: 60 * 60 * 1000, // metric enum is static between deployments
  });
}

/** Save (PUT) a quality policy for a project; invalidates the project policy cache on success. */
export function useSaveQualityPolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ projectId, config }: { projectId: string; config: QualityGateConfig }) =>
      qualityPolicyApi.putProject(projectId, config),
    onSuccess: (_data, { projectId }) => {
      qc.invalidateQueries({ queryKey: qualityPolicyKeys.project(projectId) });
    },
  });
}

/** Reset (DELETE) the project quality policy; falls back to file/bundled chain on next load. */
export function useResetQualityPolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (projectId: string) => qualityPolicyApi.deleteProject(projectId),
    onSuccess: (_data, projectId) => {
      qc.invalidateQueries({ queryKey: qualityPolicyKeys.project(projectId) });
    },
  });
}
