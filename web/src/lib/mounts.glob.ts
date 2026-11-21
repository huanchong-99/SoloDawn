// What each area mounts, by path, for mounts.svelte.ts: `components/<area>/mounts.ts`. A module
// that only holds an `import.meta.glob` is named `*.glob.ts` and is not mutated; it holds no
// logic (Stryker would wrap the pattern, which Vite must read as written).
import type { Mounts } from './mounts.svelte';

export const files = import.meta.glob<Mounts>('../components/*/mounts.ts', {
  eager: true,
  import: 'default',
});
