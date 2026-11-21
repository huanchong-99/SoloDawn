import type { Mounts } from '../../lib/mounts.svelte';
import BriefCard from './BriefCard.svelte';

export default {
  cards: { brief: BriefCard },
} satisfies Mounts;
