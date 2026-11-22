import type { Mounts } from '../../lib/mounts.svelte';
import TaskCard from './TaskCard.svelte';

export default {
  cards: { task: TaskCard },
} satisfies Mounts;
