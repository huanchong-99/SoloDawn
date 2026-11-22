<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';
  import { presets, PresetView } from './presets.svelte';

  let { group, name }: { group: Group; name: string } = $props();
  let open = $state(false);
  const view = $derived(new PresetView(group));
  const preset = $derived(presets(group).find((row) => row.name === name));
  $effect(() => {
    if (open && !preset) view.sync();
  });
</script>

<details bind:open>
  <summary><Icon name="down" sm /><b>{name}</b></summary>
  {#if open}
    {#if preset}<p class="mem-text">{preset.text}</p>
    {:else if view.failed}<Failure code={view.failed} />
    {:else if view.busy}<p class="hint" role="status">{t('preset.loading')}</p>
    {:else if view.loaded}<p class="hint">{t('preset.deleted')}</p>
    {:else}<p class="hint">{t('preset.offline')}</p>{/if}
  {/if}
</details>

<style>
  summary {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    cursor: pointer;
    list-style: none;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  .mem-text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
