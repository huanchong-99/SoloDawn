import { useQuery } from '@tanstack/react-query';
import { oauthApi } from '@/lib/api';
import { useEffect, useRef } from 'react';
import { useAuth } from '@/hooks';

interface UseAuthStatusOptions {
  enabled: boolean;
}

export function useAuthStatus(options: UseAuthStatusOptions) {
  const query = useQuery({
    queryKey: ['auth', 'status'],
    queryFn: () => oauthApi.status(),
    enabled: options.enabled,
    refetchInterval: options.enabled ? 1000 : false,
    retry: 3,
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
