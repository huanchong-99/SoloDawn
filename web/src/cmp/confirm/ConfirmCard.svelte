<script lang="ts">
  import type { CardProps } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { clock } from '../../lib/when';
  import Failure from '../../components/Failure.svelte';
  import Icon from '../../components/Icon.svelte';

  let { group, msg }: CardProps<'confirm'> = $props();
  let card: HTMLElement;
  let busy = $state(false);
  let failed = $state('');

  const status = $derived(msg.state.status);
  const needsMe = $derived(status === 'pending' && msg.body.to.includes(group.home.me.user));
  const ai = $derived(group.member(msg.body.ai)?.name ?? t('chat.gone'));
  const recipients = $derived(
    msg.body.to.map((user) => group.personName(user) ?? t('chat.gone')).join(t('confirm.sep')),
  );
  const actor = $derived(
    'by' in msg.state ? (group.member(msg.state.by)?.name ?? t('chat.gone')) : '',
  );
  const at = $derived('at' in msg.state ? clock(msg.state.at, prefs.lang) : '');

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
    {:else if status === 'pending'}
      <span class="badge">{t('confirm.waiting').replace('{who}', () => recipients)}</span>
    {:else}
      <span
        class="badge"
        data-tone={status === 'expired' ? undefined : status === 'confirmed' ? 'ok' : 'danger'}
      >
        {t(`confirm.${status}`)
          .replace('{time}', at)
          .replace('{who}', () => actor)}
      </span>
    {/if}
  </header>

  <p>
    {t(`confirm.request.${msg.body.what as 'direct' | 'ask' | 'watch'}`).replace('{ai}', () => ai)}
  </p>

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
