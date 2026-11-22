// What the area `status` mounts: the other-tab notice under the top bar.
import type { Mounts } from '../../lib/mounts.svelte';
import TabBanner from './TabBanner.svelte';

export default {
  banner: [{ order: 10, component: TabBanner }],
} satisfies Mounts;
