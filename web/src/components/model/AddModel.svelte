<script lang="ts">
  import { untrack } from 'svelte';
  import type { Group } from '../../lib/groups.svelte';
  import type { Candidate, Draft } from '../../lib/members';
  import { defaultsOf, unsetProfile } from '../defaults/defaults';
  import ModelFields from './ModelFields.svelte';
  import { listedModels } from './model';

  /** Model and level in the dialog for adding an AI, from the chosen computer's list. */
  let {
    candidate,
    draft = $bindable(),
  }: { group: Group; candidate: Candidate | undefined; draft: Draft } = $props();

  const models = $derived(candidate && (listedModels(candidate.home, candidate.agent) ?? []));

  $effect.pre(() => {
    const current = candidate;
    void current?.key;
    const defaults = current ? untrack(() => defaultsOf(current.home, current.agent)) : undefined;
    const { model, effort, tier } = defaults ?? unsetProfile;
    Object.assign(draft, { model, effort, tier });
  });
</script>

{#if models}
  <ModelFields bind:draft {models} suggest />
{/if}
