import type { Mounts } from '../../lib/mounts.svelte';
import AddModel from './AddModel.svelte';
import MemberModel from './MemberModel.svelte';

export default {
  'member.ai': [{ order: 10, component: MemberModel }],
  'member.add': [{ order: 10, component: AddModel }],
} satisfies Mounts;
