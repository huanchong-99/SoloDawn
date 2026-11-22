import type { Mounts } from '../../lib/mounts.svelte';
import MemberPanel from './MemberPanel.svelte';

export default {
  'chat.side': [{ order: 10, component: MemberPanel }],
} satisfies Mounts;
