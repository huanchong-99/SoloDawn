<script lang="ts">
  import { untrack } from 'svelte';
  import { prefersReducedMotion } from 'svelte/motion';
  import type { Group } from '../../lib/groups.svelte';
  import { mounts, side } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { dayLabel, days, now } from '../../lib/when';
  import Avatar from '../Avatar.svelte';
  import Icon from '../Icon.svelte';
  import Mount from '../Mount.svelte';
  import Composer from './Composer.svelte';
  import Message from './Message.svelte';

  /** The chat of one group: its main line, newest at the bottom, and the composer, with what the
   *  areas mount in its head, above its main line and beside it (lib/mounts.svelte.ts). */
  let { group }: { group: Group } = $props();

  // The side panel has a column from 1024 px; below that it slides over, closed on each chat.
  const sided = mounts['chat.side'].length > 0;
  side.open = false;
  let toggle = $state<HTMLButtonElement>();

  const paging = $derived(group.paging());
  const line = $derived(days(group.main));
  const newest = $derived(group.main.at(-1)?.id);
  // Older messages may exist and cannot be had now: the home refused a page, or is not reachable.
  const stuck = $derived(paging.failed || (paging.more && !group.home.ctl));
  let scroller: HTMLElement;
  let edge: HTMLElement;
  // The top of what has been read is in view, or nearly.
  let atEdge = $state(false);
  // The reader has scrolled back from the newest message, and one arrived since.
  let away = $state(false);
  let unseen = $state(false);
  // The person is using the keyboard: a key was pressed since the pointer was last used. The
  // focus that the main line is given when a chat opens counts as seen by the keyboard to a
  // browser after a page load; this does not.
  let keys = $state(false);

  $effect(() => {
    const seen = new IntersectionObserver(([entry]) => (atEdge = entry.isIntersecting), {
      root: scroller,
      rootMargin: '600px 0px 0px',
    });
    seen.observe(edge);
    return () => seen.disconnect();
  });
  // Scroll-back: while the top is in view the page before it is fetched, one page at a time.
  $effect(() => {
    if (atEdge && paging.ready && group.home.ctl) untrack(() => void group.older());
  });
  $effect(() => {
    void newest;
    if (untrack(() => away)) unseen = true;
  });
  // Opening a group moves keyboard focus to its main line, which then scrolls with the keys.
  $effect(() => scroller.focus({ preventScroll: true }));

  // The scroller's start is its bottom, so its offset is 0 at the newest message.
  function scrolled() {
    away = Math.abs(scroller.scrollTop) > 240;
    if (!away) unseen = false;
  }
  const toLatest = () =>
    scroller.scrollTo({ top: 0, behavior: prefersReducedMotion.current ? 'auto' : 'smooth' });
</script>

<svelte:window
  onkeydown={(event) => {
    keys = true;
    if (event.key === 'Escape' && side.open) {
      side.open = false;
      toggle?.focus();
    }
  }}
  onpointerdown={() => (keys = false)}
/>

<section class="chat" class:keys class:sided aria-label={group.name}>
  <header class="chat-head">
    <a class="icon-btn back" href="#/groups" aria-label={t('chat.back')}><Icon name="back" /></a>
    <Avatar id={group.g} name={group.name} group size="sm" />
    <div class="bar-title">
      <h2>{group.name}</h2>
      <p>
        {t('chat.project')} · {t('chat.members')}
        {group.members.length}
        {#if group.home.name}· <Icon name="home" sm /> {group.home.name}{/if}
      </p>
    </div>
    <Mount at="chat.head" props={{ group }} />
    {#if sided}
      <button
        class="icon-btn side-open"
        bind:this={toggle}
        aria-label={t('app.side')}
        aria-expanded={side.open}
        aria-controls="chat-side"
        onclick={() => (side.open = !side.open)}
      >
        <Icon name="users" />
      </button>
    {/if}
  </header>
  <div class="top"><Mount at="chat.top" props={{ group }} /></div>

  <!-- The scroller is reversed and holds one child: its start is the bottom, so the newest
       message is in view when it opens and stays there, and a page added above moves nothing.
       Opening the chat puts focus on it, so that the keys scroll it. -->
  <div
    class="scroller"
    bind:this={scroller}
    tabindex="-1"
    role="region"
    aria-label={t('chat.line')}
    onscroll={scrolled}
  >
    <div class="line">
      <p class="edge" bind:this={edge} role={paging.busy ? 'status' : undefined}>
        {#if stuck}
          {t('chat.older.offline')}
        {:else if paging.more}
          <span class="dots" class:busy={paging.busy} aria-hidden="true">
            <i></i><i></i><i></i>
          </span>
          <span class="sr">{t(paging.busy ? 'chat.older.busy' : 'chat.older')}</span>
        {:else if group.main.length}
          {t('chat.start')}
        {/if}
      </p>
      {#each line as day (day.key)}
        <section class="day">
          <p class="sep">
            <time datetime={new Date(day.day).toISOString()}>
              {dayLabel(day.day, now(), prefs.lang)}
            </time>
          </p>
          {#each day.msgs as { msg, joined } (msg.id)}
            <Message {group} {msg} {joined} />
          {/each}
        </section>
      {:else}
        {#if !paging.more}
          <div class="empty">
            <span class="horizon"></span>
            <h3>{t('chat.empty.title')}</h3>
            <p>{t('chat.empty.body')}</p>
          </div>
        {/if}
      {/each}
      {#if group.info && !group.info.secretary}
        <p class="nosec"><Icon name="alert" sm />{t('chat.nosecretary')}</p>
      {/if}
    </div>
  </div>

  <div class="dock">
    {#if away}
      <button class="btn btn-sm latest" onclick={toLatest}>
        <Icon name="down" sm />{t(unseen ? 'chat.latest.new' : 'chat.latest')}
        {#if unseen}<i class="dot"></i>{/if}
      </button>
    {/if}
    <Composer {group} />
  </div>
  {#if sided}
    <aside id="chat-side" class="side" class:open={side.open} aria-label={t('app.side')}>
      <button
        class="icon-btn side-close"
        aria-label={t('app.side.close')}
        onclick={() => {
          side.open = false;
          toggle?.focus();
        }}
      >
        <Icon name="x" />
      </button>
      <Mount at="chat.side" props={{ group }} />
    </aside>
  {/if}
</section>

<style>
  .chat {
    display: grid;
    grid-template: auto auto minmax(0, 1fr) auto / minmax(0, 1fr);
    min-height: 0;
    overflow: hidden;
  }
  .top {
    min-width: 0;
  }
  /* The side panel: a column beside the line from 1024 px, which the head's button does not
     need; below that, a panel that slides in over the chat from the right edge. */
  .side {
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: auto;
    border-left: 1px solid var(--line);
    background: var(--surface);
  }
  @media (min-width: 1024px) {
    .chat.sided {
      grid-template-columns: minmax(0, 1fr) var(--side-w);
    }
    .chat.sided > :not(.side) {
      grid-column: 1;
    }
    .side {
      grid-column: 2;
      grid-row: 1 / -1;
    }
    .side-open,
    .side-close {
      display: none;
    }
  }
  @media (max-width: 1023px) {
    .side {
      position: fixed;
      inset: var(--bar-h) 0 0 auto;
      z-index: 5;
      width: min(360px, 100%);
      box-shadow: var(--shadow-pop);
      transform: translateX(100%);
      visibility: hidden;
      transition:
        transform var(--dur-3) var(--ease-out),
        visibility 0s var(--dur-3);
    }
    .side.open {
      transform: none;
      visibility: visible;
      transition: transform var(--dur-4) var(--spring);
    }
    .side-close {
      align-self: flex-end;
      margin: var(--sp-2) var(--sp-2) 0;
    }
  }
  .chat-head {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    min-width: 0;
    height: var(--bar-h);
    padding: 0 var(--sp-4);
    border-bottom: 1px solid var(--line);
    background: var(--surface);
  }
  .chat-head h2 {
    font-size: var(--fs-md);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .chat-head p :global(.i) {
    vertical-align: -2px;
  }
  .scroller {
    display: flex;
    flex-direction: column-reverse;
    overflow-y: auto;
    overscroll-behavior: contain;
    outline: none;
  }
  /* The ring of the main line shows to someone at the keyboard, not for the focus it is given
     when the chat opens. */
  .keys .scroller:focus-visible {
    outline: var(--focus);
    outline-offset: -2px;
  }
  .line {
    width: 100%;
    max-width: calc(var(--measure) + 2 * var(--sp-4));
    margin-inline: auto;
    padding: var(--sp-2) var(--sp-4) var(--sp-6);
  }
  .edge {
    display: grid;
    place-items: center;
    min-height: 40px;
    color: var(--text-3);
    font-size: var(--fs-xs);
  }
  /* Three dots that breathe in turn while a page is on its way. */
  .dots {
    display: flex;
    gap: 5px;
  }
  .dots i {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: var(--text-3);
    opacity: 0.35;
  }
  .dots.busy i {
    animation: breathe 1.2s var(--ease) infinite;
  }
  .dots.busy i:nth-child(2) {
    animation-delay: 0.15s;
  }
  .dots.busy i:nth-child(3) {
    animation-delay: 0.3s;
  }
  @keyframes breathe {
    40% {
      opacity: 1;
      transform: translateY(-3px);
    }
  }
  /* The day stays at the top of the line while its messages pass under it. It stands on a band
     of the page's colour that fades out below it, so that no text shows above the day or is cut
     by it: the top edge of the line reads clean. */
  .sep {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    justify-content: center;
    margin-top: var(--sp-3);
    padding: var(--sp-2) 0 var(--sp-3);
    background: linear-gradient(var(--bg) 62%, transparent);
    pointer-events: none;
  }
  /* The band already keeps the first message of a day off the day's name. */
  .sep + :global(.msg) {
    margin-top: var(--sp-2);
  }
  .sep time {
    padding: 2px 12px;
    border: 1px solid var(--line);
    border-radius: var(--r-full);
    background: var(--bg);
    color: var(--text-3);
    font-size: var(--fs-xs);
    text-transform: capitalize;
  }
  .empty {
    display: grid;
    gap: var(--sp-2);
    margin-top: var(--sp-6);
    padding: var(--sp-5);
    border: 1px dashed var(--line-strong);
    border-radius: var(--r-lg);
    animation:
      rise var(--dur-3) var(--ease-out),
      appear var(--dur-2) var(--ease-out);
  }
  .empty h3 {
    font-size: var(--fs-lg);
  }
  .empty p {
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  .horizon {
    height: 2px;
    border-radius: 1px;
    background: var(--horizon);
  }
  .nosec {
    display: flex;
    align-items: flex-start;
    gap: var(--sp-2);
    margin-top: var(--sp-5);
    padding: 10px 12px;
    border-radius: var(--r-md);
    background: var(--dawn-soft);
    font-size: var(--fs-sm);
  }
  .nosec :global(.i) {
    margin-top: 4px;
    color: var(--dawn-text);
  }
  .dock {
    position: relative;
    min-width: 0;
  }
  /* Springs up over the composer when the reader has scrolled back. */
  .latest {
    position: absolute;
    left: 50%;
    bottom: calc(100% + var(--sp-3));
    z-index: 2;
    translate: -50% 0;
    border-radius: var(--r-full);
    box-shadow: var(--shadow-pop);
    animation:
      rise var(--dur-4) var(--spring),
      appear var(--dur-2) var(--ease-out);
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--sky);
  }
  @media (min-width: 1280px) {
    .back {
      display: none;
    }
  }
  @media (max-width: 767px) {
    .chat-head {
      padding: 0 var(--sp-3) 0 var(--sp-2);
    }
    /* On a phone the chat has the screen without the top bar. */
    .side {
      top: 0;
    }
  }
  @media (max-width: 640px) {
    .line {
      padding-inline: var(--sp-3);
    }
  }
</style>
