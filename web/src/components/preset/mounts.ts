import type { Mounts } from '../../lib/mounts.svelte';
import PresetPanel from './PresetPanel.svelte';

export default {
  'chat.side': [{ order: 30, component: PresetPanel }],
} satisfies Mounts;
