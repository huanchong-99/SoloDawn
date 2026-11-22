<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import Icon from '../Icon.svelte';

  /** Read-only notice above a group's main line while its home cannot be reached
   *  (planning/flows/sync.md section H; step-1-acceptance.md A2.6). */
  let { group }: { group: Group } = $props();
</script>

{#if !group.home.ctl}
  <div class="home-ban" role="status">
    <Icon name="home" />
    <span class="grow">
      <b>
        {group.home.name
          ? t('status.down.host').replace('{home}', group.home.name)
          : t('status.down')}
      </b>{prefs.lang === 'zh-CN' ? '' : ' '}{t('status.readonly')}
    </span>
  </div>
{/if}

<style>
  .home-ban {
    display: flex;
    align-items: center;
    gap: var(--sp-3);
    padding: 8px var(--sp-4);
    background: var(--danger-soft);
    border-bottom: 1px solid color-mix(in srgb, var(--danger) 35%, var(--line));
    font-size: var(--fs-sm);
    overflow-wrap: anywhere;
  }
  .home-ban :global(.i),
  .home-ban b {
    color: var(--danger);
  }
</style>
