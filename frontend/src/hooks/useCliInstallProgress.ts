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

    const protocol = globalThis.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(
      `${protocol}//${globalThis.location.host}/api/cli-types/${encodeURIComponent(cliTypeId)}/install/ws?job_id=${encodeURIComponent(jobId)}`
    );
    wsRef.current = ws;

    ws.onmessage = (event) => {
      let msg: InstallLogLine;
      try {
        msg = JSON.parse(event.data) as InstallLogLine;
      } catch {
        // Ignore non-JSON frames (e.g. keepalive/ping)
        return;
      }
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

    ws.onclose = () => {
      setState((prev) =>
        prev.isComplete ? prev : { ...prev, isComplete: true }
      );
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, [cliTypeId, jobId]);

  return state;
}
