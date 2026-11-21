// What the area `approval` mounts: the waiting-on-you bar above a chat's main line, and the card
// of an approval message in the main line and in threads.
import type { Mounts } from '../../lib/mounts.svelte';
import ApprovalCard from './ApprovalCard.svelte';
import WaitingBar from './WaitingBar.svelte';

export default {
  'chat.top': [{ order: 20, component: WaitingBar }],
  cards: { approval: ApprovalCard },
} satisfies Mounts;
