import type { Mounts } from '../../lib/mounts.svelte';
import GroupInfoPanel from './GroupInfoPanel.svelte';
import GroupDetails from './GroupDetails.svelte';

export default {
  'chat.side': [
    { order: 19, component: GroupDetails },
    { order: 20, component: GroupInfoPanel },
  ],
} satisfies Mounts;
