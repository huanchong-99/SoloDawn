import type { Mounts } from '../../lib/mounts.svelte';
import KnowledgePanel from './KnowledgePanel.svelte';

export default {
  'chat.side': [{ order: 22, component: KnowledgePanel }],
} satisfies Mounts;
