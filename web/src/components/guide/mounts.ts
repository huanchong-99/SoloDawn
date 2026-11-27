// What the area `guide` mounts: the top-bar chip and the welcome / guide dock overlay.
import type { Mounts } from '../../lib/mounts.svelte';
import GuideChip from './GuideChip.svelte';
import GuideDock from './GuideDock.svelte';
import Welcome from './Welcome.svelte';

export default {
  bar: [{ order: 0, component: GuideChip }],
  layer: [
    { order: 0, component: Welcome },
    { order: 10, component: GuideDock },
  ],
} satisfies Mounts;
