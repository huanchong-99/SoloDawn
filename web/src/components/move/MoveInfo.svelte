<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import type { Net } from '../../lib/net.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Icon from '../Icon.svelte';

  let { group, net }: { group: Group; net: Net } = $props();
  const uid = $props.id();
  const nodes = $derived(
    net.homes.filter((home) => home.owner && home.device !== group.home.device),
  );
  const owner = $derived(group.me?.role === 'owner');
</script>

{#if owner}
  <section class="move" aria-labelledby={`${uid}-title`}>
    <h2 class="sec-title" id={`${uid}-title`}>{t('move.title')}</h2>
    <details class="fold">
      <summary><Icon name="down" sm />{t('move.commands')}</summary>
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
  /* A disclosure row like the others in the side panel: a chevron and the label in text. */
  .fold > summary {
    color: var(--text);
    font-weight: 500;
  }
  code {
    display: block;
    padding: 10px 12px;
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    background: var(--surface-2);
    font-size: var(--fs-sm);
    line-height: 1.5;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
