<script lang="ts">
  import { tick } from 'svelte';
  import { actTextLimit, jsonBytes } from '../../lib/compose';
  import type { CardProps } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { clock } from '../../lib/when';
  import Failure from '../Failure.svelte';
  import { changed } from '../changed';
  import Icon from '../Icon.svelte';

  /** A decision card with multiple-choice options and a free-text answer field (messages.md 2.4, 3.3). */
  let { group, msg }: CardProps<'decision'> = $props();
  const uid = $props.id();

  let cardEl: HTMLElement;
  let freeText = $state('');
  let busy = $state(false);
  let failed = $state('');

  const readonly = $derived(!group.home.ctl);
  const pending = $derived(msg.state.status === 'pending');
  const inTo = $derived(msg.body.to.includes(group.home.me.user));
  const canAct = $derived(pending && inTo);
  const trimmed = $derived(freeText.trim());
  const overLimit = $derived(trimmed !== '' && jsonBytes(trimmed) > actTextLimit);
  const ready = $derived(canAct && !readonly && !busy && !overLimit);
  const canSendText = $derived(ready && trimmed !== '');

  const taskRow = $derived(group.task(msg.body.task));

  const memberName = (id: string) => group.nameOf(id);
  const waitingOn = $derived(
    msg.body.to.map((uid) => group.personName(uid) ?? uid).join(t('decision.sep')),
  );

  const allowed = (arg?: string) => ready && (arg !== undefined || trimmed !== '');

  function requestBody(arg?: string) {
    return {
      t: 'act' as const,
      g: group.g,
      msg: msg.id,
      action: 'choose' as const,
      ...(arg !== undefined ? { arg } : {}),
      ...(trimmed !== '' ? { text: trimmed } : {}),
    };
  }

  async function settleFocus() {
    await tick();
    const next = cardEl.querySelector<HTMLElement>('button:not([disabled]), input:not([disabled])');
    if (next) next.focus({ preventScroll: true });
  }

  async function choose(arg?: string) {
    if (!allowed(arg)) return;
    const hadFocus = cardEl.contains(document.activeElement);
    busy = true;
    failed = '';
    if (hadFocus) {
      cardEl.tabIndex = -1;
      cardEl.focus({ preventScroll: true });
    }
    const answer = await group.home.command(requestBody(arg));
    busy = false;
    if (answer.err) failed = answer.err.code;
    else freeText = '';
    if (hadFocus) await settleFocus();
  }
</script>

<section
  bind:this={cardEl}
  class="card"
  class:need={canAct}
  class:settled={!pending}
  use:changed={msg.state.status}
  aria-label={t('decision.kind')}
>
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
            <b><span class="key">{opt.id}</span> · {opt.label}</b>
            <span>{opt.effect}</span>
            {#if opt.id === msg.body.recommended}
              <span class="badge">{t('decision.recommended')}</span>
            {/if}
          </button>
        {:else}
          {@const chosen = msg.state.status === 'decided' && msg.state.choice === opt.id}
          <div class="opt static" class:chosen>
            <b><span class="key">{opt.id}</span> · {opt.label}</b>
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
          aria-invalid={overLimit || undefined}
          aria-describedby={`${uid}-hint`}
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
        <p class="hint over" id={`${uid}-hint`} role="status">{t('decision.too_long')}</p>
      {:else}
        <p class="hint" id={`${uid}-hint`}>
          {msg.body.options.length ? t('decision.hint') : t('decision.hint.free')}
        </p>
      {/if}
    {:else}
      <p class="hint">{t('decision.waiting_hint').replace('{name}', waitingOn)}</p>
    {/if}
  {/if}
</section>

<style>
  /* The free answer and its button share one row while the field keeps room to be read. */
  .free-answer {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-2);

    input {
      flex-basis: 220px;
    }
  }
  .context {
    color: var(--text-2);
    font-size: var(--fs-ui);
  }
  /* The options are radio cards side by side; they stack where two no longer fit. */
  .opts {
    display: grid;
    gap: var(--sp-2);
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 200px), 1fr));
  }
  .opt {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-height: 56px;
    padding: 10px 14px;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--surface);
    text-align: left;
    font-size: var(--fs-sm);
    line-height: 1.5;
    color: var(--text-2);

    b {
      color: var(--text);
      font-size: var(--fs-ui);
      font-weight: 500;
    }
    .key {
      font-family: var(--font-mono);
      font-size: var(--fs-sm);
    }
    .badge {
      align-self: flex-start;
      margin-top: 4px;
    }
    /* Decided: the options not taken step back. */
    &.static:not(.chosen) {
      color: var(--text-3);

      b {
        color: var(--text-2);
      }
    }
    /* The one taken: the accent in its border, and a tick. */
    &.chosen {
      border-color: var(--sky);
      background: var(--sky-soft);
      box-shadow: inset 0 0 0 0.5px var(--sky);

      .badge {
        padding: 0;
        background: none;
        color: var(--sky);
      }
    }
  }
  button.opt {
    position: relative;
    isolation: isolate;

    &::before {
      content: '';
      position: absolute;
      inset: 0;
      z-index: -1;
      border-radius: 11px;
      background: var(--surface-2);
      opacity: 0;
      pointer-events: none;
      transition: opacity var(--dur-1) var(--ease);
    }
    &:not([disabled]):active::before {
      opacity: 1;
    }
  }
  @media (hover: hover) {
    button.opt:not([disabled]):hover::before {
      opacity: 1;
    }
  }
  button.opt[disabled] {
    opacity: 0.55;
    cursor: not-allowed;
  }
  .answer {
    padding: 10px 12px;
    border-radius: var(--r-md);
    background: var(--surface-2);
    font-size: var(--fs-ui);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .over {
    color: var(--danger);
  }
</style>
