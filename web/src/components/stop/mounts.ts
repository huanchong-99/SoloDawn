// What the area `stop` mounts: the emergency stop (planning/product-rules.md R-64) in the top bar
// and, on a phone, in a chat's head; the notice of a paused group; the notice of AIs taken offline.
import type { Mounts } from '../../lib/mounts.svelte';
import OfflineBanner from './OfflineBanner.svelte';
import PausedNotice from './PausedNotice.svelte';
import StopButton from './StopButton.svelte';

export default {
  bar: [{ order: 20, component: StopButton }],
  banner: [{ order: 20, component: OfflineBanner }],
  'chat.head': [{ order: 20, component: StopButton }],
  'chat.write': [{ order: 10, component: PausedNotice }],
} satisfies Mounts;
