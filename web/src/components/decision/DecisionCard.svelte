<script lang="ts">
  import { actTextLimit, jsonBytes } from '../../lib/compose';
  import type { CardProps } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { clock } from '../../lib/when';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** A decision card with multiple-choice options and a free-text answer field (messages.md 2.4, 3.3). */
  let { group, msg }: CardProps<'decision'> = $props();

  let freeText = $state('');
  let busy = $state(false);
  let failed = $state('');

  const readonly = $derived(!group.home.ctl);
  const pending = $derived(msg.state.status === 'pending');
  const inTo = $derived(msg.body.to.includes(group.home.me.user));
  const canAct = $derived(pending && inTo);
  const trimmed = $derived(freeText.trim());
  const overLimit = $derived(trimmed !== '' && jsonBytes(trimmed) > actTextLimit);
  const canSendText = $derived(canAct && !readonly && !busy && trimmed !== '' && !overLimit);

  const taskRow = $derived.by(() => {
    if (!msg.body.task) return undefined;
    const row = group.rows.get(`task ${msg.body.task}`);
    return row && row.r === 'task' && !('gone' in row) ? row : undefined;
  });

  const memberName = (id: string) => group.member(id)?.name ?? t('chat.gone');
  const userName = (uid: string) =>
    group.members.find((m) => m.kind === 'human' && m.user === uid)?.name ?? uid;
  const waitingOn = $derived(msg.body.to.map(userName).join(t('decision.sep')));

  async function choose(arg?: string) {
    if (!canAct || readonly || busy || overLimit) return;
    if (arg === undefined && trimmed === '') return;
    busy = true;
    failed = '';
    const answer = await group.home.command({
      t: 'act',
      g: group.g,
      msg: msg.id,
      action: 'choose',
      ...(arg !== undefined ? { arg } : {}),
      ...(trimmed !== '' ? { text: trimmed } : {}),
    });
    busy = false;
    if (answer.err) {
      failed = answer.err.code;
      return;
    }
    freeText = '';
  }
</script>

<section class="card" class:need={canAct} aria-label={t('decision.kind')}>
  <header class="card-head">
    <Icon name="fork" />
    <span class="kind">{t('decision.kind')}</span>
    {#if taskRow}<span class="mono">T-{taskRow.num}</span>{/if}
    {#if msg.state.status === 'pending'}
      {#if inTo}
        <span class="badge solid" data-tone="dawn">{t('decision.needs_you')}</span>
      {:else}
        <span class="badge">{t('decision.waiting_on').replace('{name}', waitingOn)}</span>
      {/if}
    {:else if msg.state.status === 'decided'}
      <span class="badge" data-tone="ok">
        <Icon name="check" sm />
        {t('decision.decided')
          .replace('{name}', memberName(msg.state.by))
          .replace('{time}', clock(msg.state.at, prefs.lang))}
      </span>
    {:else}
      <span class="badge">
        {t('decision.expired').replace('{time}', clock(msg.state.at, prefs.lang))}
      </span>
    {/if}
  </header>

  <h3>{msg.body.question}</h3>
  {#if msg.body.context}<p class="context">{msg.body.context}</p>{/if}

  {#if msg.body.options.length}
    <div class="opts">
      {#each msg.body.options as opt (opt.id)}
        {#if pending}
          <button
            type="button"
            class="opt"
            disabled={!inTo || readonly || busy || overLimit}
            onclick={() => void choose(opt.id)}
          >
            <b>{opt.id} · {opt.label}</b>
            <span>{opt.effect}</span>
            {#if opt.id === msg.body.recommended}
              <span class="badge">{t('decision.recommended')}</span>
            {/if}
          </button>
        {:else}
          {@const chosen = msg.state.status === 'decided' && msg.state.choice === opt.id}
          <div class="opt static" class:chosen>
            <b>{opt.id} · {opt.label}</b>
            <span>{opt.effect}</span>
            {#if chosen}
              <span class="badge" data-tone="ok">
                <Icon name="check" sm />{t('decision.chosen')}
              </span>
            {:else if opt.id === msg.body.recommended}
              <span class="badge">{t('decision.recommended')}</span>
            {/if}
          </div>
        {/if}
      {/each}
    </div>
  {/if}

  {#if msg.state.status === 'decided' && msg.state.text !== undefined}
    <p class="answer">{msg.state.text}</p>
  {:else if pending}
    {#if failed}<Failure code={failed} />{/if}
    {#if inTo}
      <form
        class="free-answer"
        onsubmit={(event) => {
          event.preventDefault();
          void choose();
        }}
      >
        <input
          class="input grow"
          type="text"
          aria-label={msg.body.options.length
            ? t('decision.custom.label')
            : t('decision.answer.label')}
          placeholder={msg.body.options.length
            ? t('decision.custom.hint')
            : t('decision.answer.hint')}
          bind:value={freeText}
          disabled={readonly || busy}
          onkeydown={(event) => {
            if (event.isComposing && event.key === 'Enter') event.preventDefault();
          }}
        />
        <button class="btn" class:btn-dawn={msg.body.options.length === 0} disabled={!canSendText}>
          {t('decision.submit')}
        </button>
      </form>
      {#if overLimit}
        <p class="hint over" role="status">{t('decision.too_long')}</p>
      {:else}
        <p class="hint">
          {msg.body.options.length ? t('decision.hint') : t('decision.hint.free')}
        </p>
      {/if}
    {:else}
      <p class="hint">{t('decision.waiting_hint').replace('{name}', waitingOn)}</p>
    {/if}
  {/if}
</section>

<style>
  .free-answer {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    flex-wrap: wrap;
  }
  .context {
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  .opts {
    display: grid;
    gap: var(--sp-2);
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr));
  }
  .opt {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 10px 12px;
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    background: var(--surface);
    text-align: left;
    font-size: var(--fs-sm);
    color: var(--text-2);
    b {
      color: var(--text);
      font-size: 14px;
    }
    .badge {
      align-self: flex-start;
      margin-top: 4px;
    }
    &.static {
      background: var(--surface-2);
    }
    &.chosen {
      border-color: color-mix(in srgb, var(--ok) 55%, var(--line));
      background: var(--ok-soft);
    }
  }
  button.opt {
    position: relative;
    isolation: isolate;

    &::before {
      content: '';
      position: absolute;
      inset: -1px;
      z-index: -1;
      border: 1px solid var(--dawn);
      border-radius: inherit;
      background: var(--dawn-soft);
      opacity: 0;
      pointer-events: none;
      transition: opacity var(--dur-1) var(--ease);
    }
  }
  @media (hover: hover) {
    button.opt:not([disabled]):hover::before {
      opacity: 1;
    }
  }
  button.opt[disabled] {
    background: var(--surface-2);
    color: var(--text-3);
    border-style: dashed;
    cursor: not-allowed;
  }
  .answer {
    padding: 8px 10px;
    border-radius: var(--r-md);
    background: var(--surface-2);
    font-size: var(--fs-sm);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .over {
    color: var(--danger);
  }
</style>
