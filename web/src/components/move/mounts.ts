import type { Mounts } from '../../lib/mounts.svelte';
import MoveInfo from './MoveInfo.svelte';

export default {
  'chat.side': [{ order: 30, component: MoveInfo }],
} satisfies Mounts;
