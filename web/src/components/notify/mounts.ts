// What the area `notify` mounts: live in-page notifications in the shell's overlay layer.
import type { Mounts } from '../../lib/mounts.svelte';
import Notifications from './Notifications.svelte';

export default {
  layer: [{ order: 10, component: Notifications }],
} satisfies Mounts;
