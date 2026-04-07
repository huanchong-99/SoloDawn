import { useQuery } from '@tanstack/react-query';
import { configApi } from '../lib/api';

interface NativeCredentialsStatus {
  available: boolean;
  cliVersion: string | null;
  defaultModel: string | null;
}

/**
 * Check whether the local Claude Code CLI has valid OAuth credentials,
 * enabling the "Native Subscription" model option without manual API key setup.
 */
export function useNativeCredentials() {
  return useQuery<NativeCredentialsStatus>({
    queryKey: ['native-credentials-status'],
    queryFn: () => configApi.getNativeCredentialsStatus(),
    staleTime: 5 * 60 * 1000, // 5 minutes
    retry: false,
  });
}
