import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { BaseCodingAgent } from 'shared/types';
import { useAgentAvailability } from './useAgentAvailability';

const { mockCheckAgentAvailability } = vi.hoisted(() => ({
  mockCheckAgentAvailability: vi.fn(),
}));

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api');
  return {
    ...actual,
    configApi: {
      ...actual.configApi,
      checkAgentAvailability: mockCheckAgentAvailability,
    },
  };
});

describe('useAgentAvailability', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCheckAgentAvailability.mockRejectedValue(new Error('check failed'));
  });

  it('uses latest error notifier via ref without re-triggering effect', async () => {
    const onErrorA = vi.fn();
    const onErrorB = vi.fn();

    const { rerender } = renderHook(
      ({ onError, refreshToken }) =>
        useAgentAvailability(BaseCodingAgent.CODEX, { onError }, refreshToken),
      {
        initialProps: {
          onError: onErrorA,
          refreshToken: 0,
        },
      }
    );

    await waitFor(() => {
      expect(onErrorA).toHaveBeenCalledTimes(1);
    });
    expect(mockCheckAgentAvailability).toHaveBeenCalledTimes(1);

    // Changing onError alone should NOT re-trigger the effect (no infinite loop)
    rerender({ onError: onErrorB, refreshToken: 0 });
    expect(mockCheckAgentAvailability).toHaveBeenCalledTimes(1);

    // Trigger recheck via refreshToken - should use latest error handler (onErrorB)
    rerender({ onError: onErrorB, refreshToken: 1 });

    await waitFor(() => {
      expect(onErrorB).toHaveBeenCalledTimes(1);
    });
    expect(mockCheckAgentAvailability).toHaveBeenCalledTimes(2);
  });
});
