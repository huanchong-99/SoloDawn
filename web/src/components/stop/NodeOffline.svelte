<script lang="ts">
  import { tick } from 'svelte';
  import type { Home } from '../../lib/groups.svelte';
  import { nodes } from '../../lib/nodes.svelte';
  import { t } from '../../lib/prefs.svelte';
  import { isOffline } from '../../lib/stop';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** On the card of one's own computer: takes all AIs there offline, confirmed in place, and back
   *  online with one press. */
  let { home }: { home: Home } = $props();

  let strip: HTMLElement;
  let sure = $state(false);
  let busy = $state(false);
  let failed = $state('');

  const offline = $derived(isOffline(home));
  const phase = $derived(offline ? 'offline' : sure ? 'sure' : 'on');

  /** The buttons change place with what they ask, so focus waits on the strip. */
  async function ask(next: boolean) {
    sure = next;
    failed = '';
    await tick();
    strip.focus();
  }

  async function set(next: boolean) {
    busy = true;
    failed = '';
    strip.focus();
    failed = (await nodes.setOffline(home, next)).err?.code ?? '';
    busy = false;
    sure = false;
  }
</script>

<div
  class="strip"
  class:hot={phase !== 'on'}
  role="group"
  aria-label={t('stop.node.title')}
  tabindex="-1"
  bind:this={strip}
>
  <div class="grow" aria-live="polite">
    {#if phase === 'offline'}
      <b>{t('stop.node.offline')}</b>
      <p>{t('stop.node.waits')}</p>
    {:else}
      <b>{t('stop.node.title')}</b>
      <p>{t(phase === 'sure' ? 'stop.node.confirm' : 'stop.node.body')}</p>
    {/if}
    {#if failed}<Failure code={failed} />{/if}
  </div>
  <div class="acts">
    {#if phase === 'offline'}
      <button type="button" class="btn" disabled={busy} onclick={() => void set(false)}>
        {t('stop.online')}
      </button>
    {:else if phase === 'sure'}
      <!-- The press that asked is under Cancel now: a second one by accident does nothing. -->
      <button
        type="button"
        class="btn btn-danger"
        disabled={busy}
        aria-busy={busy}
        onclick={() => void set(true)}
      >
        <Icon name="power" sm />{t('stop.mine.action')}
      </button>
      <button type="button" class="btn" disabled={busy} onclick={() => void ask(false)}>
        {t('stop.node.cancel')}
      </button>
    {:else}
      <button type="button" class="btn" onclick={() => void ask(true)}>
        {t('stop.mine.action')}
      </button>
    {/if}
  </div>
</div>

<style>
  .strip {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--sp-2) var(--sp-4);
    padding: var(--sp-3) var(--sp-4);
    border-top: 1px solid var(--line);
    font-size: var(--fs-sm);
    outline-offset: -2px;
  }
  .hot {
    background: var(--danger-soft);
  }
  .grow {
    flex-basis: 220px;
  }
  b {
    font-weight: 500;
  }
  p {
    color: var(--text-2);
  }
  .acts {
    display: flex;
    gap: var(--sp-2);
    margin-left: auto;
  }
</style>
