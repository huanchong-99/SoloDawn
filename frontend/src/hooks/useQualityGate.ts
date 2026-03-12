import { useQuery } from '@tanstack/react-query';
import { handleApiResponse } from '@/lib/api';
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
// Quality API
// ============================================================================

const qualityApi = {
  getRunsForWorkflow: async (
    workflowId: string
  ): Promise<QualityRunSummary[]> => {
    const response = await fetch(
      `/api/workflows/${encodeURIComponent(workflowId)}/quality/runs`
    );
    return handleApiResponse<QualityRunSummary[]>(response);
  },

  getRunDetail: async (runId: string): Promise<QualityRunDetail | null> => {
    const response = await fetch(
      `/api/quality/runs/${encodeURIComponent(runId)}`
    );
    return handleApiResponse<QualityRunDetail | null>(response);
  },

  getIssuesForRun: async (runId: string): Promise<QualityIssueRecord[]> => {
    const response = await fetch(
      `/api/quality/runs/${encodeURIComponent(runId)}/issues`
    );
    return handleApiResponse<QualityIssueRecord[]>(response);
  },

  getLatestForTerminal: async (
    terminalId: string
  ): Promise<QualityRunSummary | null> => {
    const response = await fetch(
      `/api/terminals/${encodeURIComponent(terminalId)}/quality/latest`
    );
    return handleApiResponse<QualityRunSummary | null>(response);
  },
};

// ============================================================================
// Hooks
// ============================================================================

export function useQualityRuns(workflowId: string | undefined) {
  return useQuery({
    queryKey: qualityKeys.runsForWorkflow(workflowId ?? ''),
    queryFn: () => qualityApi.getRunsForWorkflow(workflowId!),
    enabled: !!workflowId,
    staleTime: 5 * 60 * 1000,
  });
}

export function useQualityRunDetail(runId: string | undefined) {
  return useQuery({
    queryKey: qualityKeys.runDetail(runId ?? ''),
    queryFn: () => qualityApi.getRunDetail(runId!),
    enabled: !!runId,
    staleTime: 5 * 60 * 1000,
  });
}

export function useQualityIssues(runId: string | undefined) {
  return useQuery({
    queryKey: qualityKeys.issuesForRun(runId ?? ''),
    queryFn: () => qualityApi.getIssuesForRun(runId!),
    enabled: !!runId,
    staleTime: 5 * 60 * 1000,
  });
}

export function useTerminalLatestQuality(terminalId: string | undefined) {
  return useQuery({
    queryKey: qualityKeys.latestForTerminal(terminalId ?? ''),
    queryFn: () => qualityApi.getLatestForTerminal(terminalId!),
    enabled: !!terminalId,
    staleTime: 30 * 1000,
  });
}
