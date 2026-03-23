import * as React from 'react';
import { create } from 'zustand';
import type { ModelConfig, ApiType } from '@/components/workflow/types';

/**
 * Model state management store
 * Manages AI model configurations, fetching, and verification
 */
interface ModelStoreState {
  // State
  models: ModelConfig[];
  availableModels: Map<string, string[]>; // apiType -> available model IDs
  isLoading: boolean;
  isFetching: boolean;
  isVerifying: string | null; // model ID being verified
  error: string | null;

  // Actions
  setModels: (models: ModelConfig[]) => void;
  addModel: (model: ModelConfig) => void;
  updateModel: (id: string, updates: Partial<ModelConfig>) => void;
  removeModel: (id: string) => void;
  setVerified: (id: string, verified: boolean) => void;

  // API actions
  fetchModels: (apiType: ApiType, apiKey: string, baseUrl?: string) => Promise<string[]>;
  verifyModel: (model: ModelConfig) => Promise<boolean>;

  // Available models cache
  setAvailableModels: (apiType: string, models: string[]) => void;
  getAvailableModels: (apiType: string) => string[];

  // Loading states
  setLoading: (loading: boolean) => void;
  setFetching: (fetching: boolean) => void;
  setVerifying: (modelId: string | null) => void;
  setError: (error: string | null) => void;

  // Reset
  reset: () => void;
}

// Default models for each API type (fallback when API fetch fails)
const DEFAULT_MODELS: Record<ApiType, string[]> = {
  anthropic: ['claude-3-5-sonnet-20241022', 'claude-3-5-haiku-20241022', 'claude-3-opus-20240229'],
  'anthropic-compatible': [],
  google: ['gemini-2.0-flash-exp', 'gemini-1.5-pro', 'gemini-1.5-flash'],
  openai: ['gpt-4o', 'gpt-4-turbo', 'gpt-4', 'gpt-3.5-turbo'],
  'openai-compatible': [],
};

export const useModelStore = create<ModelStoreState>((set, get) => ({
  // Initial state
  models: [],
  availableModels: new Map(),
  isLoading: false,
  isFetching: false,
  isVerifying: null,
  error: null,

  setModels: (models) => {
    set({ models });
  },

  addModel: (model) => {
    set((state) => ({
      models: [...state.models, model],
    }));
  },

  updateModel: (id, updates) => {
    set((state) => ({
      models: state.models.map((m) => (m.id === id ? { ...m, ...updates } : m)),
    }));
  },

  removeModel: (id) => {
    set((state) => ({
      models: state.models.filter((m) => m.id !== id),
    }));
  },

  setVerified: (id, verified) => {
    set((state) => ({
      models: state.models.map((m) => (m.id === id ? { ...m, isVerified: verified } : m)),
    }));
  },

  fetchModels: async (apiType, apiKey, baseUrl) => {
    set({ isFetching: true, error: null });

    try {
      // Build request URL
      const params = new URLSearchParams({ apiType });
      if (baseUrl) {
        params.append('baseUrl', baseUrl);
      }

      const response = await fetch(`/api/models/list?${params.toString()}`, {
        headers: {
          'X-API-Key': apiKey,
        },
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch models: ${response.statusText}`);
      }

      const data = await response.json();
      const models = data.models as string[];

      // Cache the available models
      get().setAvailableModels(apiType, models);

      set({ isFetching: false });
      return models;
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to fetch models';
      set({ error: errorMessage, isFetching: false });

      // Return default models as fallback
      const defaultModels = DEFAULT_MODELS[apiType] ?? [];
      get().setAvailableModels(apiType, defaultModels);
      return defaultModels;
    }
  },

  verifyModel: async (model) => {
    set({ isVerifying: model.id, error: null });

    try {
      const response = await fetch('/api/models/verify', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          apiType: model.apiType,
          baseUrl: model.baseUrl,
          apiKey: model.apiKey,
          modelId: model.modelId,
        }),
      });

      if (!response.ok) {
        throw new Error(`Verification failed: ${response.statusText}`);
      }

      const data = await response.json();
      const verified = data.verified as boolean;

      // Update model verification status
      get().setVerified(model.id, verified);

      set({ isVerifying: null });
      return verified;
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Verification failed';
      set({ error: errorMessage, isVerifying: null });
      return false;
    }
  },

  setAvailableModels: (apiType, models) => {
    set((state) => {
      const newAvailableModels = new Map(state.availableModels);
      newAvailableModels.set(apiType, models);
      return { availableModels: newAvailableModels };
    });
  },

  getAvailableModels: (apiType) => {
    const models = get().availableModels.get(apiType);
    return models ?? DEFAULT_MODELS[apiType as ApiType] ?? [];
  },

  setLoading: (loading) => {
    set({ isLoading: loading });
  },

  setFetching: (fetching) => {
    set({ isFetching: fetching });
  },

  setVerifying: (modelId) => {
    set({ isVerifying: modelId });
  },

  setError: (error) => {
    set({ error });
  },

  reset: () => {
    set({
      models: [],
      availableModels: new Map(),
      isLoading: false,
      isFetching: false,
      isVerifying: null,
      error: null,
    });
  },
}));

/**
 * Hook to get models as array
 */
export function useModelList() {
  return useModelStore((state) => state.models);
}

/**
 * Hook to get verified models only
 * Uses useMemo to prevent unnecessary re-renders
 */
export function useVerifiedModels() {
  const models = useModelStore((state) => state.models);
  return React.useMemo(() => models.filter((m) => m.isVerified), [models]);
}

/**
 * Hook to get available models for an API type
 */
export function useAvailableModels(apiType: string) {
  const availableModels = useModelStore((state) => state.availableModels);
  return availableModels.get(apiType) ?? DEFAULT_MODELS[apiType as ApiType] ?? [];
}
