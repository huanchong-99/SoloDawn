<script lang="ts">
  import { recent } from '../../lib/groups.svelte';
  import { guideState, guideSteps, openGuide } from '../../lib/guide.svelte';
  import type { Net } from '../../lib/net.svelte';
  import { nodes } from '../../lib/nodes.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Icon from '../Icon.svelte';

  /** Top-bar chip to reopen the guide while closed and unfinished. */
  let { net }: { net: Net } = $props();

  const steps = $derived(guideSteps(nodes.list(net), recent()));
  const doneCount = $derived(steps.filter((s) => s.done).length);
  const allDone = $derived(doneCount === steps.length);
  const shown = $derived(guideState.value === 'closed' && !allDone);
  const parts = $derived(t('guide.chip').split('{done}'));
  const label = $derived(t('guide.chip').replace('{done}', String(doneCount)));
</script>

{#if shown}
  <button
    type="button"
    class="btn btn-sm btn-ghost guide-chip"
    aria-label={label}
    onclick={openGuide}
  >
    <Icon name="compass" sm />
    <span><span class="only-wide">{parts[0]}</span>{doneCount}{parts[1]}</span>
  </button>
{/if}
