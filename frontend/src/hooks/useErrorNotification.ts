import { useCallback } from 'react';

export interface ErrorNotificationOptions {
  onError?: (error: Error) => void;
  context?: string;
}

export interface ErrorNotificationHandle {
  notifyError: (error: unknown, contextOverride?: string) => void;
  wrapAsync: <Args extends unknown[], Result>(
    fn: (...args: Args) => Promise<Result>,
    contextOverride?: string
  ) => (...args: Args) => Promise<Result>;
}

const toError = (error: unknown, context?: string): Error => {
  if (error instanceof Error) {
    return error;
  }
  if (typeof error === 'string') {
    return new Error(context ? `${context}: ${error}` : error);
  }
  return new Error(context ? `${context}: Unknown error` : 'Unknown error');
};

/**
 * Normalizes errors and dispatches them through the optional onError handler.
 */
export function useErrorNotification(
  options: ErrorNotificationOptions = {}
): ErrorNotificationHandle {
  const { onError, context } = options;

  const notifyError = useCallback(
    (error: unknown, contextOverride?: string) => {
      const ctx = contextOverride ?? context;
      const normalized = toError(error, ctx);
      onError?.(normalized);
    },
    [context, onError]
  );

  // [W2-40] `wrapAsync` depends on `notifyError`, which in turn depends on
  // the consumer-provided `onError` and `context`. Each of those can change
  // between renders, which will cause `wrapAsync` to change identity too.
  // Accepted: wrapAsync is intended to be invoked fresh per async call
  // (not memoized by consumers as a dependency), and always pulling the
  // latest `notifyError` is required so errors are dispatched through the
  // caller's current handler rather than a stale one captured on first
  // render.
  const wrapAsync = useCallback(
    <Args extends unknown[], Result>(
      fn: (...args: Args) => Promise<Result>,
      contextOverride?: string
    ) =>
      async (...args: Args) => {
        try {
          return await fn(...args);
        } catch (error) {
          notifyError(error, contextOverride);
          throw error;
        }
      },
    [notifyError]
  );

  return { notifyError, wrapAsync };
}
