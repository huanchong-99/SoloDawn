import type { Mounts } from '../../lib/mounts.svelte';
import CallBlock from './CallBlock.svelte';

export default {
  cards: { call: CallBlock },
} satisfies Mounts;
