import { useQuery } from '@tanstack/react-query';
import { oauthApi } from '@/lib/api';
import { useEffect } from 'react';
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
  const { refetch } = query;
  useEffect(() => {
    if (!options.enabled) return;
    refetch();
  }, [isSignedIn, options.enabled, refetch]);

  return query;
}
