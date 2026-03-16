import { useCallback, useState } from 'react';
import type { ApiType } from '@/components/workflow/types';

interface ModelEntry {
  id: string;
  name: string;
}

export interface UseModelVerificationResult {
  models: ModelEntry[];
  isLoading: boolean;
  isVerified: boolean;
  isVerifying: boolean;
  verifyError: string | null;
  fetchModels: (apiType: string, apiKey: string, baseUrl?: string) => Promise<void>;
  verifyModel: (config: {
    apiType: string;
    apiKey: string;
    baseUrl?: string;
    modelId: string;
  }) => Promise<boolean>;
  reset: () => void;
}

const DEFAULT_MODELS: Record<ApiType, string[]> = {
  anthropic: ['claude-3-5-sonnet-20241022', 'claude-3-5-haiku-20241022', 'claude-3-opus-20240229'],
  google: ['gemini-2.0-flash-exp', 'gemini-1.5-pro', 'gemini-1.5-flash'],
  openai: ['gpt-4o', 'gpt-4-turbo', 'gpt-4', 'gpt-3.5-turbo'],
  'openai-compatible': [],
};

/**
 * Reusable hook for model fetching and verification.
 * Extracts API interaction logic from Step3Models into a composable unit.
 */
export function useModelVerification(): UseModelVerificationResult {
  const [models, setModels] = useState<ModelEntry[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [isVerified, setIsVerified] = useState(false);
  const [isVerifying, setIsVerifying] = useState(false);
  const [verifyError, setVerifyError] = useState<string | null>(null);

  const fetchModels = useCallback(
    async (apiType: string, apiKey: string, baseUrl?: string) => {
      setIsLoading(true);
      setVerifyError(null);

      try {
        const params = new URLSearchParams({ apiType });
        if (baseUrl) {
          params.append('baseUrl', baseUrl);
        }

        const response = await fetch(`/api/models/list?${params.toString()}`, {
          headers: { 'X-API-Key': apiKey },
        });

        if (!response.ok) {
          throw new Error(`Failed to fetch models: ${response.statusText}`);
        }

        const data = await response.json();
        const fetched = (data.models as string[]).map((m) => ({
          id: m,
          name: m,
        }));
        setModels(fetched);
      } catch (_err) {
        // Fall back to default models for the provider
        const defaults = DEFAULT_MODELS[apiType as ApiType] ?? [];
        setModels(defaults.map((m) => ({ id: m, name: m })));
        setVerifyError(
          _err instanceof Error ? _err.message : 'Failed to fetch models'
        );
      } finally {
        setIsLoading(false);
      }
    },
    []
  );

  const verifyModel = useCallback(
    async (cfg: {
      apiType: string;
      apiKey: string;
      baseUrl?: string;
      modelId: string;
    }): Promise<boolean> => {
      setIsVerifying(true);
      setVerifyError(null);

      try {
        const response = await fetch('/api/models/verify', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            apiType: cfg.apiType,
            baseUrl: cfg.baseUrl ?? '',
            apiKey: cfg.apiKey,
            modelId: cfg.modelId,
          }),
        });

        if (!response.ok) {
          throw new Error(`Verification failed: ${response.statusText}`);
        }

        const data = await response.json();
        const verified = data.verified as boolean;

        setIsVerified(verified);
        if (!verified) {
          setVerifyError('Model verification returned false');
        }
        return verified;
      } catch (_err) {
        const msg =
          _err instanceof Error ? _err.message : 'Verification failed';
        setVerifyError(msg);
        setIsVerified(false);
        return false;
      } finally {
        setIsVerifying(false);
      }
    },
    []
  );

  const reset = useCallback(() => {
    setModels([]);
    setIsLoading(false);
    setIsVerified(false);
    setIsVerifying(false);
    setVerifyError(null);
  }, []);

  return {
    models,
    isLoading,
    isVerified,
    isVerifying,
    verifyError,
    fetchModels,
    verifyModel,
    reset,
  };
}
