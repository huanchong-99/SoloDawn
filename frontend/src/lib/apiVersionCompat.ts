import { ApiError } from '@/lib/api';

/**
 * Returns `true` when the quality-gate backend endpoints are available.
 * A 404 response indicates the backend predates the quality-gate feature,
 * so we degrade gracefully instead of surfacing an error.
 *
 * W2-31-08: This 404-based feature detection is intentional and retained
 * for compatibility with older self-hosted backend deployments that do not
 * yet expose the quality-gate routes. The frontend and backend ship in the
 * same repository but are not always upgraded in lockstep by users, so a
 * missing endpoint must degrade to "feature unavailable" rather than an
 * error toast. If/when we gain a first-class feature-capability endpoint
 * (e.g. `GET /api/capabilities`), prefer that and remove this heuristic.
 */
export function isQualityGateAvailable(error: unknown): boolean {
  if (error instanceof ApiError && error.status === 404) return false;
  if (error instanceof Response && error.status === 404) return false;
  if (
    error &&
    typeof error === 'object' &&
    'status' in error &&
    (error as { status: unknown }).status === 404
  )
    return false;
  return true;
}
