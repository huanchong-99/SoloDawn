// Cards and rows owned by the thread area: scored reviews and system notices. A report is its
// text: the assignee's own account of the work (contracts/messages.md 2.7).
import type { Mounts } from '../../lib/mounts.svelte';
import ReviewBlock from './ReviewBlock.svelte';
import SystemRow from './SystemRow.svelte';

export default {
  cards: {
    review: ReviewBlock,
    system: SystemRow,
  },
} satisfies Mounts;
