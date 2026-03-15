import { useEffect, useMemo, useRef, useState } from 'react';
import { useExecutionProcessesContext } from '@/contexts/ExecutionProcessesContext';

export interface DevserverPreviewState {
  status: 'idle' | 'searching' | 'ready' | 'error';
  url?: string;
  port?: number;
  scheme: 'http' | 'https';
}

interface UseDevserverPreviewOptions {
  projectHasDevScript?: boolean;
  projectId: string; // Required for context-based URL persistence
  lastKnownUrl?: {
    url: string;
    port?: number;
    scheme: 'http' | 'https';
  };
}

const DEFAULT_OPTIONS: UseDevserverPreviewOptions = {
  projectId: '',
  projectHasDevScript: false,
} as const;

export function useDevserverPreview(
  attemptId?: string | null | undefined,
  options: UseDevserverPreviewOptions = DEFAULT_OPTIONS
): DevserverPreviewState {
  const { projectHasDevScript = false, lastKnownUrl } = options;
  const {
    executionProcessesVisible: executionProcesses,
    error: processesError,
  } = useExecutionProcessesContext();

  const [state, setState] = useState<DevserverPreviewState>({
    status: 'idle',
    scheme: 'http',
  });

  const prevAttemptIdRef = useRef(attemptId);

  const selectedProcess = useMemo(() => {
    const devserverProcesses = executionProcesses.filter(
      (process) =>
        process.runReason === 'devserver' && process.status === 'running'
    );

    if (devserverProcesses.length === 0) return null;

    return devserverProcesses.sort(
      (a, b) =>
        new Date(b.createdAt).getTime() -
        new Date(a.createdAt).getTime()
    )[0];
  }, [executionProcesses]);

  useEffect(() => {
    if (prevAttemptIdRef.current !== attemptId) {
      prevAttemptIdRef.current = attemptId;
      setState({
        status: 'idle',
        scheme: 'http',
        url: undefined,
        port: undefined,
      });
      return;
    }

    if (processesError) {
      setState((prev) => ({ ...prev, status: 'error' }));
      return;
    }

    if (!selectedProcess) {
      setState((prev) => ({
        status: projectHasDevScript ? 'searching' : 'idle',
        scheme: prev.scheme ?? 'http',
        url: undefined,
        port: undefined,
      }));
      return;
    }

    if (lastKnownUrl) {
      setState((prev) => {
        if (
          prev.status === 'ready' &&
          prev.url === lastKnownUrl.url &&
          prev.port === lastKnownUrl.port &&
          prev.scheme === lastKnownUrl.scheme
        ) {
          return prev;
        }

        return {
          status: 'ready',
          url: lastKnownUrl.url,
          port: lastKnownUrl.port,
          scheme: lastKnownUrl.scheme ?? 'http',
        };
      });
      return;
    }

    setState((prev) => ({
      status: 'searching',
      scheme: prev.scheme ?? 'http',
      url: undefined,
      port: undefined,
    }));
  }, [processesError, selectedProcess, lastKnownUrl, projectHasDevScript, attemptId]);

  return state;
}
