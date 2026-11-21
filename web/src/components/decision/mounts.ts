import type { Mounts } from '../../lib/mounts.svelte';
import DecisionCard from './DecisionCard.svelte';

export default {
  cards: { decision: DecisionCard },
} satisfies Mounts;
