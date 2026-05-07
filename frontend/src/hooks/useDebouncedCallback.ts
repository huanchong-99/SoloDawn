import { useRef, useEffect } from 'react';

/**
 * Returns a debounced version of the callback that delays invocation
 * until after `delay` milliseconds have elapsed since the last call.
 * Also returns a cancel function to clear any pending invocation.
 *
 * Identity stability (W2-40): the returned `debounced` and `cancel` functions
 * are stable for the lifetime of the component — they are stored in
 * `useRef` and never recreated, so they are safe to include in effect
 * dependency arrays without causing re-runs. The latest `callback` and
 * `delay` are read via refs that are kept in sync with the props, so
 * changing them does not invalidate the debounced function identity.
 */
export function useDebouncedCallback<Args extends unknown[]>(
  callback: (...args: Args) => void,
  delay: number
): { debounced: (...args: Args) => void; cancel: () => void } {
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const callbackRef = useRef(callback);
  const delayRef = useRef(delay);

  // Keep callback ref up to date
  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  useEffect(() => {
    delayRef.current = delay;
  }, [delay]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
    };
  }, []);

  // Return stable function reference
  const debouncedRef = useRef((...args: Args) => {
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }
    timeoutRef.current = setTimeout(() => {
      callbackRef.current(...args);
    }, delayRef.current);
  });

  // Cancel function to clear pending timeout
  const cancelRef = useRef(() => {
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    }
  });

  return { debounced: debouncedRef.current, cancel: cancelRef.current };
}
