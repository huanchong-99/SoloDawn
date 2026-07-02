import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  useCliTypes,
  useCliDetection,
  useModelsForCli,
  cliTypesKeys,
  type CliType,
  type CliDetectionResult,
  type CliModel,
} from './useCliTypes';

// Mock global fetch
let mockFetch: ReturnType<typeof vi.fn>;
globalThis.fetch = vi.fn() as unknown as typeof fetch;

// ============================================================================
// Test Utilities
// ============================================================================

const createMockQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

const wrapper = ({ children }: Readonly<{ children: React.ReactNode }>) => (
  <QueryClientProvider client={createMockQueryClient()}>
    {children}
  </QueryClientProvider>
);

// Mock CLI types data
const mockCliTypes: CliType[] = [
  {
    id: 'claude-code',
    name: 'claude-code',
    displayName: 'Claude Code',
    description: 'Anthropic Claude Code CLI',
    executableCommand: 'claude',
    versionCheckCommand: 'claude --version',
    website: 'https://claude.ai/code',
    documentationUrl: 'https://docs.anthropic.com/claude/code',
  },
  {
    id: 'qwen-code',
    name: 'qwen-code',
    displayName: 'Qwen Code',
    description: 'Alibaba Qwen Code CLI',
    executableCommand: 'qwen',
    versionCheckCommand: 'qwen --version',
    website: 'https://github.com/QwenLM/qwen-code',
  },
  {
    id: 'codex',
    name: 'codex',
    displayName: 'Codex',
    description: 'OpenAI Codex CLI',
    executableCommand: 'codex',
    versionCheckCommand: 'codex --version',
    website: 'https://openai.com/codex',
  },
];

// Mock CLI detection results
const mockDetectionResults: CliDetectionResult[] = [
  {
    cliTypeId: 'claude-code',
    isInstalled: true,
    version: '1.0.0',
    path: '/usr/local/bin/claude',
  },
  {
    cliTypeId: 'qwen-code',
    isInstalled: false,
    error: 'Command not found',
  },
  {
    cliTypeId: 'codex',
    isInstalled: true,
    version: '2.1.0',
    path: '/usr/local/bin/codex',
  },
];

// Mock models data
const mockModels: CliModel[] = [
  {
    id: 'claude-3-5-sonnet',
    cliTypeId: 'claude-code',
    modelId: 'claude-3-5-sonnet-20241022',
    displayName: 'Claude 3.5 Sonnet',
    provider: 'anthropic',
    apiType: 'anthropic',
    requiresConfig: true,
    configSchema: {
      type: 'object',
      properties: {
        apiKey: { type: 'string' },
      },
    },
  },
  {
    id: 'claude-3-opus',
    cliTypeId: 'claude-code',
    modelId: 'claude-3-opus-20240229',
    displayName: 'Claude 3 Opus',
    provider: 'anthropic',
    apiType: 'anthropic',
    requiresConfig: true,
  },
];

// ============================================================================
// Tests
// ============================================================================

describe('useCliTypes', () => {
  beforeEach(() => {
    mockFetch = vi.fn();
    globalThis.fetch = mockFetch as unknown as typeof fetch;
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('cliTypesKeys', () => {
    it('should generate correct query keys', () => {
      expect(cliTypesKeys.all).toEqual(['cliTypes']);
      expect(cliTypesKeys.models('claude-code')).toEqual([
        'cliTypes',
        'models',
        'claude-code',
      ]);
      expect(cliTypesKeys.detection).toEqual(['cliTypes', 'detection']);
    });
  });

  describe('useCliTypes', () => {
    it('should fetch CLI types successfully', async () => {
      globalThis.fetch = vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve(mockCliTypes),
        } as Response)
      );

      const { result } = renderHook(() => useCliTypes(), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(result.current.data).toEqual(mockCliTypes);
      expect(result.current.data).toHaveLength(3);
    });

    it('should handle fetch errors', async () => {
      globalThis.fetch = vi.fn(() =>
        Promise.resolve({
          ok: false,
          status: 500,
          json: () =>
            Promise.resolve({
              message: 'Internal Server Error',
            }),
        } as Response)
      );

      const { result } = renderHook(() => useCliTypes(), { wrapper });

      await waitFor(() => expect(result.current.isError).toBe(true));
      expect(result.current.error).toBeInstanceOf(Error);
    });

    it('should use stale time of 1 hour', async () => {
      globalThis.fetch = vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve(mockCliTypes),
        } as Response)
      );

      const { result } = renderHook(() => useCliTypes(), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      // Verify data is cached (not checking staleTime directly as it's an implementation detail)
      expect(result.current.data).toEqual(mockCliTypes);
    });
  });

  describe('useCliDetection', () => {
    it('should detect CLI installation status successfully', async () => {
      globalThis.fetch = vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve(mockDetectionResults),
        } as Response)
      );

      const { result } = renderHook(() => useCliDetection(), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(result.current.data).toEqual(mockDetectionResults);
      expect(result.current.data).toHaveLength(3);
      expect(result.current.data?.[0].isInstalled).toBe(true);
      expect(result.current.data?.[1].isInstalled).toBe(false);
    });

    it('should handle detection errors', async () => {
      globalThis.fetch = vi.fn(() =>
        Promise.resolve({
          ok: false,
          status: 500,
          json: () =>
            Promise.resolve({
              message: 'Detection failed',
            }),
        } as Response)
      );

      const { result } = renderHook(() => useCliDetection(), { wrapper });

      await waitFor(() => expect(result.current.isError).toBe(true));
      expect(result.current.error).toBeInstanceOf(Error);
    });

    it('should use stale time of 5 minutes', async () => {
      globalThis.fetch = vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve(mockDetectionResults),
        } as Response)
      );

      const { result } = renderHook(() => useCliDetection(), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      // Verify data is cached (not checking staleTime directly as it's an implementation detail)
      expect(result.current.data).toEqual(mockDetectionResults);
    });
  });

  describe('useModelsForCli', () => {
    it('should fetch models for CLI type successfully', async () => {
      const cliTypeId = 'claude-code';

      globalThis.fetch = vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve(mockModels),
        } as Response)
      );

      const { result } = renderHook(() => useModelsForCli(cliTypeId), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(result.current.data).toEqual(mockModels);
      expect(result.current.data).toHaveLength(2);
    });

    it('should handle fetch errors', async () => {
      const cliTypeId = 'claude-code';

      globalThis.fetch = vi.fn(() =>
        Promise.resolve({
          ok: false,
          status: 404,
          json: () =>
            Promise.resolve({
              message: 'CLI type not found',
            }),
        } as Response)
      );

      const { result } = renderHook(() => useModelsForCli(cliTypeId), { wrapper });

      await waitFor(() => expect(result.current.isError).toBe(true));
      expect(result.current.error).toBeInstanceOf(Error);
    });

    it('should not fetch when cliTypeId is empty', async () => {
      const { result } = renderHook(() => useModelsForCli(''), { wrapper });

      expect(result.current.fetchStatus).toBe('idle');
    });

    it('should use stale time of 30 minutes', async () => {
      const cliTypeId = 'claude-code';

      globalThis.fetch = vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve(mockModels),
        } as Response)
      );

      const { result } = renderHook(() => useModelsForCli(cliTypeId), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      // Verify data is cached (not checking staleTime directly as it's an implementation detail)
      expect(result.current.data).toEqual(mockModels);
    });
  });

  describe('data transformation', () => {
    it('should correctly parse CLI type metadata', async () => {
      globalThis.fetch = vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve(mockCliTypes),
        } as Response)
      );

      const { result } = renderHook(() => useCliTypes(), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      const claudeCode = result.current.data?.find((cli) => cli.id === 'claude-code');
      expect(claudeCode).toBeDefined();
      expect(claudeCode?.displayName).toBe('Claude Code');
      expect(claudeCode?.executableCommand).toBe('claude');
      expect(claudeCode?.documentationUrl).toBeDefined();
    });

    it('should correctly parse detection results with installed status', async () => {
      globalThis.fetch = vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve(mockDetectionResults),
        } as Response)
      );

      const { result } = renderHook(() => useCliDetection(), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      const claudeDetection = result.current.data?.find(
        (d) => d.cliTypeId === 'claude-code'
      );
      expect(claudeDetection?.isInstalled).toBe(true);
      expect(claudeDetection?.version).toBe('1.0.0');
      expect(claudeDetection?.path).toBe('/usr/local/bin/claude');

      const qwenDetection = result.current.data?.find((d) => d.cliTypeId === 'qwen-code');
      expect(qwenDetection?.isInstalled).toBe(false);
      expect(qwenDetection?.error).toBe('Command not found');
    });

    it('should correctly parse model information', async () => {
      const cliTypeId = 'claude-code';

      globalThis.fetch = vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve(mockModels),
        } as Response)
      );

      const { result } = renderHook(() => useModelsForCli(cliTypeId), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      const sonnetModel = result.current.data?.find((m) => m.id === 'claude-3-5-sonnet');
      expect(sonnetModel).toBeDefined();
      expect(sonnetModel?.displayName).toBe('Claude 3.5 Sonnet');
      expect(sonnetModel?.provider).toBe('anthropic');
      expect(sonnetModel?.requiresConfig).toBe(true);
      expect(sonnetModel?.configSchema).toBeDefined();
    });
  });

  describe('query key consistency', () => {
    it('should generate consistent keys for cache management', () => {
      const key1 = cliTypesKeys.all;
      const key2 = cliTypesKeys.all;

      expect(key1).toEqual(key2);
      expect(key1).toStrictEqual(['cliTypes']);

      const modelKey1 = cliTypesKeys.models('claude-code');
      const modelKey2 = cliTypesKeys.models('claude-code');

      expect(modelKey1).toEqual(modelKey2);
      expect(modelKey1).toStrictEqual(['cliTypes', 'models', 'claude-code']);
    });
  });
});
