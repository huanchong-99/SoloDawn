import { useQuery } from '@tanstack/react-query';
import { useState, useCallback, useEffect, useMemo, useRef } from 'react';
import { sessionsApi } from '@/lib/api';
import type { Session } from 'shared/types';

interface UseWorkspaceSessionsOptions {
  enabled?: boolean;
}

/** Discriminated union for session selection state */
export type SessionSelection =
  | { mode: 'existing'; sessionId: string }
  | { mode: 'new' };

interface UseWorkspaceSessionsResult {
  sessions: Session[];
  selectedSession: Session | undefined;
  selectedSessionId: string | undefined;
  selectSession: (sessionId: string) => void;
  selectLatestSession: () => void;
  isLoading: boolean;
  /** Whether user is creating a new session */
  isNewSessionMode: boolean;
  /** Enter new session mode */
  startNewSession: () => void;
}

/**
 * Hook for managing sessions within a workspace.
 * Fetches all sessions for a workspace and provides session switching capability.
 * Sessions are ordered by most recently used (latest non-dev server execution first).
 */
export function useWorkspaceSessions(
  workspaceId: string | undefined,
  options: UseWorkspaceSessionsOptions = {}
): UseWorkspaceSessionsResult {
  const { enabled = true } = options;
  const [selection, setSelection] = useState<SessionSelection | undefined>(
    undefined
  );

  const { data: sessions = [], isLoading } = useQuery<Session[]>({
    queryKey: ['workspaceSessions', workspaceId],
    queryFn: () => sessionsApi.getByWorkspace(workspaceId!),
    enabled: enabled && !!workspaceId,
  });

  // Track the workspaceId for which selection was last applied so a stale
  // sessions array (from a previous workspace, still in react-query cache)
  // cannot clobber selection after the user switches workspaces.
  const appliedWorkspaceIdRef = useRef<string | undefined>(undefined);

  // Combined effect: handle workspace changes and auto-select sessions
  // This replaces two separate effects that had a race condition where the reset
  // effect would fire after auto-select when sessions were cached, undoing the selection.
  useEffect(() => {
    // "Did I change" token check: if workspaceId changed, reset selection and
    // wait for the matching sessions fetch before auto-selecting. This avoids
    // applying sessions from the previous workspace.
    if (appliedWorkspaceIdRef.current !== workspaceId) {
      appliedWorkspaceIdRef.current = workspaceId;
      setSelection(undefined);
      if (isLoading) return;
    }

    setSelection((prev) => {
      if (prev?.mode === 'new') return prev;

      if (sessions.length === 0) {
        // No sessions - reset selection (handles workspace change before fetch completes)
        return undefined;
      }

      if (
        prev?.mode === 'existing' &&
        sessions.some((session) => session.id === prev.sessionId)
      ) {
        // Keep user's current selection when it is still available.
        return prev;
      }

      // Sessions are ordered by most recently used, so first is the most recently used
      return { mode: 'existing', sessionId: sessions[0].id };
    });
  }, [workspaceId, sessions, isLoading]);

  // Derived values from selection state
  const isNewSessionMode = selection?.mode === 'new';
  const selectedSessionId =
    selection?.mode === 'existing' ? selection.sessionId : undefined;

  const selectedSession = useMemo(
    () => sessions.find((s) => s.id === selectedSessionId),
    [sessions, selectedSessionId]
  );

  const selectSession = useCallback((sessionId: string) => {
    setSelection({ mode: 'existing', sessionId });
  }, []);

  const selectLatestSession = useCallback(() => {
    if (sessions.length > 0) {
      setSelection({ mode: 'existing', sessionId: sessions[0].id });
    }
  }, [sessions]);

  const startNewSession = useCallback(() => {
    setSelection({ mode: 'new' });
  }, []);

  return {
    sessions,
    selectedSession,
    selectedSessionId,
    selectSession,
    selectLatestSession,
    isLoading,
    isNewSessionMode,
    startNewSession,
  };
}
