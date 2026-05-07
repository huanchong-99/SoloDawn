import { useQuery, useQueryClient } from '@tanstack/react-query';
import { oauthApi } from '@/lib/api';
import { useEffect } from 'react';
import { useAuth } from '@/hooks/auth/useAuth';
import { useWsStore } from '@/stores/wsStore';

export function useCurrentUser() {
  const { isSignedIn } = useAuth();
  const query = useQuery({
    queryKey: ['auth', 'user'],
    queryFn: () => oauthApi.getCurrentUser(),
    enabled: isSignedIn,
    retry: 2,
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });

  const queryClient = useQueryClient();
  useEffect(() => {
    if (!isSignedIn) return;
    queryClient.invalidateQueries({ queryKey: ['auth', 'user'] });
    // Cleanup runs when isSignedIn flips to false (or on unmount); this
    // ensures stale user data is cleared on sign-out.
    return () => {
      queryClient.removeQueries({ queryKey: ['auth', 'user'] });
      // W2-22-12: Reset WS store on sign-out so mutated handler Maps and
      // cached connection state don't leak across user sessions.
      useWsStore.getState().reset();
    };
  }, [queryClient, isSignedIn]);

  return query;
}
