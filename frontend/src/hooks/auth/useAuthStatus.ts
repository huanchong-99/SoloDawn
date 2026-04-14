import { useQuery } from '@tanstack/react-query';
import { ApiError, oauthApi } from '@/lib/api';
import { useEffect, useRef } from 'react';
import { useAuth } from '@/hooks';

interface UseAuthStatusOptions {
  enabled: boolean;
}

function getErrorStatus(err: unknown): number | undefined {
  if (err instanceof ApiError) return err.status;
  if (typeof err === 'object' && err !== null && 'status' in err) {
    const status = (err as { status?: unknown }).status;
    return typeof status === 'number' ? status : undefined;
  }
  return undefined;
}

export function useAuthStatus(options: UseAuthStatusOptions) {
  const query = useQuery({
    queryKey: ['auth', 'status'],
    queryFn: () => oauthApi.status(),
    enabled: options.enabled,
    refetchInterval: options.enabled ? 1000 : false,
    // Do not retry on auth failures — retrying a 401/403 against the auth
    // status endpoint will never succeed and only adds polling pressure.
    retry: (count, err) => {
      const status = getErrorStatus(err);
      if (status === 401 || status === 403) return false;
      return count < 3;
    },
    // Exponential backoff with 30s ceiling keeps retries sane when the
    // backend is flaking but we're still signed in.
    retryDelay: (attemptIndex) =>
      Math.min(1000 * 2 ** attemptIndex, 30_000),
    staleTime: 0, // Always fetch fresh data when enabled
  });

  const { isSignedIn } = useAuth();
  // Keep latest refetch in a ref so the effect below does not re-run (and
  // race the observer) every time react-query returns a new refetch identity.
  const refetchRef = useRef(query.refetch);
  refetchRef.current = query.refetch;
  useEffect(() => {
    if (!options.enabled) return;
    refetchRef.current();
  }, [isSignedIn, options.enabled]);

  return query;
}
