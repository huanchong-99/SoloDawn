<script lang="ts">
  import { parts } from '../../lib/compose';
  import {
    authorOf,
    forgetDisabledReason,
    groupNotes,
    groupReviews,
    MemoryView,
    previewText,
    type ForgetRowType,
  } from '../../lib/groupinfo.svelte';
  import type { Group } from '../../lib/groups.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { now, shortWhen } from '../../lib/when';
  import Icon from '../Icon.svelte';
  import ForgetDialog, { type ForgetTarget } from './ForgetDialog.svelte';

  /** Group info and project-memory section mounted in `chat.side` below the member list. */
  let { group }: { group: Group } = $props();
  const uid = $props.id();

  let open = $state(false);
  let notesHead = $state<HTMLElement>();
  let ledgerHead = $state<HTMLElement>();
  let pending = $state.raw<ForgetTarget>();

  const memory = $derived(new MemoryView(group));
  const notes = $derived(groupNotes(group));
  const reviews = $derived(groupReviews(group));
  const forgetReason = $derived(forgetDisabledReason(group));
  const noteStuck = $derived(memory.note.failed || (memory.note.more && !group.home.ctl));
  const reviewStuck = $derived(memory.review.failed || (memory.review.more && !group.home.ctl));

  $effect(() => {
    if (open) memory.sync();
  });

  function askForget(r: ForgetRowType, id: string, label: string) {
    pending = { r, id, label, heading: r === 'note' ? notesHead : ledgerHead };
  }
</script>

<section class="groupinfo" aria-labelledby={`${uid}-head`}>
  <h2 id={`${uid}-head`} class="sec-title">{t('groupinfo.title')}</h2>

  <details class="fold" bind:open>
    <summary>
      <Icon name="down" sm />
      <b>{t('groupinfo.memory')}</b>
      <span class="cut grow muted">{t('groupinfo.memory.lead')}</span>
    </summary>

    {#if open}
      <div class="fold-body mem-body">
        {#if forgetReason && (notes.length > 0 || reviews.length > 0)}
          <p id={`${uid}-why`} class="hint">{forgetReason}</p>
        {/if}

        <div class="mem-sec">
          <h3 bind:this={notesHead} id={`${uid}-notes`} class="sec-title" tabindex="-1">
            {t('groupinfo.notes')}
          </h3>
          {#if noteStuck}
            <p class="hint">{t('groupinfo.older.offline')}</p>
          {:else if memory.note.more}
            {#if notes.length > 0}
              <button
                type="button"
                class="btn btn-sm btn-ghost older"
                disabled={memory.note.busy}
                onclick={() => void memory.older('note')}
              >
                {t(memory.note.busy ? 'groupinfo.older.busy' : 'groupinfo.older')}
              </button>
            {:else if memory.note.busy}
              <p class="hint" role="status">{t('groupinfo.loading')}</p>
            {/if}
          {/if}
          {#if notes.length > 0}
            <ul class="mem-list" aria-labelledby={`${uid}-notes`}>
              {#each notes as note (note.id)}
                {@const task = note.task ? group.task(note.task) : undefined}
                {@const label = previewText(note.text)}
                <li class="mem-item">
                  <div class="mem-meta">
                    <b>{authorOf(group, note.by)}</b>
                    {#if task}<span class="mono muted">T-{task.num}</span>{/if}
                    <time datetime={new Date(note.ts).toISOString()}>
                      {shortWhen(note.ts, now(), prefs.lang)}
                    </time>
                    <button
                      type="button"
                      class="btn btn-sm btn-ghost del"
                      disabled={!group.info || Boolean(forgetReason)}
                      aria-label={t('groupinfo.forget.note').replace('{text}', label)}
                      aria-describedby={forgetReason ? `${uid}-why` : undefined}
                      onclick={() => askForget('note', note.id, label)}
                    >
                      {t('groupinfo.forget')}
                    </button>
                  </div>
                  <p class="mem-text">
                    {#each parts(note.text, []) as part, i (i)}
                      {#if part.kind === 'code'}<code>{part.text}</code>{:else}{part.text}{/if}
                    {/each}
                  </p>
                </li>
              {/each}
            </ul>
          {:else if !memory.note.more && !noteStuck}
            <p class="hint">{t('groupinfo.notes.empty')}</p>
          {/if}
        </div>

        <div class="mem-sec">
          <h3 bind:this={ledgerHead} id={`${uid}-ledger`} class="sec-title" tabindex="-1">
            {t('groupinfo.ledger')}
          </h3>
          {#if reviewStuck}
            <p class="hint">{t('groupinfo.older.offline')}</p>
          {:else if memory.review.more}
            {#if reviews.length > 0}
              <button
                type="button"
                class="btn btn-sm btn-ghost older"
                disabled={memory.review.busy}
                onclick={() => void memory.older('review')}
              >
                {t(memory.review.busy ? 'groupinfo.older.busy' : 'groupinfo.older')}
              </button>
            {:else if memory.review.busy}
              <p class="hint" role="status">{t('groupinfo.loading')}</p>
            {/if}
          {/if}
          {#if reviews.length > 0}
            <ul class="mem-list" aria-labelledby={`${uid}-ledger`}>
              {#each reviews as review (review.id)}
                {@const body = review.body}
                {@const label = previewText(`T-${body.num} ${body.title}`)}
                <li class="mem-item">
                  <div class="mem-meta">
                    <span class="mono">T-{body.num}</span>
                    <span class="badge" data-tone={body.ok ? 'ok' : 'danger'}>
                      {body.score} / {body.pass}
                    </span>
                    <time datetime={new Date(review.ts).toISOString()}>
                      {shortWhen(review.ts, now(), prefs.lang)}
                    </time>
                    <button
                      type="button"
                      class="btn btn-sm btn-ghost del"
                      disabled={!group.info || Boolean(forgetReason)}
                      aria-label={t('groupinfo.forget.review').replace('{text}', label)}
                      aria-describedby={forgetReason ? `${uid}-why` : undefined}
                      onclick={() => askForget('msg', review.id, label)}
                    >
                      {t('groupinfo.forget')}
                    </button>
                  </div>
                  <p class="mem-sub">
                    <b>{body.title}</b>
                    <span class="muted">
                      · {authorOf(group, review.by)} · {t('groupinfo.round').replace(
                        '{n}',
                        String(body.round),
                      )}
                    </span>
                  </p>
                  {#if body.items.length > 0}
                    <ul class="bul">
                      {#each body.items as item, i (i)}
                        <li>
                          {#if item.veto}
                            <span class="badge solid veto" data-tone="danger">
                              {t('groupinfo.veto')}
                            </span>
                          {/if}
                          {#each parts(item.text, []) as part, j (j)}
                            {#if part.kind === 'code'}<code>
                                {part.text}
                              </code>{:else}{part.text}{/if}
                          {/each}
                          {#if item.file}
                            <code class="loc">
                              {item.line !== undefined ? `${item.file}:${item.line}` : item.file}
                            </code>
                          {/if}
                        </li>
                      {/each}
                    </ul>
                  {:else}
                    <p class="muted"><code>{body.commit.slice(0, 7)}</code></p>
                  {/if}
                </li>
              {/each}
            </ul>
          {:else if !memory.review.more && !reviewStuck}
            <p class="hint">{t('groupinfo.ledger.empty')}</p>
          {/if}
        </div>
      </div>
    {/if}
  </details>
</section>

{#if pending}
  <ForgetDialog {group} target={pending} onclose={() => (pending = undefined)} />
{/if}

<style>
  .groupinfo {
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
    padding: var(--sp-4) var(--sp-4) 14px;
    border-top: 1px solid var(--line);
  }
  .groupinfo > h2 + .fold {
    border-top: 0;
    margin-top: -4px;
  }
  .groupinfo > .fold > summary {
    border-radius: 0;
  }
  code {
    padding: 1px 5px;
    border-radius: var(--r-sm);
    background: var(--surface-3);
    overflow-wrap: anywhere;
  }
  .mem-body {
    display: flex;
    flex-direction: column;
    gap: var(--sp-3);
    padding-top: var(--sp-3);
  }
  .mem-sec {
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
  }
  h3.sec-title:focus {
    outline: none;
  }
  h3.sec-title:focus-visible {
    outline: var(--focus);
    outline-offset: 2px;
    border-radius: var(--r-sm);
  }
  .older {
    align-self: flex-start;
  }
  .mem-list {
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
  }
  .mem-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 10px var(--sp-3);
    border-radius: var(--r-md);
    background: var(--surface-2);
    font-size: var(--fs-sm);
    line-height: 1.5;
    min-width: 0;
  }
  .mem-meta {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
    font-size: var(--fs-xs);
    color: var(--text-3);
  }
  .mem-meta b {
    color: var(--text);
    font-size: var(--fs-sm);
  }
  .del {
    margin-left: auto;
  }
  .del:not([disabled]) {
    color: var(--danger);
    --hover: var(--danger-soft);
  }
  .mem-text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .mem-sub {
    overflow-wrap: anywhere;
  }
  .bul {
    list-style: disc;
    padding-left: 1.2em;
  }
  .bul li + li {
    margin-top: 2px;
  }
  .bul code {
    background: var(--surface-3);
  }
  .loc {
    margin-left: 4px;
  }
  .veto {
    height: 18px;
    padding: 0 6px;
    margin-right: 4px;
    font-size: 11px;
  }
</style>
