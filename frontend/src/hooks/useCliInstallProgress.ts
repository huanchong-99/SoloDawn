import { useEffect, useRef, useState } from 'react';

// ============================================================================
// Types
// ============================================================================

export interface InstallLogLine {
  type: 'stdout' | 'stderr' | 'completed' | 'error';
  content: string;
  timestamp?: number;
  exit_code?: number;
}

export interface InstallProgressState {
  lines: InstallLogLine[];
  isComplete: boolean;
  exitCode: number | null;
  error: string | null;
}

// ============================================================================
// Hook
// ============================================================================

/**
 * Hook that connects to a WebSocket for real-time install/uninstall progress.
 * Returns streaming log lines and completion status.
 *
 * @param cliTypeId - The CLI type being installed/uninstalled
 * @param jobId - The job ID returned from the install/uninstall mutation
 */
export function useCliInstallProgress(
  cliTypeId: string | null,
  jobId: string | null
): InstallProgressState {
  const [state, setState] = useState<InstallProgressState>({
    lines: [],
    isComplete: false,
    exitCode: null,
    error: null,
  });
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    if (!cliTypeId || !jobId) return;

    // Reset state for new connection
    setState({ lines: [], isComplete: false, exitCode: null, error: null });

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(
      `${protocol}//${window.location.host}/api/cli-types/${encodeURIComponent(cliTypeId)}/install/ws?job_id=${encodeURIComponent(jobId)}`
    );
    wsRef.current = ws;

    ws.onmessage = (event) => {
      const msg: InstallLogLine = JSON.parse(event.data);
      setState((prev) => {
        const newLines = [...prev.lines, msg];
        if (msg.type === 'completed') {
          return {
            ...prev,
            lines: newLines,
            isComplete: true,
            exitCode: msg.exit_code ?? null,
          };
        }
        if (msg.type === 'error') {
          return {
            ...prev,
            lines: newLines,
            isComplete: true,
            error: msg.content,
          };
        }
        return { ...prev, lines: newLines };
      });
    };

    ws.onerror = () => {
      setState((prev) => ({
        ...prev,
        isComplete: true,
        error: 'WebSocket connection error',
      }));
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, [cliTypeId, jobId]);

  return state;
}
