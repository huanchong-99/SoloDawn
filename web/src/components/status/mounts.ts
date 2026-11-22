// What the area `status` mounts: the other-tab notice under the top bar, and the unreachable-home
// read-only notice above a chat's main line.
import type { Mounts } from '../../lib/mounts.svelte';
import HomeBanner from './HomeBanner.svelte';
import TabBanner from './TabBanner.svelte';

export default {
  banner: [{ order: 10, component: TabBanner }],
  'chat.top': [{ order: 10, component: HomeBanner }],
} satisfies Mounts;
