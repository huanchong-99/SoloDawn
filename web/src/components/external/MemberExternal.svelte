<script lang="ts">
  import type { Points } from '../../lib/mounts.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Failure from '../Failure.svelte';
  import SetupDialog from './SetupDialog.svelte';

  let { group, ai }: Points['member.ai'] = $props();
  let open = $state(false);
  let confirming = $state(false);
  let busy = $state(false);
  let failed = $state('');
  const mine = $derived(ai.user === group.home.me.user);

  async function remove(event: MouseEvent & { currentTarget: HTMLButtonElement }) {
    if (!confirming) {
      event.currentTarget.focus();
      confirming = true;
      return;
    }
    confirming = false;
    busy = true;
    failed = '';
    const answer = await group.home.command({ t: 'member', g: group.g, op: 'remove', id: ai.id });
    busy = false;
    if (answer.err) failed = answer.err.code;
  }
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
      <button
        type="button"
        class="btn btn-sm {confirming ? 'btn-danger' : 'btn-ghost'}"
        disabled={busy || !group.home.ctl}
        onclick={remove}
        onblur={() => (confirming = false)}
        onkeydown={(event) => {
          if (confirming && event.key === 'Escape') {
            event.preventDefault();
            confirming = false;
          }
        }}
      >
        {t(confirming ? 'external.remove.confirm' : 'external.remove')}
      </button>
      {#if failed === 'conflict'}
        <p class="hint" role="alert">{t('external.remove.conflict')}</p>
      {:else if failed}
        <Failure code={failed} />
      {/if}
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
