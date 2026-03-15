import { useEffect, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { cliTypesKeys } from './useCliTypes';

// ============================================================================
// Types
// ============================================================================

export interface CliStatusChange {
  cli_type_id: string;
  cli_name: string;
  previous_installed: boolean;
  current_installed: boolean;
  previous_version: string | null;
  current_version: string | null;
  detected_at: string;
}

// ============================================================================
// Hook
// ============================================================================

/**
 * Hook that connects to the CLI status SSE stream and auto-updates
 * React Query cache when CLI installation status changes are detected.
 */
export function useCliStatusStream(enabled = true) {
  const queryClient = useQueryClient();
  const eventSourceRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (!enabled) return;

    const eventSource = new EventSource('/api/cli-types/status/stream');
    eventSourceRef.current = eventSource;

    eventSource.addEventListener('cli_status_change', (event) => {
      const change: CliStatusChange = JSON.parse(event.data);
      // Invalidate CLI detection queries to refresh UI
      queryClient.invalidateQueries({ queryKey: cliTypesKeys.detection });
      console.info(
        `CLI status change: ${change.cli_name} ${change.current_installed ? 'installed' : 'removed'}`
      );
    });

    eventSource.addEventListener('connection_established', () => {
      console.debug('CLI status stream connected');
    });

    eventSource.onerror = () => {
      console.warn('CLI status stream disconnected, will reconnect');
    };

    return () => {
      eventSource.close();
      eventSourceRef.current = null;
    };
  }, [enabled, queryClient]);
}
