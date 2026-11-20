<script lang="ts">
  import { parts } from '../../lib/compose';
  import { subject, type Group } from '../../lib/groups.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import type { MsgRow } from '../../lib/proto';
  import { clock } from '../../lib/when';
  import Avatar from '../Avatar.svelte';

  /** One message of the main line. `joined`: it follows a message of the same author closely, and
   *  is drawn under it without a head of its own. */
  let { group, msg, joined }: { group: Group; msg: MsgRow; joined: boolean } = $props();

  const author = $derived(group.member(msg.by));
  const time = $derived(clock(msg.ts, prefs.lang));
  // Only the members the row itself names are marked in its text.
  const text = $derived(
    parts(
      msg.text,
      group.members.filter((member) => msg.mentions.includes(member.id)),
    ),
  );
  const plain = $derived(msg.kind === 'text');
  // A card is drawn by its own component; until that is mounted, one line says what it is.
  const card = $derived(msg.state !== null || msg.kind === 'summary');
  const needsMe = $derived(group.waiting.includes(msg));
</script>

{#snippet body()}
  {#if text.length}
    <p class="text">
      {#each text as part, i (i)}
        {#if part.kind === 'code'}<code>{part.text}</code>{:else if part.kind === 'at'}<span
            class="at"
          >
            {part.text}
          </span>{:else}{part.text}{/if}
      {/each}
    </p>
  {/if}
  {#if card}
    <div class="card stub" class:need={needsMe}>
      <span class="kind">{t(`chat.kind.${msg.kind}`)}</span>
      <span class="grow cut">{subject(msg) === msg.text ? '' : subject(msg)}</span>
      {#if needsMe}
        <span class="badge solid" data-tone="dawn">{t('groups.waiting')}</span>
      {/if}
    </div>
  {/if}
{/snippet}

{#if msg.by === 'system'}
  <!-- Written by the home: a quiet line, with no author. -->
  <p class="msg note" id={`m-${msg.id}`}>
    <span class="badge">{t(`chat.kind.${msg.kind}`)}</span>
    {#if msg.kind === 'system'}
      <code>{msg.body.code}</code>
    {:else if msg.kind === 'task'}
      {@const task = group.rows.get(`task ${msg.body.task}`)}
      {#if task && 'title' in task}
        <span class="cut"><span class="mono">T-{task.num}</span> {task.title}</span>
      {/if}
    {/if}
    <time datetime={new Date(msg.ts).toISOString()}>{time}</time>
  </p>
{:else}
  <article class="msg" class:joined id={`m-${msg.id}`}>
    {#if joined}
      <time class="aside" datetime={new Date(msg.ts).toISOString()}>{time}</time>
    {:else}
      <Avatar
        id={author?.kind === 'human' ? author.user : msg.by}
        name={author?.name ?? '?'}
        cli={author?.kind === 'ai' ? author.cli : undefined}
      />
      <p class="msg-head">
        <b class="cut">{author?.name ?? t('chat.gone')}</b>
        {#if author?.kind === 'ai' && author.secretary}
          <span class="badge">{t('chat.secretary')}</span>
        {/if}
        {#if author && author.id === group.me?.id}
          <span class="badge">{t('chat.you')}</span>
        {/if}
        {#if !plain && !card}<span class="badge">{t(`chat.kind.${msg.kind}`)}</span>{/if}
        <time datetime={new Date(msg.ts).toISOString()}>{time}</time>
      </p>
    {/if}
    <div class="msg-body">{@render body()}</div>
  </article>
{/if}

<style>
  .msg {
    display: grid;
    grid-template-columns: 32px minmax(0, 1fr);
    column-gap: 10px;
    row-gap: 4px;
    margin-top: var(--sp-5);
    scroll-margin-top: var(--sp-12);
    /* A message arrives from just below its place. */
    animation:
      rise var(--dur-3) var(--ease-out),
      appear var(--dur-2) var(--ease-out);
  }
  .msg > :global(.av) {
    grid-row: 1 / span 2;
  }
  .msg-head {
    grid-column: 2;
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    min-width: 0;
    font-size: var(--fs-sm);
    line-height: 1.4;
  }
  .msg-head b {
    font-size: 14px;
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
    gap: var(--sp-2);
    min-width: 0;
  }
  /* Written as it was typed: line breaks kept, long words broken. */
  .text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  code {
    padding: 1px 5px;
    border-radius: var(--r-sm);
    background: var(--surface-2);
  }
  .joined {
    margin-top: var(--sp-1);
  }
  /* The time of a joined message shows in the gutter while the pointer is on the message. */
  .aside {
    grid-column: 1;
    align-self: start;
    justify-self: end;
    line-height: calc(var(--fs-md) * var(--lh-body));
    font-size: 11px;
    opacity: 0;
    transition: opacity var(--dur-1) var(--ease);
  }
  .joined:hover .aside,
  .joined:focus-within .aside {
    opacity: 1;
  }
  .note {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--sp-2);
    color: var(--text-3);
    font-size: var(--fs-sm);
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
    font-weight: 600;
  }
  @media (max-width: 640px) {
    .msg > :global(.av) {
      grid-row: 1;
    }
    .msg-body {
      grid-column: 1 / -1;
    }
    .aside {
      display: none;
    }
  }
</style>
