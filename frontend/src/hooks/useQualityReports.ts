import { useQuery } from '@tanstack/react-query';
import { handleApiResponse } from '@/lib/api';
import type {
  QualityRun,
  QualityIssueRecord
} from 'shared/types';

export const qualityKeys = {
  all: ['quality'] as const,
  workflow: (workflowId: string) => [...qualityKeys.all, 'workflow', workflowId] as const,
  terminal: (terminalId: string) => [...qualityKeys.all, 'terminal', terminalId] as const,
  issues: (runId: string) => [...qualityKeys.all, 'issues', runId] as const,
};

const qualityApi = {
  getWorkflowQuality: async (workflowId: string): Promise<QualityRun[]> => {
    const response = await fetch(`/api/workflows/${encodeURIComponent(workflowId)}/quality`);
    return handleApiResponse<QualityRun[]>(response);
  },
  getTerminalQuality: async (terminalId: string): Promise<QualityRun[]> => {
    const response = await fetch(`/api/terminals/${encodeURIComponent(terminalId)}/quality`);
    return handleApiResponse<QualityRun[]>(response);
  },
  getQualityIssues: async (runId: string): Promise<QualityIssueRecord[]> => {
    const response = await fetch(`/api/quality/runs/${encodeURIComponent(runId)}/issues`);
    return handleApiResponse<QualityIssueRecord[]>(response);
  }
};

interface UseQualityQueryOptions {
  enabled?: boolean;
  refetchInterval?: number | false | ((data: any) => number | false);
}

export function useWorkflowQuality(workflowId: string | undefined | null, options?: UseQualityQueryOptions) {
  return useQuery({
    queryKey: qualityKeys.workflow(workflowId || ''),
    queryFn: () => qualityApi.getWorkflowQuality(workflowId!),
    enabled: Boolean(workflowId) && (options?.enabled ?? true),
    refetchInterval: options?.refetchInterval ?? false,
    staleTime: 1000 * 60,
  });
}

export function useTerminalQuality(terminalId: string | undefined | null, options?: UseQualityQueryOptions) {
  return useQuery({
    queryKey: qualityKeys.terminal(terminalId || ''),
    queryFn: () => qualityApi.getTerminalQuality(terminalId!),
    enabled: Boolean(terminalId) && (options?.enabled ?? true),
    refetchInterval: options?.refetchInterval ?? false,
    staleTime: 1000 * 60, // 1 min update cache
  });
}

export function useQualityIssues(runId: string | undefined | null, options?: UseQualityQueryOptions) {
  return useQuery({
    queryKey: qualityKeys.issues(runId || ''),
    queryFn: () => qualityApi.getQualityIssues(runId!),
    enabled: Boolean(runId) && (options?.enabled ?? true),
    refetchInterval: options?.refetchInterval ?? false,
    staleTime: 1000 * 60 * 5, // 5 mins cache
  });
}
