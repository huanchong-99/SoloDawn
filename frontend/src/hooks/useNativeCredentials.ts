import { useQuery } from '@tanstack/react-query';
import { configApi } from '../lib/api';

/** One switchable official Claude model for native-subscription users. */
export interface NativeClaudeModelOption {
  /** model_config row id, e.g. 'model-claude-sonnet'. */
  id: string;
  /** Display name, e.g. 'Claude Sonnet'. */
  displayName: string;
  /** Concrete API model id, e.g. 'claude-sonnet-5'. */
  apiModelId: string;
  /** Whether this is the DB default for Claude Code. */
  isDefault: boolean;
}

interface NativeCredentialsStatus {
  available: boolean;
  cliVersion: string | null;
  /** DB default official Claude model id (e.g. 'claude-sonnet-5'). */
  defaultModel: string | null;
  /** Official Claude models the subscription user can switch between (default first). */
  models: NativeClaudeModelOption[];
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
