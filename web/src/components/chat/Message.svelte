<script lang="ts">
  import { untrack } from 'svelte';
  import { subject, type Group } from '../../lib/groups.svelte';
  import { card as cardOf } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import type { MsgRow } from '../../lib/proto';
  import { clock } from '../../lib/when';
  import Avatar from '../Avatar.svelte';
  import Rich from '../Rich.svelte';
  import Waits from '../queue/Waits.svelte';

  /** One message of the main line. `joined`: it follows a message of the same author closely, and
   *  is drawn under it without a head of its own. `above`: it stands above an unclosed gap, so
   *  scroll anchoring skips it. */
  let {
    group,
    msg,
    joined,
    above = false,
  }: { group: Group; msg: MsgRow; joined: boolean; above?: boolean } = $props();

  const author = $derived(group.member(msg.by));
  const time = $derived(clock(msg.ts, prefs.lang));
  const plain = $derived(msg.kind === 'text');
  // The component an area mounts for this kind of message (lib/mounts.svelte.ts) draws it under
  // its head and text. A card without one is one line that says what it is.
  const Card = $derived(cardOf(msg.kind));
  const card = $derived(msg.state !== null || msg.kind === 'summary');
  const needsMe = $derived(group.waiting.includes(msg));
  // The person's own messages stand on the right, without a picture.
  const mine = $derived(author !== undefined && author.id === group.me?.id);
  // Drawn within a moment of being written: it arrived while the chat was open, and comes in from
  // its side. What a chat opens with, and a page of older messages, is simply there.
  const live = untrack(() => Date.now() - msg.ts < 3000);
</script>

{#snippet body()}
  {#if msg.text && !(msg.kind === 'call' && Card)}
    <div class="said">
      <p class="text"><Rich {group} text={msg.text} mentions={msg.mentions} /></p>
      {#if joined}<time class="aside" datetime={new Date(msg.ts).toISOString()}>{time}</time>{/if}
    </div>
  {:else if joined}
    <time class="aside alone" datetime={new Date(msg.ts).toISOString()}>{time}</time>
  {/if}
  {#if Card}
    <Card {group} {msg} />
  {:else if card}
    <div class="card stub" class:need={needsMe}>
      <span class="kind">{t(`chat.kind.${msg.kind}`)}</span>
      <span class="grow cut">{subject(group, msg) === msg.text ? '' : subject(group, msg)}</span>
      {#if needsMe}
        <span class="badge solid" data-tone="dawn">{t('groups.waiting')}</span>
      {/if}
    </div>
  {/if}
{/snippet}

{#if msg.by === 'system' && Card}
  <!-- Written by the home, and drawn whole by the component mounted for its kind. -->
  <div class="msg sys" class:above id={`m-${msg.id}`}><Card {group} {msg} /></div>
{:else if msg.by === 'system'}
  <!-- Written by the home: a quiet line, with no author. -->
  <p class="msg note" class:above id={`m-${msg.id}`}>
    <span class="badge">{t(`chat.kind.${msg.kind}`)}</span>
    {#if msg.kind === 'system'}
      <code>{msg.body.code}</code>
    {:else if msg.kind === 'task'}
      {@const task = group.task(msg.body.task)}
      {#if task}
        <span class="cut"><span class="mono">T-{task.num}</span> {task.title}</span>
      {/if}
    {/if}
    <time datetime={new Date(msg.ts).toISOString()}>{time}</time>
  </p>
{:else}
  <article class="msg" class:joined class:mine class:live class:above id={`m-${msg.id}`}>
    {#if !joined}
      {#if !mine}
        <Avatar
          id={author?.kind === 'human' ? author.user : msg.by}
          name={group.nameOf(msg.by)}
          cli={author?.kind === 'ai' ? author.cli : undefined}
        />
      {/if}
      <p class="msg-head">
        <b class="cut">{group.nameOf(msg.by)}</b>
        {#if author?.kind === 'ai' && author.secretary}
          <span class="badge">{t('chat.secretary')}</span>
        {/if}
        {#if author && author.id === group.me?.id}
          <span class="badge you">{t('chat.you')}</span>
        {/if}
        {#if !plain && !card}<span class="badge">{t(`chat.kind.${msg.kind}`)}</span>{/if}
        <time datetime={new Date(msg.ts).toISOString()}>{time}</time>
      </p>
    {/if}
    <div class="msg-body">
      {@render body()}
      <Waits {group} {msg} />
    </div>
  </article>
{/if}

<style>
  /* The main line is a conversation: who said what is told by side and bubble before any name is
     read. Others stand on the left under their picture and name, the person's own messages on the
     right. A card is a document with actions, so it is not put in a bubble: it stands on its own
     surface in its author's column. */
  .msg {
    display: grid;
    grid-template-columns: var(--av, 36px) minmax(0, 1fr);
    column-gap: 10px;
    row-gap: 4px;
    margin-top: var(--sp-4);
    scroll-margin-top: var(--sp-12);
  }
  .above {
    overflow-anchor: none;
  }
  /* A message that arrives while the chat is open comes in from its own side. */
  .live {
    transform-origin: bottom left;
    animation: arrive-msg var(--dur-2) var(--ease-out);
  }
  .live.mine {
    transform-origin: bottom right;
  }
  @keyframes arrive-msg {
    from {
      opacity: 0;
      transform: translateY(var(--rise)) scale(0.98);
    }
  }
  .msg > :global(.av) {
    grid-row: 1 / span 2;
  }
  .msg-head {
    grid-column: 2;
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    min-height: 20px;
    color: var(--text-3);
    font-size: var(--fs-xs);
    line-height: 1.4;
  }
  .msg-head b {
    color: var(--text-2);
    font-size: var(--fs-sm);
    font-weight: 500;
  }
  time {
    flex: none;
    color: var(--text-3);
    font-size: var(--fs-xs);
  }
  .msg-body {
    grid-column: 2;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 6px;
    min-width: 0;
  }
  .msg-body > :global(*) {
    max-width: 100%;
  }
  .msg-body > :global(.card),
  .sys > :global(.card) {
    width: 100%;
    max-width: var(--card-w);
  }
  /* The bubble and, for a message joined to the one before it, its time on the outer side. */
  .said {
    display: flex;
    align-items: flex-end;
    gap: var(--sp-2);
    max-width: var(--bubble-w);
  }
  /* Written as it was typed: line breaks kept, long words broken. */
  .text {
    min-width: 0;
    padding: 9px 14px;
    border-radius: var(--r-xl);
    background: var(--surface-2);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .text :global(code) {
    background: var(--surface-3);
  }
  /* In a run of one author the corners toward the neighbour, on the author's side, tighten. */
  .joined .text {
    border-top-left-radius: var(--r-sm);
  }
  .msg:has(+ :global(.msg.joined)) .text {
    border-bottom-left-radius: var(--r-sm);
  }
  code {
    padding: 1px 5px;
    border-radius: var(--r-sm);
    background: var(--surface-2);
  }
  .joined {
    margin-top: var(--sp-1);
  }

  /* The person's own: no picture, the head one quiet line at the right, the bubble in the tint
     of the accent. The name stays for a screen reader. */
  .mine {
    grid-template-columns: minmax(0, 1fr);
  }
  .mine .msg-head,
  .mine .msg-body {
    grid-column: 1;
  }
  .mine .msg-head {
    justify-content: flex-end;
    gap: 0;
  }
  .mine .msg-head b {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
  }
  .mine .msg-head .you {
    gap: 0;
    height: auto;
    padding: 0;
    background: none;
    color: var(--text-3);
    font-weight: 400;
  }
  .mine .msg-head .you::after {
    content: '·';
    margin-inline: 4px;
  }
  .mine .msg-head .badge:not(.you) {
    margin-right: 6px;
    order: -1;
  }
  .mine .msg-body {
    align-items: flex-end;
  }
  .mine .said {
    flex-direction: row-reverse;
  }
  .mine .text {
    background: var(--mine);
  }
  /* On the tint of the accent a mention has no ground of its own: the accent on --mine is 4.5:1
     or more (features/web_a11y.feature), a darker ground under it would not be. */
  .mine .text :global(.at) {
    padding: 0;
    background: none;
    font-weight: 600;
  }
  .mine.joined .text {
    border-top-left-radius: var(--r-xl);
    border-top-right-radius: var(--r-sm);
  }
  .mine:has(+ :global(.msg.joined)) .text {
    border-bottom-left-radius: var(--r-xl);
    border-bottom-right-radius: var(--r-sm);
  }

  /* The time of a joined message shows beside its bubble while the pointer is on the message. A
     screen reader reads it at any time. */
  .aside {
    padding-bottom: 6px;
    font-size: 11px;
    opacity: 0;
    transition: opacity var(--dur-1) var(--ease);
  }
  /* A joined card has no bubble to stand beside: its time is for the screen reader alone. */
  .aside.alone {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
  }
  /* For someone at the keyboard (`keys` in Chat.svelte) the times of all joined messages show
     while focus is on the main line or on a button of the chat: no message is a stop of its own
     for the Tab key. Not while focus is in the field, where the keys are for writing. */
  .joined:hover .aside,
  :global(.chat.keys:has(:focus-visible:not(textarea))) .aside {
    opacity: 1;
  }
  /* Written by the home and drawn whole by its component: a card in the column of the cards. */
  .sys {
    display: block;
    padding-left: calc(var(--av, 36px) + 10px);
  }
  /* A notice of the home in the main line is a quiet centred line, as its notes are. */
  .sys:has(> :global(.sys-row)) {
    padding-left: 0;
  }
  .sys > :global(.sys-row) {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: center;
    gap: 2px 6px;
    margin-top: var(--sp-3);
    font-size: var(--fs-xs);
    text-align: center;
  }
  /* A quiet line of the home: centred, small, no chip. */
  .note {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    margin-top: var(--sp-3);
    color: var(--text-3);
    font-size: var(--fs-xs);
  }
  .note .badge {
    height: auto;
    padding: 0;
    background: none;
    color: inherit;
    font-weight: 400;
  }
  .note .badge::after {
    content: '·';
    margin-left: 6px;
  }
  .stub {
    flex-direction: row;
    align-items: center;
    gap: var(--sp-2);
    padding: 10px var(--sp-3);
    font-size: var(--fs-sm);
  }
  .stub .kind {
    flex: none;
    font-weight: 500;
  }
  @media (max-width: 640px) {
    .msg {
      --av: 32px;
      column-gap: var(--sp-2);
    }
    /* A card spans the column, under the head. */
    .msg:has(:global(.card)) > :global(.av) {
      grid-row: 1;
    }
    .msg:has(:global(.card)) .msg-body {
      grid-column: 1 / -1;
    }
    .msg-body > :global(.card),
    .sys > :global(.card) {
      max-width: none;
    }
    .sys {
      padding-left: 0;
    }
    .said {
      max-width: 82%;
    }
    .msg:not(.mine) .said {
      max-width: 88%;
    }
    .aside {
      display: none;
    }
  }
</style>
