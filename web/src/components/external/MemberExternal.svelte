<script lang="ts">
  import type { Points } from '../../lib/mounts.svelte';
  import { t } from '../../lib/prefs.svelte';
  import SetupDialog from './SetupDialog.svelte';

  let { group, ai }: Points['member.ai'] = $props();
  let open = $state(false);
  const mine = $derived(ai.user === group.home.me.user);
</script>

{#if ai.cli === 'external'}
  <dt>{t('external.agent.type')}</dt>
  <dd class="tool">
    {t('external.agent')}{#if ai.tool}{' · '}{ai.tool}{/if}
  </dd>
  {#if mine}
    <dd class="actions">
      <button type="button" class="btn btn-sm" onclick={() => (open = true)}>
        {t('external.setup.open')}
      </button>
      {#if open}
        <SetupDialog {group} {ai} onclose={() => (open = false)} />
      {/if}
    </dd>
  {/if}
{/if}

<style>
  .tool {
    overflow-wrap: anywhere;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-2);
    grid-column: 1 / -1;
  }
</style>
