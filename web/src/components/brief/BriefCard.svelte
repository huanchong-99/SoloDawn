<script lang="ts">
  import { tick } from 'svelte';
  import { actTextLimit, jsonBytes } from '../../lib/compose';
  import type { CardProps } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { clock } from '../../lib/when';
  import Avatar from '../Avatar.svelte';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';
  import BriefPreset from '../preset/BriefPreset.svelte';

  /** A secretary's task brief in all four states (contracts/messages.md 2.1, 3.1). */
  let { group, msg }: CardProps<'brief'> = $props();
  const uid = $props.id();

  let cardEl: HTMLElement;
  let focusNote = false;
  let focusChange = false;
  let revising = $state(false);
  let note = $state('');
  let busy = $state(false);
  let failed = $state('');

  const readonly = $derived(!group.home.ctl);
  const pending = $derived(msg.state.status === 'pending');
  const isRequester = $derived(group.me !== undefined && msg.body.requester === group.me.id);
  const isAdmin = $derived(
    group.me?.kind === 'human' && (group.me.role === 'owner' || group.me.role === 'admin'),
  );
  const canAct = $derived(pending && (isRequester || isAdmin));
  const trimmed = $derived(note.trim());
  const overLimit = $derived(trimmed !== '' && jsonBytes(trimmed) > actTextLimit);
  const canSendRevise = $derived(canAct && !readonly && !busy && trimmed !== '' && !overLimit);
  const revisionNote = $derived('note' in msg.state ? msg.state.note : undefined);

  const nameOf = (id: string) => group.nameOf(id);
  const passText = $derived.by(() => {
    const hasFoundation = msg.body.tasks.some((item) => item.foundation);
    const foundationPass = group.info?.pass_foundation;
    if (hasFoundation && foundationPass !== undefined) {
      return t('brief.pass.foundation')
        .replace('{n}', String(msg.body.pass))
        .replace('{f}', String(foundationPass));
    }
    return t('brief.pass').replace('{n}', String(msg.body.pass));
  });

  const takeNoteFocus = (el: HTMLElement) => {
    if (!focusNote) return;
    focusNote = false;
    el.focus({ preventScroll: true });
  };

  const takeChangeFocus = (el: HTMLElement) => {
    if (!focusChange) return;
    focusChange = false;
    el.focus({ preventScroll: true });
  };

  function openRevise() {
    focusNote = true;
    revising = true;
    failed = '';
  }

  function closeRevise() {
    focusChange = true;
    revising = false;
    failed = '';
  }

  async function settleFocus() {
    await tick();
    const next =
      cardEl.querySelector<HTMLElement>(
        'summary, textarea:not([disabled]), button:not([disabled])',
      ) ?? cardEl;
    if (next === cardEl) cardEl.tabIndex = -1;
    next.focus({ preventScroll: true });
  }

  async function act(fields: { action: 'confirm' } | { action: 'revise'; text: string }) {
    const hadFocus = cardEl.contains(document.activeElement);
    busy = true;
    failed = '';
    if (hadFocus) {
      cardEl.tabIndex = -1;
      cardEl.focus({ preventScroll: true });
    }
    const answer = await group.home.command({ t: 'act', g: group.g, msg: msg.id, ...fields });
    busy = false;
    if (answer.err) {
      failed = answer.err.code;
    } else {
      revising = false;
      note = '';
    }
    if (hadFocus) await settleFocus();
  }
</script>

{#snippet details(folded: boolean)}
  <dl class="kv wide" class:fold-body={folded}>
    <dt>{t('brief.goal')}</dt>
    <dd>{msg.body.goal}</dd>
    <dt>{t('brief.criteria')}</dt>
    <dd>
      <ol class="num">
        {#each msg.body.criteria as item, i (i)}<li>{item}</li>{/each}
      </ol>
    </dd>
    <dt>{t('brief.scope')}</dt>
    <dd class="scope">
      {#if msg.body.scope.in.length}
        <p>
          <span class="pill-do">{t('brief.scope.in')}</span>{msg.body.scope.in.join(t('brief.sep'))}
        </p>
      {/if}
      {#if msg.body.scope.out.length}
        <p>
          <span class="pill-dont">{t('brief.scope.out')}</span>{msg.body.scope.out.join(
            t('brief.sep'),
          )}
        </p>
      {/if}
    </dd>
    <dt>{t('brief.split')}</dt>
    <dd>
      <ul class="split">
        {#each msg.body.tasks as item, i (i)}
          {@const assignee = group.member(item.assignee)}
          <li>
            <span class="mono">#{i + 1}</span>
            <span class="grow">
              {item.title}
              {#if item.foundation}<span class="badge">{t('brief.foundation')}</span>{/if}
              {#if item.depends.length}
                <span class="muted">
                  ({t('brief.after').replace(
                    '{tasks}',
                    item.depends.map((d) => `#${d + 1}`).join(', '),
                  )})
                </span>
              {/if}
            </span>
            <span class="who muted">
              <Avatar
                id={item.assignee}
                name={assignee?.name ?? '?'}
                cli={assignee?.kind === 'ai' ? assignee.cli : undefined}
                size="sm"
              />
              {nameOf(item.assignee)} · {t('brief.review_by').replace(
                '{name}',
                nameOf(item.reviewer),
              )}
            </span>
          </li>
        {/each}
        <li>
          <span class="mono muted">{t('brief.review')}</span>
          <span class="grow muted">{passText}</span>
        </li>
      </ul>
    </dd>
    {#if msg.body.preset}
      <dt>{t('preset.label')}</dt>
      <dd><BriefPreset {group} name={msg.body.preset} /></dd>
    {/if}
  </dl>
{/snippet}

<section
  bind:this={cardEl}
  class="card"
  class:need={canAct}
  aria-label={`${t('brief.kind')} · ${msg.body.title}`}
>
  <header class="card-head">
    <Icon name="doc" />
    <span class="kind">{t('brief.kind')}</span>
    {#if msg.state.status === 'pending'}
      {#if isRequester}
        <span class="badge solid" data-tone="dawn">{t('brief.needs_you')}</span>
      {:else}
        <span class="badge">
          {t('brief.waiting_on').replace('{name}', nameOf(msg.body.requester))}
        </span>
      {/if}
    {:else if msg.state.status === 'confirmed'}
      <span class="badge" data-tone="ok">
        <Icon name="check" sm />
        {t('brief.confirmed')
          .replace('{name}', nameOf(msg.state.by))
          .replace('{time}', clock(msg.state.at, prefs.lang))}
      </span>
    {:else if msg.state.status === 'revise'}
      <span class="badge">
        {t('brief.revised')
          .replace('{name}', nameOf(msg.state.by))
          .replace('{time}', clock(msg.state.at, prefs.lang))}
      </span>
    {:else}
      <span class="badge">{t('brief.superseded')}</span>
    {/if}
  </header>

  <h3>{msg.body.title}</h3>

  {#if pending}
    {@render details(false)}
    {#if failed}<Failure code={failed} />{/if}
    {#if revising && canAct}
      <form
        class="revise"
        onsubmit={(event) => {
          event.preventDefault();
          if (canSendRevise) void act({ action: 'revise', text: trimmed });
        }}
      >
        <textarea
          use:takeNoteFocus
          class="input"
          rows="2"
          aria-label={t('brief.note.label')}
          aria-invalid={overLimit || undefined}
          aria-describedby={overLimit ? `${uid}-over` : undefined}
          placeholder={t('brief.note.hint')}
          bind:value={note}
          disabled={readonly || busy}
          onkeydown={(event) => {
            if (event.key === 'Escape' && !busy) {
              event.preventDefault();
              closeRevise();
            }
          }}></textarea>
        <div class="card-foot">
          <button class="btn btn-dawn" disabled={!canSendRevise}>{t('brief.send_changes')}</button>
          <button type="button" class="btn" disabled={busy} onclick={closeRevise}>
            {t('brief.cancel')}
          </button>
          {#if overLimit}
            <span class="hint over" id={`${uid}-over`} role="status">{t('brief.too_long')}</span>
          {/if}
        </div>
      </form>
    {:else}
      <footer class="card-foot">
        <button
          type="button"
          class="btn btn-dawn"
          disabled={!canAct || readonly || busy}
          onclick={() => void act({ action: 'confirm' })}
        >
          <Icon name="check" sm />{t('brief.confirm')}
        </button>
        <button
          use:takeChangeFocus
          type="button"
          class="btn"
          disabled={!canAct || readonly || busy}
          onclick={openRevise}
        >
          {t('brief.change')}
        </button>
        <span class="hint">
          {canAct
            ? t('brief.hint')
            : t('brief.waiting_hint').replace('{name}', nameOf(msg.body.requester))}
        </span>
      </footer>
    {/if}
  {:else}
    {#if revisionNote}<p class="revise-note">{revisionNote}</p>{/if}
    <details class="fold">
      <summary><Icon name="down" sm /><b>{t('brief.details')}</b></summary>
      {@render details(true)}
    </details>
  {/if}
</section>

<style>
  .scope p + p {
    margin-top: 4px;
  }
  .num {
    list-style: decimal;
    padding-left: 1.4em;
    li + li {
      margin-top: 2px;
    }
  }
  .pill-do,
  .pill-dont {
    display: inline-block;
    margin-right: 6px;
    padding: 0 6px;
    border-radius: var(--r-sm);
    font-size: var(--fs-xs);
    font-weight: 600;
  }
  .pill-do {
    background: var(--ok-soft);
    color: var(--ok);
  }
  .pill-dont {
    background: var(--surface-2);
    color: var(--text-2);
  }
  .split li {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    flex-wrap: wrap;
    padding: 6px 0;
    & + li {
      border-top: 1px dashed var(--line);
    }
  }
  .who {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }
  .revise {
    display: grid;
    gap: var(--sp-2);
  }
  .revise-note {
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
  @media (max-width: 640px) {
    .split .grow {
      flex-basis: 100%;
    }
  }
</style>
