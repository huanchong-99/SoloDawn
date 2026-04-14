import { useQuery, useQueryClient } from '@tanstack/react-query';
import { oauthApi } from '@/lib/api';
import { useEffect } from 'react';
import { useAuth } from '@/hooks/auth/useAuth';

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
    };
  }, [queryClient, isSignedIn]);

  return query;
}
