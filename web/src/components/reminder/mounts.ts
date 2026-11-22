import type { Mounts } from '../../lib/mounts.svelte';
import ReminderSync from './ReminderSync.svelte';

export default {
  layer: [{ order: 20, component: ReminderSync }],
} satisfies Mounts;
