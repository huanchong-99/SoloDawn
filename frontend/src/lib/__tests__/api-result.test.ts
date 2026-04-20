import { afterEach, describe, expect, it, vi } from 'vitest';
import { attemptsApi } from '../api';

type PushErrorBody = {
  error?: { type: string };
  error_data?: { type: string };
  message: string;
};

const conflictResponseInit = {
  status: 409,
  statusText: 'Conflict',
  headers: { 'Content-Type': 'application/json' },
};

function stubPushFailure(body: PushErrorBody) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ success: false, ...body }), conflictResponseInit))
  );
}

async function pushAttempt() {
  return attemptsApi.push('attempt-id', {
    repo_id: 'repo-id',
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('handleApiResponseAsResult', () => {
  it('preserves structured error_data for non-2xx responses', async () => {
    const structuredError = { type: 'force_push_required' };

    stubPushFailure({
      message: 'Need force push',
      error_data: structuredError,
    });

    const result = await pushAttempt();

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error).toEqual(structuredError);
      expect(result.message).toBe('Need force push');
    }
  });

  it('falls back to the `error` field when `error_data` is absent', async () => {
    const structuredError = { type: 'force_push_required' };

    stubPushFailure({
      message: 'Need force push',
      error: structuredError,
    });

    const result = await pushAttempt();

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error).toEqual(structuredError);
      expect(result.message).toBe('Need force push');
    }
  });

  it('prefers `error_data` over `error` when both are present', async () => {
    const primaryError = { type: 'primary' };
    const legacyError = { type: 'legacy' };

    stubPushFailure({
      message: 'Conflict',
      error_data: primaryError,
      error: legacyError,
    });

    const result = await pushAttempt();

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error).toEqual(primaryError);
    }
  });
});
