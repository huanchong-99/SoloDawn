import type { Mounts } from '../../lib/mounts.svelte';
import SummaryCard from './SummaryCard.svelte';

export default {
  cards: { summary: SummaryCard },
} satisfies Mounts;
