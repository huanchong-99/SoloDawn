import { useCallback, useMemo, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useWorkflow, workflowKeys } from '@/hooks/useWorkflows';
import { makeRequest, handleApiResponse } from '@/lib/api';
import {
  useWorkflowEvents,
  type GitCommitPayload,
  type TaskStatusPayload,
  type TerminalStatusPayload,
  type WorkflowStatusPayload,
} from '@/stores/wsStore';

/**
 * A single event entry displayed in the live progress timeline.
 */
export interface LiveEvent {
  id: string;
  type: 'git_commit' | 'task_status' | 'terminal_status' | 'workflow_status';
  timestamp: string;
  summary: string;
}

const MAX_EVENTS = 50;

/**
 * Hook that provides live workflow execution data by combining React Query
 * polling with WebSocket event subscriptions.
 *
 * Follows the same debounced-invalidation pattern used in Board.tsx (300ms).
 */
export function useWorkflowLiveStatus(workflowId: string | null) {
  const queryClient = useQueryClient();
  const [liveEvents, setLiveEvents] = useState<LiveEvent[]>([]);

  const {
    data: workflow,
    isLoading,
  } = useWorkflow(workflowId ?? '', {
    staleTime: 5_000,
    refetchInterval: 10_000,
  });

  // --- Debounced cache invalidation (same pattern as Board.tsx) ---
  const invalidationTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const debouncedInvalidate = useCallback(() => {
    if (!workflowId) return;
    if (invalidationTimerRef.current) {
      clearTimeout(invalidationTimerRef.current);
    }
    invalidationTimerRef.current = setTimeout(() => {
      queryClient.invalidateQueries({
        queryKey: workflowKeys.byId(workflowId),
      });
      invalidationTimerRef.current = null;
    }, 300);
  }, [queryClient, workflowId]);

  const pushEvent = useCallback(
    (event: Omit<LiveEvent, 'id' | 'timestamp'>) => {
      const entry: LiveEvent = {
        ...event,
        id: globalThis.crypto.randomUUID(),
        timestamp: new Date().toISOString(),
      };
      setLiveEvents((prev) => [entry, ...prev].slice(0, MAX_EVENTS));
    },
    [],
  );

  // --- WebSocket event handlers ---
  const handlers = useMemo(
    () => ({
      onWorkflowStatusChanged: (payload: WorkflowStatusPayload) => {
        pushEvent({
          type: 'workflow_status',
          summary: `Workflow ${payload.status}`,
        });
        debouncedInvalidate();
      },
      onTaskStatusChanged: (payload: TaskStatusPayload) => {
        pushEvent({
          type: 'task_status',
          summary: `Task ${payload.taskId.slice(0, 8)} ${payload.status}`,
        });
        debouncedInvalidate();
      },
      onTerminalStatusChanged: (payload: TerminalStatusPayload) => {
        pushEvent({
          type: 'terminal_status',
          summary: `Terminal ${payload.terminalId.slice(0, 8)} ${payload.status}`,
        });
        debouncedInvalidate();
      },
      onTerminalCompleted: () => {
        debouncedInvalidate();
      },
      onGitCommitDetected: (payload: GitCommitPayload) => {
        const shortHash = payload.commitHash.slice(0, 7);
        const shortMsg = payload.message.length > 60
          ? `${payload.message.slice(0, 57)}...`
          : payload.message;
        pushEvent({
          type: 'git_commit',
          summary: `${shortHash} ${shortMsg}`,
        });
        debouncedInvalidate();
      },
    }),
    [pushEvent, debouncedInvalidate],
  );

  const { connectionStatus } = useWorkflowEvents(workflowId, handlers);

  const workflowStatus = workflow?.status ?? null;
  const tasks = workflow?.tasks ?? [];
  // liveEventCount triggers re-render when new WS events arrive

  // Fetch persisted events from backend
  const { data: persistedEvents } = useQuery({
    queryKey: ['workflowEvents', workflowId],
    queryFn: async () => {
      const res = await makeRequest(`/api/workflows/${workflowId}/events`);
      return handleApiResponse<Array<{ id: string; event_type: string; summary: string; created_at: string }>>(res);
    },
    enabled: !!workflowId,
    staleTime: 30_000,
  });

  // Merge persisted events with live events
  const events = useMemo(() => {
    const historical: LiveEvent[] = (persistedEvents ?? []).map((e) => ({
      id: e.id,
      type: e.event_type as LiveEvent['type'],
      timestamp: e.created_at,
      summary: e.summary,
    }));
    // Live events take priority (newest first)
    if (liveEvents.length > 0) {
      const liveIds = new Set(liveEvents.map((e) => e.id));
      const merged = [...liveEvents, ...historical.filter((e) => !liveIds.has(e.id))];
      return merged.slice(0, MAX_EVENTS);
    }
    return historical;
  }, [persistedEvents, liveEvents]);

  return {
    workflowStatus,
    tasks,
    events,
    isLoading,
    connectionStatus,
  };
}
