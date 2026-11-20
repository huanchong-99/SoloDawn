// Motion in script. The durations and easings are in tokens.css; what CSS cannot say is here.
import { prefersReducedMotion } from 'svelte/motion';

/** A damped spring that starts at 0, overshoots by about 4% and approaches 1.
 *  --spring in tokens.css is this curve, sampled. */
export const spring = (t: number) => 1 - Math.exp(-8 * t) * (Math.cos(8 * t) + Math.sin(8 * t));

/** Makes a change to the whole page (theme, language) under a cross-fade, which animates opacity
 *  only. Firefox before 144 and Safari before 18 have no view transitions, and a person may ask
 *  for reduced motion: there the change is immediate. */
export function shift(change: () => void) {
  if (prefersReducedMotion.current || !('startViewTransition' in document)) return change();
  document.startViewTransition(change);
}
