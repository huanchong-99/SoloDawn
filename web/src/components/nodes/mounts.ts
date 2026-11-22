import type { Mounts } from '../../lib/mounts.svelte';
import Indicator from './Indicator.svelte';

export default {
  bar: [{ order: 10, component: Indicator }],
  'chat.head': [{ order: 10, component: Indicator }],
} satisfies Mounts;
