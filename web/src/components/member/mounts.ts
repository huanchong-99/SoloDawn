import type { Mounts } from '../../lib/mounts.svelte';
import MemberPanel from './MemberPanel.svelte';

export default {
  layer: [{ order: 90, component: MemberPanel }],
  'chat.side': [{ order: 10, component: MemberPanel }],
} satisfies Mounts;
