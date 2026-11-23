import type { Mounts } from '../../lib/mounts.svelte';
import AddExternal from './AddExternal.svelte';
import AfterJoin from './AfterJoin.svelte';
import MemberExternal from './MemberExternal.svelte';

export default {
  'member.ai': [{ order: 5, component: MemberExternal }],
  'member.add': [{ order: 5, component: AddExternal }],
  'member.joined': [{ order: 10, component: AfterJoin }],
} satisfies Mounts;
