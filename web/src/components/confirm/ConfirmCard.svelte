<script lang="ts">
  import { confirmSentence, confirmStatus } from '../../lib/confirm';
  import type { CardProps } from '../../lib/mounts.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  let { group, msg }: CardProps<'confirm'> = $props();
  let card: HTMLElement;
  let busy = $state(false);
  let failed = $state('');

  const status = $derived(msg.state.status);
  const needsMe = $derived(status === 'pending' && msg.body.to.includes(group.home.me.user));
  const tones = { pending: undefined, expired: undefined, confirmed: 'ok', denied: 'danger' };

  async function decide(action: 'confirm' | 'deny') {
    card.tabIndex = -1;
    card.focus({ preventScroll: true });
    busy = true;
    failed = '';
    const answer = await group.home.command({ t: 'act', g: group.g, msg: msg.id, action });
    busy = false;
    if (answer.err) failed = answer.err.code;
  }
</script>

<section bind:this={card} class="card" class:need={needsMe} aria-label={t('chat.kind.confirm')}>
  <header class="card-head">
    <Icon name="shield" />
    <span class="kind">{t('chat.kind.confirm')}</span>
    {#if needsMe}
      <span class="badge solid" data-tone="dawn">{t('confirm.needs_you')}</span>
    {:else}
      <span class="badge" data-tone={tones[status]}>{confirmStatus(group, msg)}</span>
    {/if}
  </header>

  <p>{confirmSentence(group, msg)}</p>

  {#if needsMe}
    <footer class="card-foot">
      <button
        type="button"
        class="btn btn-dawn"
        disabled={busy || !group.home.ctl}
        onclick={() => decide('confirm')}
      >
        <Icon name="check" sm /><span>{t('confirm.yes')}</span>
      </button>
      <button
        type="button"
        class="btn"
        disabled={busy || !group.home.ctl}
        onclick={() => decide('deny')}
      >
        {t('confirm.no')}
      </button>
      {#if !group.home.ctl}<span class="hint">{t('confirm.offline')}</span>{/if}
    </footer>
    {#if failed}<Failure code={failed} />{/if}
  {/if}
</section>
