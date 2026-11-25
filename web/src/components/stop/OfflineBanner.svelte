<script lang="ts">
  import { homes } from '../../lib/groups.svelte';
  import { nodes } from '../../lib/nodes.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { route } from '../../lib/route.svelte';
  import { computers, names, offlineAIs } from '../../lib/stop';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** Under the top bar while the owner's AIs are offline on a computer that can be reached, with
   *  the way back. The computers page says it on the computer itself. */
  let busy = $state(false);
  let failed = $state('');

  const owned = $derived(homes.filter((home) => home.owner));
  const stopped = $derived(computers(homes).stopped.filter((home) => home.ctl));

  // Another browser of the owner may take them offline, or back: the node pushes no host.info, and
  // the groups show the AIs change. A computer that connects after the page opened is read too.
  const shown = $derived(owned.map((home) => offlineAIs(home, homes)).join('\n'));
  $effect(() => {
    void shown;
    nodes.sync(owned, true);
  });

  async function online() {
    busy = true;
    failed = '';
    const answers = await Promise.all(stopped.map((home) => nodes.setOffline(home, false)));
    failed = answers.find((answer) => answer.err)?.err?.code ?? '';
    busy = false;
    if (!failed) document.querySelector('main')!.focus();
  }
</script>

{#if stopped.length && route.page !== 'nodes'}
  <div class="off">
    <span class="glyph"><Icon name="power" /></span>
    <span class="grow" role="status">
      {t('stop.offline.banner').replace('{names}', () => names(stopped, prefs.lang))}
    </span>
    {#if failed}<Failure code={failed} />{/if}
    <button
      type="button"
      class="btn btn-sm"
      disabled={busy}
      aria-busy={busy}
      onclick={() => void online()}
    >
      {t('stop.online')}
    </button>
  </div>
{/if}

<style>
  .off {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--sp-2) var(--sp-3);
    padding: 8px var(--sp-4);
    border-bottom: 1px solid color-mix(in srgb, var(--danger) 30%, var(--line));
    background: var(--danger-soft);
    font-size: var(--fs-sm);
  }
  .glyph {
    display: grid;
    color: var(--danger);
  }
  .grow {
    flex-basis: 180px;
  }
</style>
