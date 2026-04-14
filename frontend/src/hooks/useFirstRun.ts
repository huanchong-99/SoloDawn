import { useCallback } from 'react';
import { useUserSystem } from '@/components/ConfigProvider';

/**
 * Hook for detecting and managing first-run state.
 *
 * Checks whether the application is running in "installer mode" (standalone
 * Windows installation) and whether the first-run wizard has been completed.
 */
export function useFirstRun() {
  const { config, updateAndSaveConfig } = useUserSystem();

  const isFirstRun = !(config as Record<string, unknown>)?.first_run_completed;

  // [W2-40] `updateAndSaveConfig` dependency: this function is provided by
  // `useUserSystem()` and may change identity across renders, which would
  // invalidate the `completeFirstRun` memoization. Accepted as-is because
  // (a) `completeFirstRun` is only invoked imperatively at most once per
  // session (the first-run wizard completion), so identity churn has no
  // measurable cost, and (b) always closing over the latest config setter
  // is safer than caching a stale reference.
  const completeFirstRun = useCallback(async () => {
    await updateAndSaveConfig({
      first_run_completed: true,
    } as Record<string, unknown>);
  }, [updateAndSaveConfig]);

  return { isFirstRun, completeFirstRun };
}
