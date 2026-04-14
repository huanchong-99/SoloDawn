import { describe, expect, it, vi } from 'vitest';
import { attemptsApi } from '../api';

describe('handleApiResponseAsResult', () => {
  it('preserves structured error_data for non-2xx responses', async () => {
    const structuredError = { type: 'force_push_required' };

    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              success: false,
              message: 'Need force push',
              error_data: structuredError,
            }),
            {
              status: 409,
              statusText: 'Conflict',
              headers: { 'Content-Type': 'application/json' },
            }
          )
      )
    );

    const result = await attemptsApi.push('attempt-id', {
      repo_id: 'repo-id',
    });

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error).toEqual(structuredError);
      expect(result.message).toBe('Need force push');
    }

    vi.unstubAllGlobals();
  });

  it('falls back to the `error` field when `error_data` is absent', async () => {
    const structuredError = { type: 'force_push_required' };

    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              success: false,
              message: 'Need force push',
              error: structuredError,
            }),
            {
              status: 409,
              statusText: 'Conflict',
              headers: { 'Content-Type': 'application/json' },
            }
          )
      )
    );

    const result = await attemptsApi.push('attempt-id', {
      repo_id: 'repo-id',
    });

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error).toEqual(structuredError);
      expect(result.message).toBe('Need force push');
    }

    vi.unstubAllGlobals();
  });

  it('prefers `error_data` over `error` when both are present', async () => {
    const primaryError = { type: 'primary' };
    const legacyError = { type: 'legacy' };

    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              success: false,
              message: 'Conflict',
              error_data: primaryError,
              error: legacyError,
            }),
            {
              status: 409,
              statusText: 'Conflict',
              headers: { 'Content-Type': 'application/json' },
            }
          )
      )
    );

    const result = await attemptsApi.push('attempt-id', {
      repo_id: 'repo-id',
    });

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error).toEqual(primaryError);
    }

    vi.unstubAllGlobals();
  });
});
