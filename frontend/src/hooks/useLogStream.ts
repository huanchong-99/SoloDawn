import { useEffect, useState, useRef, useCallback } from 'react';
import type { PatchType } from 'shared/types';

type LogEntry = Extract<PatchType, { type: 'STDOUT' } | { type: 'STDERR' }>;
const MAX_LOG_ENTRIES = 5000;

interface UseLogStreamResult {
  logs: LogEntry[];
  error: string | null;
}

function parseLogPatches(data: unknown): LogEntry[] {
  if (!data || typeof data !== 'object' || !('JsonPatch' in data)) return [];
  const patches = (data as { JsonPatch: Array<{ value?: PatchType }> }).JsonPatch;
  const entries: LogEntry[] = [];
  for (const patch of patches) {
    const value = patch?.value;
    if (value?.type === 'STDOUT' || value?.type === 'STDERR') {
      entries.push({ type: value.type, content: value.content });
    }
  }
  return entries;
}

export const useLogStream = (processId: string): UseLogStreamResult => {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const retryCountRef = useRef<number>(0);
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isIntentionallyClosed = useRef<boolean>(false);
  // Track current processId to prevent stale WebSocket messages from contaminating logs
  const currentProcessIdRef = useRef<string>(processId);

  const addLogEntry = useCallback((entry: LogEntry, capturedProcessId: string) => {
    if (currentProcessIdRef.current !== capturedProcessId) return;
    setLogs((prev) => {
      const next = [...prev, entry];
      return next.length <= MAX_LOG_ENTRIES ? next : next.slice(next.length - MAX_LOG_ENTRIES);
    });
  }, []);

  const handleMessage = useCallback((event: MessageEvent, capturedProcessId: string) => {
    try {
      const data = JSON.parse(event.data);
      const entries = parseLogPatches(data);
      for (const entry of entries) {
        addLogEntry(entry, capturedProcessId);
      }
      if (data.finished === true) {
        isIntentionallyClosed.current = true;
        wsRef.current?.close();
      }
    } catch (e) {
      console.error('Failed to parse message:', e);
    }
  }, [addLogEntry]);

  useEffect(() => {
    if (!processId) {
      return;
    }

    // Update the ref to track the current processId
    currentProcessIdRef.current = processId;

    // Reset retry count on process id change to prevent unbounded growth
    retryCountRef.current = 0;

    // Clear logs when process changes
    setLogs([]);
    setError(null);

    const open = () => {
      // Capture processId at the time of opening the WebSocket
      const capturedProcessId = processId;
      const protocol = globalThis.window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const host = globalThis.window.location.host;
      const ws = new WebSocket(
        `${protocol}//${host}/api/execution-processes/${processId}/raw-logs/ws`
      );
      wsRef.current = ws;
      isIntentionallyClosed.current = false;

      ws.onopen = () => {
        if (currentProcessIdRef.current !== capturedProcessId) {
          ws.close();
          return;
        }
        setError(null);
        setLogs([]);
        retryCountRef.current = 0;
      };

      ws.onmessage = (event) => handleMessage(event, capturedProcessId);

      ws.onerror = () => {
        if (currentProcessIdRef.current !== capturedProcessId) return;
        setError('Connection failed');
      };

      ws.onclose = (event) => {
        if (currentProcessIdRef.current !== capturedProcessId) return;
        if (!isIntentionallyClosed.current && event.code !== 1000) {
          const next = retryCountRef.current + 1;
          retryCountRef.current = next;
          if (next <= 6) {
            const delay = Math.min(1500, 250 * 2 ** (next - 1));
            retryTimerRef.current = setTimeout(open, delay);
          }
        }
      };
    };

    open();

    return () => {
      if (wsRef.current) {
        isIntentionallyClosed.current = true;
        wsRef.current.close();
        wsRef.current = null;
      }
      if (retryTimerRef.current) {
        clearTimeout(retryTimerRef.current);
        retryTimerRef.current = null;
      }
    };
  }, [processId, handleMessage]);

  return { logs, error };
};
