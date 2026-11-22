<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import type { Net } from '../../lib/net.svelte';
  import { t } from '../../lib/prefs.svelte';

  let { group, net }: { group: Group; net: Net } = $props();
  const uid = $props.id();
  const nodes = $derived(
    net.homes.filter((home) => home.owner && home.device !== group.home.device),
  );
  const owner = $derived(group.me?.kind === 'human' && group.me.role === 'owner');
</script>

{#if owner}
  <section class="move" aria-labelledby={`${uid}-title`}>
    <h2 class="sec-title" id={`${uid}-title`}>{t('move.title')}</h2>
    <details class="fold">
      <summary>{t('move.commands')}</summary>
      <div class="fold-body field">
        <p class="hint">{t('move.lead')}</p>
        <p>{t('move.export').replace('{computer}', group.home.name)}</p>
        {#each nodes as node (node.device)}
          <h3 class="sec-title">{node.name}</h3>
          <code>{`solodawn group export ${group.g} group.sqlite --to ${node.device}`}</code>
        {:else}
          <p class="hint">{t('move.none')}</p>
          <code>{`solodawn group export ${group.g} group.sqlite --to DEVICE_ID`}</code>
        {/each}
        <p>{t('move.import')}</p>
        <code>solodawn group import group.sqlite</code>
        <p class="note" data-tone="dawn">{t('move.warning')}</p>
        <p class="hint">{t('move.copy')}</p>
      </div>
    </details>
  </section>
{/if}

<style>
  .move {
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
    padding: var(--sp-4) var(--sp-4) 14px;
    border-top: 1px solid var(--line);
  }
  code {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
