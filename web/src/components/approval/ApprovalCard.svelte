<script lang="ts">
  import type { CardProps } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { clock } from '../../lib/when';
  import Avatar from '../Avatar.svelte';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** An AI's permission request in any of its five states (contracts/messages.md 2.3 and 3.2). */
  let { group, msg }: CardProps<'approval'> = $props();

  const categories = ['command', 'file', 'network', 'other'] as const;

  let busy = $state(false);
  let failed = $state('');

  const category = $derived(categories.find((each) => each === msg.body.category) ?? 'other');
  const pending = $derived(msg.state.status === 'pending');
  const canApprove = $derived(msg.body.approvers.includes(group.home.me.user));
  const needsMe = $derived(pending && canApprove);
  const readonly = $derived(!group.home.ctl);
  const canAct = $derived(needsMe && !busy && !readonly);

  const ai = $derived.by(() => {
    const member = group.member(msg.body.ai);
    return member?.kind === 'ai' ? member : undefined;
  });
  const task = $derived.by(() => {
    const row = group.rows.get(`task ${msg.body.task}`);
    return row && row.r === 'task' && !('gone' in row) ? row : undefined;
  });
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
      .map((uid) => group.members.find((m) => m.kind === 'human' && m.user === uid)?.name)
      .filter((name): name is string => Boolean(name))
      .join(t('approval.sep')),
  );
  const actor = $derived(
    'by' in msg.state ? (group.member(msg.state.by)?.name ?? t('approval.gone')) : '',
  );
  const at = $derived('at' in msg.state ? clock(msg.state.at, prefs.lang) : '');

  async function decide(action: 'approve' | 'deny') {
    if (!canAct) return;
    busy = true;
    failed = '';
    const answer = await group.home.command({ t: 'act', g: group.g, msg: msg.id, action });
    busy = false;
    if (answer.err) failed = answer.err.code;
  }
</script>

<section class="card" class:need={needsMe} aria-label={t('approval.kind')}>
  <header class="card-head">
    <Icon name="shield" />
    <span class="kind">{t('approval.kind')}</span>
    <span class="badge">{t(`approval.category.${category}`)}</span>
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
        <span>{ai?.name ?? t('approval.gone')}</span>
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
        disabled={!canAct}
        onclick={() => decide('approve')}
      >
        <Icon name="check" sm /><span>{t('approval.approve')}</span>
      </button>
      <button type="button" class="btn" disabled={!canAct} onclick={() => decide('deny')}>
        {t('approval.deny')}
      </button>
      <span class="hint">{t(readonly ? 'approval.hint.offline' : 'approval.hint.once')}</span>
    </footer>
    {#if failed}<Failure code={failed} />{/if}
  {:else if pending}
    <footer class="card-foot">
      <button type="button" class="btn" disabled>{t('approval.approve')}</button>
      <button type="button" class="btn" disabled>{t('approval.deny')}</button>
      <span class="hint">
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
  .card-head {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    flex-wrap: wrap;
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  .kind {
    font-weight: 600;
    color: var(--text);
  }
  .card-head .badge:last-child {
    margin-left: auto;
  }
  .kv {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 6px var(--sp-4);
    font-size: var(--fs-sm);
  }
  .kv dt {
    color: var(--text-3);
  }
  .kv dd {
    min-width: 0;
  }
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
  .card-foot {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    flex-wrap: wrap;
  }
  .hint {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  @media (max-width: 640px) {
    .kv.wide {
      grid-template-columns: minmax(0, 1fr);
      gap: 2px;
    }
    .kv.wide dd {
      margin-bottom: var(--sp-2);
    }
  }
</style>
