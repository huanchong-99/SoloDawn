import { useCallback, useEffect, useMemo, useRef } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from './auth/useAuth';
import { secureRandomIdFragment } from '@/utils/id';

const MAX_VISITED_PATHS = 50;
const SESSION_STORAGE_KEY = 'solodawn.previous-path.session-id';
const visitedByScope = new Map<string, string[]>();

function createSessionId(): string {
  return `${Date.now()}-${secureRandomIdFragment(8)}`;
}

function getSessionId(): string {
  if (globalThis.window === undefined) {
    return 'server';
  }

  try {
    const existing = globalThis.window.sessionStorage.getItem(SESSION_STORAGE_KEY);
    if (existing) {
      return existing;
    }

    const created = createSessionId();
    globalThis.window.sessionStorage.setItem(SESSION_STORAGE_KEY, created);
    return created;
  } catch {
    return 'memory';
  }
}

function getScopeKey(userId: string | null): string {
  return `${getSessionId()}:${userId ?? 'anonymous'}`;
}

function getVisitedPaths(scopeKey: string): string[] {
  const existing = visitedByScope.get(scopeKey);
  if (existing) {
    return existing;
  }

  const created: string[] = [];
  visitedByScope.set(scopeKey, created);
  return created;
}

export function resetPreviousPathHistory(scopeKey?: string): void {
  if (scopeKey) {
    visitedByScope.delete(scopeKey);
    return;
  }
  visitedByScope.clear();
}

export function usePreviousPath() {
  const navigate = useNavigate();
  const location = useLocation();
  const { userId } = useAuth();
  const scopeKey = useMemo(() => getScopeKey(userId), [userId]);
  const lastRecordedRef = useRef<{ scopeKey: string; pathname: string } | null>(
    null
  );

  // Track pathnames as user navigates
  useEffect(() => {
    // Guard: only record when pathname (or scope) actually changed since last
    // effect run. Prevents redundant array mutation on unrelated re-renders
    // or StrictMode double-invocations.
    const last = lastRecordedRef.current;
    if (
      last &&
      last.scopeKey === scopeKey &&
      last.pathname === location.pathname
    ) {
      return;
    }

    const scopedVisited = getVisitedPaths(scopeKey);
    if (scopedVisited.at(-1) !== location.pathname) {
      scopedVisited.push(location.pathname);
      // Keep only last N entries to prevent memory bloat
      if (scopedVisited.length > MAX_VISITED_PATHS) {
        scopedVisited.splice(0, scopedVisited.length - MAX_VISITED_PATHS);
      }
    }
    lastRecordedRef.current = { scopeKey, pathname: location.pathname };
  }, [location.pathname, scopeKey]);

  return useCallback(() => {
    const scopedVisited = getVisitedPaths(scopeKey);
    // Find last non-settings route in history, skipping the current (last) entry
    const history = scopedVisited.slice(0, -1);
    const lastNonSettingsPath = [...history]
      .reverse()
      .find((p) => !p.startsWith('/settings'));
    navigate(lastNonSettingsPath || '/');
  }, [navigate, scopeKey]);
}
