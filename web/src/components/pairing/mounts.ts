// Captures a `#pair=<secret>` link before `route.svelte.ts` reads the initial hash.
import type { Mounts } from '../../lib/mounts.svelte';
import { capturePairHash } from '../../lib/onboard.svelte';

capturePairHash();
addEventListener('hashchange', capturePairHash);

export default {} satisfies Mounts;
