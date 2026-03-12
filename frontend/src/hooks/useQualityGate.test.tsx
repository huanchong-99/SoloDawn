import { describe, it, expect, beforeEach, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

const { mockLogApiError } = vi.hoisted(() => ({
  mockLogApiError: vi.fn(),
}));

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api');
  return {
    ...actual,
    logApiError: mockLogApiError,
  };
});

import {
  useQualityRuns,
  useQualityRunDetail,
  useQualityIssues,
  useTerminalLatestQuality,
  qualityKeys,
} from './useQualityGate';

// ============================================================================
// Test Utilities
// ============================================================================

const createMockQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

const wrapper = ({ children }: Readonly<{ children: React.ReactNode }>) => (
  <QueryClientProvider client={createMockQueryClient()}>
    {children}
  </QueryClientProvider>
);

const createSuccessResponse = (data: unknown) =>
  ({
    ok: true,
    json: async () => ({ success: true, data }),
  }) as Response;

const createErrorResponse = (message: string, status: number = 500) =>
  ({
    ok: false,
    status,
    statusText: message,
    json: async () => ({ success: false, message }),
  }) as Response;

// ============================================================================
// Mock Data
// ============================================================================

const mockRunSummary = {
  id: 'run-1',
  workflowId: 'wf-1',
  taskId: 'task-1',
  terminalId: 'term-1',
  commitHash: 'abc123',
  gateLevel: 'terminal',
  gateStatus: 'ok',
  mode: 'warn',
  totalIssues: 0,
  blockingIssues: 0,
  newIssues: 0,
  durationMs: 1200,
  errorMessage: null,
  createdAt: '2024-01-01T00:00:00Z',
  completedAt: '2024-01-01T00:00:01Z',
};

const mockRunDetail = {
  ...mockRunSummary,
  providersRun: null,
  reportJson: null,
  decisionJson: null,
};

const mockIssue = {
  id: 'issue-1',
  runId: 'run-1',
  provider: 'clippy',
  ruleId: 'unused_variable',
  severity: 'warning',
  message: 'Unused variable x',
  filePath: 'src/main.rs',
  startLine: 10,
  endLine: 10,
  startCol: null,
  endCol: null,
  isNew: true,
  isBlocking: false,
  snippet: 'let x = 5;',
  suggestion: 'Remove or use x',
  createdAt: '2024-01-01T00:00:00Z',
};

// ============================================================================
// Tests
// ============================================================================

beforeEach(() => {
  vi.clearAllMocks();
});

describe('qualityKeys', () => {
  it('should generate correct query keys', () => {
    expect(qualityKeys.all).toEqual(['quality']);
    expect(qualityKeys.runsForWorkflow('wf-1')).toEqual([
      'quality',
      'runs',
      'workflow',
      'wf-1',
    ]);
    expect(qualityKeys.runDetail('run-1')).toEqual([
      'quality',
      'run',
      'run-1',
    ]);
    expect(qualityKeys.issuesForRun('run-1')).toEqual([
      'quality',
      'issues',
      'run-1',
    ]);
    expect(qualityKeys.latestForTerminal('term-1')).toEqual([
      'quality',
      'latest',
      'terminal',
      'term-1',
    ]);
  });
});

describe('useQualityRuns', () => {
  it('should fetch quality runs for a workflow', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => createSuccessResponse([mockRunSummary]))
    );

    const { result } = renderHook(() => useQualityRuns('wf-1'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockRunSummary]);
    expect(fetch).toHaveBeenCalledWith('/api/workflows/wf-1/quality/runs');
  });

  it('should not fetch when workflowId is undefined', () => {
    vi.stubGlobal('fetch', vi.fn());

    renderHook(() => useQualityRuns(undefined), { wrapper });

    expect(fetch).not.toHaveBeenCalled();
  });

  it('should handle fetch errors', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => createErrorResponse('Server error'))
    );

    const { result } = renderHook(() => useQualityRuns('wf-1'), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useQualityRunDetail', () => {
  it('should fetch quality run detail', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => createSuccessResponse(mockRunDetail))
    );

    const { result } = renderHook(() => useQualityRunDetail('run-1'), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(mockRunDetail);
    expect(fetch).toHaveBeenCalledWith('/api/quality/runs/run-1');
  });

  it('should not fetch when runId is undefined', () => {
    vi.stubGlobal('fetch', vi.fn());

    renderHook(() => useQualityRunDetail(undefined), { wrapper });

    expect(fetch).not.toHaveBeenCalled();
  });
});

describe('useQualityIssues', () => {
  it('should fetch quality issues for a run', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => createSuccessResponse([mockIssue]))
    );

    const { result } = renderHook(() => useQualityIssues('run-1'), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockIssue]);
    expect(fetch).toHaveBeenCalledWith('/api/quality/runs/run-1/issues');
  });

  it('should not fetch when runId is undefined', () => {
    vi.stubGlobal('fetch', vi.fn());

    renderHook(() => useQualityIssues(undefined), { wrapper });

    expect(fetch).not.toHaveBeenCalled();
  });
});

describe('useTerminalLatestQuality', () => {
  it('should fetch latest quality run for a terminal', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => createSuccessResponse(mockRunSummary))
    );

    const { result } = renderHook(
      () => useTerminalLatestQuality('term-1'),
      { wrapper }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(mockRunSummary);
    expect(fetch).toHaveBeenCalledWith(
      '/api/terminals/term-1/quality/latest'
    );
  });

  it('should not fetch when terminalId is undefined', () => {
    vi.stubGlobal('fetch', vi.fn());

    renderHook(() => useTerminalLatestQuality(undefined), { wrapper });

    expect(fetch).not.toHaveBeenCalled();
  });

  it('should handle null response (no quality runs)', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => createSuccessResponse(null))
    );

    const { result } = renderHook(
      () => useTerminalLatestQuality('term-1'),
      { wrapper }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBeNull();
  });
});
