import { useQuery } from '@tanstack/react-query';
import { handleApiResponse, makeRequest } from '@/lib/api';
import { isQualityGateAvailable } from '@/lib/apiVersionCompat';
import type {
  QualityRunSummary,
  QualityRunDetail,
  QualityIssueRecord,
} from 'shared/types';

// ============================================================================
// Query Keys
// ============================================================================

export const qualityKeys = {
  all: ['quality'] as const,
  runsForWorkflow: (workflowId: string) =>
    ['quality', 'runs', 'workflow', workflowId] as const,
  runDetail: (runId: string) => ['quality', 'run', runId] as const,
  issuesForRun: (runId: string) => ['quality', 'issues', runId] as const,
  latestForTerminal: (terminalId: string) =>
    ['quality', 'latest', 'terminal', terminalId] as const,
};

// ============================================================================
// Version-tolerant fetch wrapper
// ============================================================================

/**
 * Wraps a quality API call so that a 404 (endpoint missing on older backends)
 * returns `fallback` instead of throwing, enabling graceful degradation.
 */
async function qualityFetchSafe<T>(
  fn: () => Promise<T>,
  fallback: T,
): Promise<T> {
  try {
    return await fn();
  } catch (err: unknown) {
    if (!isQualityGateAvailable(err)) return fallback;
    throw err;
  }
}

// ============================================================================
// Quality API
// ============================================================================

const qualityApi = {
  getRunsForWorkflow: async (
    workflowId: string
  ): Promise<QualityRunSummary[]> => {
    return qualityFetchSafe(async () => {
      const response = await makeRequest(
        `/api/workflows/${encodeURIComponent(workflowId)}/quality/runs`
      );
      return handleApiResponse<QualityRunSummary[]>(response);
    }, []);
  },

  getRunDetail: async (runId: string): Promise<QualityRunDetail | null> => {
    return qualityFetchSafe(async () => {
      const response = await makeRequest(
        `/api/quality/runs/${encodeURIComponent(runId)}`
      );
      return handleApiResponse<QualityRunDetail | null>(response);
    }, null);
  },

  getIssuesForRun: async (runId: string): Promise<QualityIssueRecord[]> => {
    return qualityFetchSafe(async () => {
      const response = await makeRequest(
        `/api/quality/runs/${encodeURIComponent(runId)}/issues`
      );
      return handleApiResponse<QualityIssueRecord[]>(response);
    }, []);
  },

  getLatestForTerminal: async (
    terminalId: string
  ): Promise<QualityRunSummary | null> => {
    return qualityFetchSafe(async () => {
      const response = await makeRequest(
        `/api/terminals/${encodeURIComponent(terminalId)}/quality/latest`
      );
      return handleApiResponse<QualityRunSummary | null>(response);
    }, null);
  },
};

// ============================================================================
// Hooks
// ============================================================================

// Sentinel key segment used when the input id is missing. Prevents all
// disabled consumers from sharing the same `['quality', …, '']` cache
// entry (which would cause cross-caller collisions if react-query ever
// materialized a record for a disabled query). [E19-09]
const DISABLED_KEY = '__disabled__';

export function useQualityRuns(workflowId: string | undefined) {
  return useQuery({
    queryKey: qualityKeys.runsForWorkflow(workflowId ?? DISABLED_KEY),
    queryFn: () => qualityApi.getRunsForWorkflow(workflowId!),
    enabled: !!workflowId,
    staleTime: 5 * 60 * 1000,
  });
}

export function useQualityRunDetail(runId: string | undefined) {
  return useQuery({
    queryKey: qualityKeys.runDetail(runId ?? DISABLED_KEY),
    queryFn: () => qualityApi.getRunDetail(runId!),
    enabled: !!runId,
    staleTime: 5 * 60 * 1000,
  });
}

export function useQualityIssues(runId: string | undefined) {
  return useQuery({
    queryKey: qualityKeys.issuesForRun(runId ?? DISABLED_KEY),
    queryFn: () => qualityApi.getIssuesForRun(runId!),
    enabled: !!runId,
    staleTime: 5 * 60 * 1000,
  });
}

export function useTerminalLatestQuality(terminalId: string | undefined) {
  return useQuery({
    queryKey: qualityKeys.latestForTerminal(terminalId ?? DISABLED_KEY),
    queryFn: () => qualityApi.getLatestForTerminal(terminalId!),
    enabled: !!terminalId,
    staleTime: 30 * 1000,
  });
}
