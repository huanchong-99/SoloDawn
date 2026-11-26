<script lang="ts">
  import { tick } from 'svelte';
  import type { CardProps } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { clock } from '../../lib/when';
  import Avatar from '../Avatar.svelte';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** An AI's permission request in any of its five states (contracts/messages.md 2.3 and 3.2). */
  let { group, msg }: CardProps<'approval'> = $props();
  const uid = $props.id();

  type Category = 'command' | 'file' | 'network' | 'other';

  let cardEl: HTMLElement;
  let busy = $state(false);
  let failed = $state('');

  const pending = $derived(msg.state.status === 'pending');
  const canApprove = $derived(msg.body.approvers.includes(group.home.me.user));
  const needsMe = $derived(pending && canApprove);
  const readonly = $derived(!group.home.ctl);
  const canAct = $derived(needsMe && !busy && !readonly);

  const ai = $derived.by(() => {
    const member = group.member(msg.body.ai);
    return member?.kind === 'ai' ? member : undefined;
  });
  const task = $derived(group.task(msg.body.task));
  const aiMeta = $derived.by(() => {
    const details: string[] = [];
    if (ai?.host_name) {
      const own = ai.user === group.home.me.user;
      details.push(own ? t('approval.host.yours').replace('{host}', ai.host_name) : ai.host_name);
    }
    if (task) details.push(t('approval.task').replace('{n}', String(task.num)));
    return details.length ? `· ${details.join(' · ')}` : '';
  });
  const approvers = $derived(
    msg.body.approvers
      .map((uid) => group.personName(uid))
      .filter((name): name is string => Boolean(name))
      .join(t('approval.sep')),
  );
  const actor = $derived('by' in msg.state ? group.nameOf(msg.state.by) : '');
  const at = $derived('at' in msg.state ? clock(msg.state.at, prefs.lang) : '');

  async function settleFocus() {
    await tick();
    const btn = cardEl.querySelector<HTMLElement>('button:not([disabled])');
    if (btn) btn.focus({ preventScroll: true });
  }

  async function decide(action: 'approve' | 'deny') {
    if (!canAct) return;
    const hadFocus = cardEl.contains(document.activeElement);
    busy = true;
    failed = '';
    if (hadFocus) {
      cardEl.tabIndex = -1;
      cardEl.focus({ preventScroll: true });
    }
    const answer = await group.home.command({ t: 'act', g: group.g, msg: msg.id, action });
    busy = false;
    if (answer.err) failed = answer.err.code;
    if (hadFocus) await settleFocus();
  }
</script>

<section bind:this={cardEl} class="card" class:need={needsMe} aria-label={t('approval.kind')}>
  <header class="card-head">
    <Icon name="shield" />
    <span class="kind">{t('approval.kind')}</span>
    <span class="badge">{t(`approval.category.${msg.body.category as Category}`)}</span>
    {#if needsMe}
      <span class="badge solid" data-tone="dawn">{t('approval.status.needs_you')}</span>
    {:else if pending}
      <span class="badge">
        {approvers
          ? t('approval.status.waiting_for').replace('{who}', approvers)
          : t('approval.status.waiting')}
      </span>
    {:else if msg.state.status === 'approved'}
      <span class="badge" data-tone="ok">
        {t('approval.status.approved').replace('{who}', actor).replace('{time}', at)}
      </span>
    {:else if msg.state.status === 'denied'}
      <span class="badge" data-tone="danger">
        {t('approval.status.denied').replace('{who}', actor).replace('{time}', at)}
      </span>
    {:else if msg.state.status === 'reassigned'}
      <span class="badge">{t('approval.status.reassigned').replace('{time}', at)}</span>
    {:else}
      <span class="badge">{t('approval.status.expired').replace('{time}', at)}</span>
    {/if}
  </header>

  <dl class="kv wide">
    <dt>{t('approval.who')}</dt>
    <dd>
      <span class="who">
        <Avatar
          id={ai ? ai.id : msg.body.ai}
          name={ai?.name ?? '?'}
          cli={ai?.cli ?? ''}
          size="sm"
        />
        <span>{group.nameOf(msg.body.ai)}</span>
        {#if aiMeta}<span class="muted">{aiMeta}</span>{/if}
      </span>
    </dd>
    <dt>{t('approval.wants')}</dt>
    <dd class="wants">
      <pre class="cmd">{msg.body.command}</pre>
      {#if msg.body.full !== undefined}
        <p class="note" data-tone="dawn">
          <Icon name="alert" sm />
          <span>{t('approval.truncated').replace('{n}', String(msg.body.full))}</span>
        </p>
      {/if}
    </dd>
    {#if msg.body.why}
      <dt>{t('approval.why')}</dt>
      <dd>{msg.body.why}</dd>
    {/if}
  </dl>

  {#if needsMe}
    <footer class="card-foot">
      <button
        type="button"
        class="btn btn-dawn"
        aria-describedby={`${uid}-hint`}
        disabled={!canAct}
        onclick={() => decide('approve')}
      >
        <Icon name="check" sm /><span>{t('approval.approve')}</span>
      </button>
      <button
        type="button"
        class="btn"
        aria-describedby={`${uid}-hint`}
        disabled={!canAct}
        onclick={() => decide('deny')}
      >
        {t('approval.deny')}
      </button>
      <span class="hint" id={`${uid}-hint`}>
        {t(readonly ? 'approval.hint.offline' : 'approval.hint.once')}
      </span>
    </footer>
    {#if failed}<Failure code={failed} />{/if}
  {:else if pending}
    <footer class="card-foot">
      <button type="button" class="btn" aria-describedby={`${uid}-hint`} disabled>
        {t('approval.approve')}
      </button>
      <button type="button" class="btn" aria-describedby={`${uid}-hint`} disabled>
        {t('approval.deny')}
      </button>
      <span class="hint" id={`${uid}-hint`}>
        <svg class="i sm" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M6 11h12v9H6zM8.5 11V8a3.5 3.5 0 0 1 7 0v3" />
        </svg>
        {approvers
          ? t('approval.hint.locked_who').replace('{who}', approvers)
          : t('approval.hint.locked')}
      </span>
    </footer>
  {:else if msg.state.status === 'approved'}
    <p class="hint">
      {ai ? t('approval.hint.approved').replace('{ai}', ai.name) : t('approval.hint.approved_gone')}
    </p>
  {:else if msg.state.status === 'denied'}
    <p class="hint">
      {ai ? t('approval.hint.denied').replace('{ai}', ai.name) : t('approval.hint.denied_gone')}
    </p>
  {:else if msg.state.status === 'reassigned'}
    <p class="hint">{t('approval.hint.reassigned')}</p>
  {:else}
    <p class="hint">{t('approval.hint.expired')}</p>
  {/if}
</section>

<style>
  .who {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
    min-width: 0;
    vertical-align: middle;
  }
  .wants {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .cmd {
    padding: 8px 10px;
    border-radius: var(--r-sm);
    background: var(--surface-2);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    line-height: 1.5;
  }
  .note {
    display: flex;
    gap: var(--sp-2);
    padding: 10px 12px;
    border-radius: var(--r-md);
    background: var(--tone-soft, var(--surface-2));
    color: var(--text);
    font-size: var(--fs-sm);
  }
  .note :global(.i) {
    color: var(--tone-text, var(--text-3));
    margin-top: 2px;
  }
</style>
