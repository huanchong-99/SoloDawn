<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { withoutInlineReplies } from '../../lib/call';
  import { subject, type Group } from '../../lib/groups.svelte';
  import { jumpToMessage } from '../../lib/jump';
  import { card as cardOf } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import type { MsgRow, TaskRow } from '../../lib/proto';
  import { clock, now } from '../../lib/when';
  import Avatar from '../Avatar.svelte';
  import Composer from '../chat/Composer.svelte';
  import Waits from '../queue/Waits.svelte';
  import Rich from '../Rich.svelte';

  /** The messages of one task's thread and the field to write in it, inside a task card's fold. */
  let { group, task }: { group: Group; task: TaskRow } = $props();

  const msgs = $derived(withoutInlineReplies(group.thread(task.id), Boolean(cardOf('call'))));
  const paging = $derived(group.paging(task.id));
  const stuck = $derived(paging.failed || (paging.more && !group.home.ctl));

  let loaded = $state.raw<object>();
  $effect(() => {
    if (paging.ready && group.home.ctl && loaded !== paging) {
      loaded = paging;
      untrack(() => void group.older(task.id));
    }
  });

  const assignee = $derived(group.member(task.assignee));
  const working = $derived(
    assignee?.kind === 'ai' && assignee.state === 'working' && assignee.task === task.id
      ? assignee
      : undefined,
  );
  const workMins = $derived(working ? Math.floor((now() - working.since) / 60_000) : 0);

  type Badge = { label: string; tone?: 'ok' | 'danger' | 'dawn' | 'sky' };
  const tones: Partial<Record<MsgRow['kind'], Badge['tone']>> = {
    call: 'sky',
    report: 'ok',
    summary: 'ok',
  };

  function badgeOf(msg: MsgRow): Badge | null {
    if (msg.kind === 'text') return null;
    if (msg.kind === 'review') {
      const key = msg.body.ok ? 'thread.review.badge.ok' : 'thread.review.badge.fail';
      return {
        label: t(key).replace('{n}', String(msg.body.round)),
        tone: msg.body.ok ? 'ok' : 'danger',
      };
    }
    return {
      label: t(`chat.kind.${msg.kind}`),
      tone: group.waiting.includes(msg) ? 'dawn' : tones[msg.kind],
    };
  }

  function lineText(msg: MsgRow): string {
    if (msg.kind === 'summary' && !msg.text) {
      const score = t('thread.summary.score').replace('{n}', String(msg.body.score));
      const num = group.task(msg.body.task)?.num;
      return num !== undefined ? `T-${num} · ${score}` : score;
    }
    return subject(group, msg) || t(`chat.kind.${msg.kind}`);
  }
  let root: HTMLElement;
  let listEl: HTMLElement;

  function moveThreadFocus() {
    const summary = root.closest('details')?.querySelector<HTMLElement>('summary');
    const next =
      root.querySelector<HTMLElement>('.tpage button:not([disabled])') ?? summary ?? listEl;
    if (next === listEl) listEl.tabIndex = -1;
    next.focus({ preventScroll: true });
  }

  async function loadOlder() {
    const hadFocus = root.contains(document.activeElement);
    if (hadFocus) {
      listEl.tabIndex = -1;
      listEl.focus({ preventScroll: true });
    }
    await group.older(task.id);
    await tick();
    if (hadFocus) moveThreadFocus();
  }
</script>

<div class="thread" bind:this={root}>
  {#if stuck}
    <p class="tpage">{t('thread.older.offline')}</p>
  {:else if paging.more}
    <div class="tpage" role={paging.busy ? 'status' : undefined}>
      <button
        type="button"
        class="btn btn-sm btn-ghost"
        disabled={paging.busy}
        onclick={() => void loadOlder()}
      >
        {t(paging.busy ? 'thread.older.busy' : 'thread.older')}
      </button>
    </div>
  {/if}

  <ol class="tlist" bind:this={listEl}>
    {#each msgs as msg (msg.id)}
      {@const time = clock(msg.ts, prefs.lang)}
      {@const iso = new Date(msg.ts).toISOString()}
      {#if msg.by === 'system'}
        {@const Card = cardOf(msg.kind)}
        <li class="ti sys">
          {#if Card}
            <Card {group} {msg} />
          {:else}
            <p>{t(`chat.kind.${msg.kind}`)} · <time datetime={iso}>{time}</time></p>
          {/if}
        </li>
      {:else}
        {@const author = group.member(msg.by)}
        {@const badge = badgeOf(msg)}
        <li class="ti">
          <Avatar
            id={author?.kind === 'human' ? author.user : msg.by}
            name={group.nameOf(msg.by)}
            cli={author?.kind === 'ai' ? author.cli : undefined}
            size="sm"
          />
          <div>
            <p class="ti-head">
              <b>{group.nameOf(msg.by)}</b>
              {#if badge}<span class="badge" data-tone={badge.tone}>{badge.label}</span>{/if}
              <time datetime={iso}>{time}</time>
            </p>
            {#if msg.main}
              <p class="text">
                <a
                  class="tcard"
                  href="#m-{msg.id}"
                  onclick={(event) => jumpToMessage(event, msg.id, msg.kind)}
                >
                  {lineText(msg)}
                </a>
              </p>
            {:else}
              {@const Card = cardOf(msg.kind)}
              {#if msg.text && !(msg.kind === 'call' && Card)}
                <p class="text"><Rich {group} text={msg.text} mentions={msg.mentions} /></p>
              {/if}
              {#if Card}
                <Card {group} {msg} />
              {/if}
              <Waits {group} {msg} />
            {/if}
          </div>
        </li>
      {/if}
    {:else}
      {#if !paging.more && !stuck && !working}
        <li class="ti sys empty"><p>{t('thread.empty')}</p></li>
      {/if}
    {/each}

    {#if working}
      <li class="ti sys working">
        <span class="typing" aria-hidden="true"><i></i><i></i><i></i></span>
        <p>
          {workMins >= 1
            ? t('thread.working.since')
                .replace('{name}', working.name)
                .replace('{n}', String(workMins))
            : t('thread.working').replace('{name}', working.name)}
        </p>
      </li>
    {/if}
  </ol>
  <Composer {group} {task} />
</div>

<style>
  /* A thread is a work log: compact flat rows, no bubbles and no boxes. */
  .thread {
    border-radius: 0 0 var(--r-lg) var(--r-lg);
    font-size: var(--fs-sm);
    line-height: 1.5;
  }
  .tpage {
    display: flex;
    justify-content: center;
    padding: var(--sp-1) var(--sp-4) 0;
    color: var(--text-3);
    font-size: var(--fs-xs);
  }
  .tlist {
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
    padding: var(--sp-1) var(--sp-4) var(--sp-4);
  }
  .ti {
    display: grid;
    grid-template-columns: 20px minmax(0, 1fr);
    gap: 2px var(--sp-2);
    color: var(--text-2);
  }
  .ti > :global(.av) {
    margin-top: 1px;
  }
  .ti > div {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .ti.sys {
    display: block;
    color: var(--text-3);
  }
  .ti.working {
    display: grid;
    align-items: center;
  }
  .ti.empty {
    padding-block: var(--sp-2);
    text-align: center;
  }
  .ti-head {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
    min-height: 20px;
  }
  .ti-head b {
    color: var(--text);
    font-weight: 500;
  }
  .ti-head time {
    color: var(--text-3);
    font-size: var(--fs-xs);
  }
  .text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  /* The AI is at work: its dots breathe, the one motion that loops. */
  .typing {
    display: inline-flex;
    justify-self: center;
    gap: 3px;
  }
  .typing i {
    width: 4px;
    height: 4px;
    border-radius: 50%;
    background: var(--sky);
    animation: pulse 1.6s ease-in-out infinite;
  }
  .typing i:nth-child(2) {
    animation-delay: 0.2s;
  }
  .typing i:nth-child(3) {
    animation-delay: 0.4s;
  }
</style>
