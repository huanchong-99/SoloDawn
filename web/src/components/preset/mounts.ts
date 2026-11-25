import type { Mounts } from '../../lib/mounts.svelte';
import PresetPanel from './PresetPanel.svelte';

export default {
  'chat.side': [{ order: 21, component: PresetPanel, project: true }],
} satisfies Mounts;
