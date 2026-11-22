import type { Mounts } from '../../lib/mounts.svelte';
import HandbackMeta from './HandbackMeta.svelte';

export default {
  cards: { handback: HandbackMeta },
} satisfies Mounts;
