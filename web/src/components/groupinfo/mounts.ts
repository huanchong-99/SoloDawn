import type { Mounts } from '../../lib/mounts.svelte';
import GroupInfoPanel from './GroupInfoPanel.svelte';

export default {
  'chat.side': [{ order: 20, component: GroupInfoPanel }],
} satisfies Mounts;
