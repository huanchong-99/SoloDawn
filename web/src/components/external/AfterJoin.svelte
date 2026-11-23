<script lang="ts">
  import type { Points } from '../../lib/mounts.svelte';
  import SetupDialog from './SetupDialog.svelte';

  let { group, candidate, name }: Points['member.joined'] = $props();
  let open = $state(true);
  const ai = $derived(group.members.find((member) => member.name === name));
</script>

{#if open && candidate.agent === 'external' && ai?.kind === 'ai' && ai.host === candidate.device && ai.cli === 'external'}
  <SetupDialog {group} {ai} autoCheck onclose={() => (open = false)} />
{/if}
