<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import type { Candidate, Draft } from '../../lib/members';
  import ModelFields from './ModelFields.svelte';
  import { listedModels } from './model';

  /** Model and level in the dialog for adding an AI, from the chosen computer's list. */
  let {
    candidate,
    draft = $bindable(),
  }: { group: Group; candidate: Candidate | undefined; draft: Draft } = $props();

  const models = $derived(candidate && (listedModels(candidate.home, candidate.agent) ?? []));

  $effect.pre(() => {
    void candidate?.key;
    Object.assign(draft, { model: '', effort: '', tier: 'standard' });
  });
</script>

{#if models}
  <ModelFields bind:draft {models} suggest />
{/if}
