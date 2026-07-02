import { useMemo, useCallback } from 'react';
import { useQueries } from '@tanstack/react-query';
import { attemptsApi, executionProcessesApi } from '@/lib/api';
import { useTaskStopping } from '@/stores/useTaskDetailsUiStore';
import { useExecutionProcessesContext } from '@/contexts/ExecutionProcessesContext';
import type { AttemptData } from '@/lib/types';
import type { ExecutionProcess } from 'shared/types';

export function useAttemptExecution(attemptId?: string, taskId?: string) {
  const stopStateKey = taskId ?? attemptId;
  const { isStopping, setIsStopping } = useTaskStopping(stopStateKey);

  const {
    executionProcessesVisible: executionProcesses,
    isAttemptRunningVisible: isAttemptRunning,
    isLoading: streamLoading,
  } = useExecutionProcessesContext();

  // Get setup script processes that need detailed info
  const setupProcesses = useMemo(() => {
    if (!executionProcesses.length) return [] as ExecutionProcess[];
    return executionProcesses.filter((p) => p.runReason === 'setupscript');
  }, [executionProcesses]);

  // Fetch details for setup processes
  const processDetailQueries = useQueries({
    queries: setupProcesses.map((process) => ({
      queryKey: ['processDetails', process.id],
      queryFn: () => executionProcessesApi.getDetails(process.id),
      enabled: !!process.id,
    })),
  });

  // Extract data from queries so useMemo has a stable dependency
  const processDetailData = processDetailQueries.map((q) => q.data);

  // Build attempt data combining processes and details
  const attemptData: AttemptData = useMemo(() => {
    if (!executionProcesses.length) {
      return { processes: [], runningProcessDetails: {} };
    }

    // Build runningProcessDetails from the detail queries
    const runningProcessDetails: Record<string, ExecutionProcess> = {};

    setupProcesses.forEach((process, index) => {
      const detail = processDetailData[index];
      if (detail) {
        runningProcessDetails[process.id] = detail;
      }
    });

    return {
      processes: executionProcesses,
      runningProcessDetails,
    };
  }, [executionProcesses, setupProcesses, processDetailData]);

  // Stop execution function
  const stopExecution = useCallback(async () => {
    if (!attemptId || !isAttemptRunning || isStopping) return;

    try {
      setIsStopping(true);
      await attemptsApi.stop(attemptId);
    } catch (error) {
      console.error('Failed to stop executions:', error);
      throw error;
    } finally {
      setIsStopping(false);
    }
  }, [attemptId, isAttemptRunning, isStopping, setIsStopping]);

  const isLoading =
    streamLoading || processDetailQueries.some((q) => q.isLoading);
  const isFetching =
    streamLoading || processDetailQueries.some((q) => q.isFetching);

  return {
    // Data
    processes: executionProcesses,
    attemptData,
    runningProcessDetails: attemptData.runningProcessDetails,

    // Status
    isAttemptRunning,
    isLoading,
    isFetching,

    // Actions
    stopExecution,
    isStopping,
  };
}
