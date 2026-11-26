<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { prefersReducedMotion } from 'svelte/motion';
  import { MediaQuery } from 'svelte/reactivity';
  import { withoutInlineReplies } from '../../lib/call';
  import type { Group } from '../../lib/groups.svelte';
  import { release } from '../../lib/jump';
  import type { Net } from '../../lib/net.svelte';
  import { card as cardOf, mounts, side } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { dayLabel, days, now } from '../../lib/when';
  import Avatar from '../Avatar.svelte';
  import Icon from '../Icon.svelte';
  import Mount from '../Mount.svelte';
  import Composer from './Composer.svelte';
  import Message from './Message.svelte';

  /** The chat of one group: its main line, newest at the bottom, and the composer, with what the
   *  areas mount in its head, above its main line and beside it (lib/mounts.svelte.ts). */
  let { group, net }: { group: Group; net: Net } = $props();

  // The side panel has a column from 1024 px; below that it slides over, closed on each chat.
  // While it is over the chat, focus is in it and the chat behind it cannot be reached.
  const sided = mounts['chat.side'].length > 0;
  side.open = false;
  const sliding = new MediaQuery('max-width: 1023px');
  const covered = $derived(side.open && sliding.current);
  let toggle = $state<HTMLButtonElement>();
  let closer = $state<HTMLButtonElement>();

  async function openSide() {
    side.open = true;
    await tick();
    if (sliding.current) closer?.focus();
  }
  async function closeSide() {
    side.open = false;
    await tick();
    toggle?.focus();
  }
  // Escape closes the panel over the chat, unless a dialog took it, or a part already did.
  function escape(event: KeyboardEvent) {
    const inDialog = event.target instanceof Element && event.target.closest('dialog');
    if (event.key === 'Escape' && covered && !event.defaultPrevented && !inDialog) closeSide();
  }

  const paging = $derived(group.paging());
  const line = $derived(days(withoutInlineReplies(group.main, Boolean(cardOf('call')))));
  // A group of people only has nobody to turn a request into a brief.
  const hasAI = $derived(group.members.some((member) => member.kind === 'ai'));
  const emptyBody = $derived(hasAI ? 'chat.empty.body' : 'chat.empty.people');
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

  // The observer can look twice before it tells, oldest first: as when a jump scrolls to the top
  // in the frame after the line drew the newest messages.
  $effect(() => {
    const seen = new IntersectionObserver((looks) => (atEdge = looks.at(-1)!.isIntersecting), {
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
  function toLatest() {
    release();
    scroller.scrollTo({ top: 0, behavior: prefersReducedMotion.current ? 'auto' : 'smooth' });
    scroller.focus({ preventScroll: true });
  }
</script>

<svelte:window
  onkeydown={(event) => {
    keys = true;
    escape(event);
  }}
  onpointerdown={() => (keys = false)}
/>

<section class="chat" class:keys class:sided aria-label={group.name}>
  <header class="chat-head" inert={covered}>
    <a class="icon-btn back" href="#/groups" aria-label={t('chat.back')}><Icon name="back" /></a>
    <Avatar id={group.g} name={group.name} group size="sm" />
    <div class="bar-title">
      <h2>{group.name}</h2>
      <p>
        {#if group.info}
          {t('chat.members')}
          {group.members.length}
          {#if group.home.name}· <Icon name="home" sm /> {group.home.name}{/if}
        {:else}&nbsp;{/if}
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
        onclick={() => (side.open ? closeSide() : openSide())}
      >
        <Icon name="users" />
      </button>
    {/if}
  </header>
  <div class="top" inert={covered}>
    <Mount at="chat.top" props={{ group }} />
  </div>

  <!-- The scroller is reversed and holds one child: its start is the bottom, so the newest
       message is in view when it opens and stays there, and a page added above moves nothing.
       Opening the chat puts focus on it, so that the keys scroll it. -->
  <div
    class="scroller"
    bind:this={scroller}
    inert={covered}
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
          <div class="empty blank">
            <span class="blank-mark"><Icon name="chat" /></span>
            <h3>{t('chat.empty.title')}</h3>
            <p>{t(emptyBody)}</p>
          </div>
        {/if}
      {/each}
      {#if group.info && hasAI && !group.info.secretary}
        <p class="nosec"><Icon name="alert" sm />{t('chat.nosecretary')}</p>
      {/if}
    </div>
  </div>

  <div class="dock" inert={covered}>
    {#if away}
      <button class="btn btn-sm latest" onclick={toLatest}>
        <Icon name="down" sm />{t(unseen ? 'chat.latest.new' : 'chat.latest')}
        {#if unseen}<i class="dot"></i>{/if}
      </button>
    {/if}
    <Mount at="chat.write" props={{ group }} />
    <Composer {group} {net} />
  </div>
  {#if sided}
    <aside id="chat-side" class="side" class:open={side.open} aria-label={t('app.side')}>
      <button
        class="icon-btn side-close"
        bind:this={closer}
        aria-label={t('app.side.close')}
        onclick={closeSide}
      >
        <Icon name="x" />
      </button>
      <Mount at="chat.side" props={{ group, net }} />
    </aside>
  {/if}
</section>

<style>
  /* The chat is the content: it stands on the card colour, while the navigation and the group
     list beside it stay on the page colour. */
  .chat {
    display: grid;
    grid-template: auto auto minmax(0, 1fr) auto / minmax(0, 1fr);
    min-height: 0;
    overflow: hidden;
    background: var(--surface);
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
    /* It comes in over 300 ms and leaves faster than it came. */
    .side {
      position: fixed;
      inset: var(--bar-h) 0 0 auto;
      z-index: 5;
      width: min(360px, 100%);
      border-left: 0;
      box-shadow: var(--shadow-dlg);
      transform: translateX(100%);
      visibility: hidden;
      transition:
        transform var(--dur-out) var(--ease-in),
        visibility 0s var(--dur-out);
    }
    .side.open {
      transform: none;
      visibility: visible;
      transition: transform var(--dur-3) var(--ease-out);
    }
    /* A scrim over the chat behind the panel; it fades in with it. On the chat, since the
       panel's own scrolling would clip it. */
    .chat::after {
      content: '';
      position: fixed;
      inset: var(--bar-h) 0 0;
      z-index: 4;
      background: var(--scrim);
      opacity: 0;
      pointer-events: none;
      transition: opacity var(--dur-out) var(--ease-in);
    }
    .chat:has(.side.open)::after {
      opacity: 1;
      transition: opacity var(--dur-2) var(--ease-out);
    }
    .side-close {
      align-self: flex-end;
      margin: var(--sp-2) var(--sp-2) 0;
    }
  }
  .chat-head {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
    height: 56px;
    padding: 0 var(--sp-4) 0 var(--sp-5);
    border-bottom: 1px solid var(--line);
  }
  .chat-head h2 {
    font-size: var(--fs-md);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .chat-head .bar-title {
    min-width: 3ch;
  }
  .chat-head p :global(.i) {
    width: 12px;
    height: 12px;
    vertical-align: -1.5px;
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
    max-width: calc(var(--measure) + 2 * var(--sp-6));
    margin-inline: auto;
    padding: 0 var(--sp-6) var(--sp-5);
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
     of the canvas colour, so that nothing shows above the day and nothing is covered by it: the
     top edge of the line reads clean. */
  .sep {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    justify-content: center;
    margin: var(--sp-3) calc(-1 * var(--sp-2)) 0;
    padding: 6px 0;
    background: var(--surface);
    pointer-events: none;
  }
  .sep time {
    display: inline-flex;
    align-items: center;
    height: 24px;
    padding: 0 12px;
    border-radius: var(--r-full);
    background: var(--surface-2);
    color: var(--text-2);
    font-size: var(--fs-xs);
    text-transform: capitalize;
  }
  /* The band already keeps the first message of a day off the day's name. */
  .sep + :global(.msg) {
    margin-top: var(--sp-2);
  }
  /* A quiet word under the line: this group has no secretary. Told by its glyph and its words. */
  .nosec {
    display: flex;
    align-items: flex-start;
    justify-content: center;
    gap: 6px;
    margin-top: var(--sp-5);
    color: var(--text-2);
    font-size: var(--fs-sm);
    line-height: 1.5;
  }
  .nosec :global(.i) {
    margin-top: 2px;
    color: var(--dawn-text);
  }
  .dock {
    position: relative;
    min-width: 0;
  }
  /* Comes up over the composer when the reader has scrolled back. */
  .latest {
    position: absolute;
    left: 50%;
    bottom: calc(100% + var(--sp-3));
    z-index: 2;
    translate: -50% 0;
    height: 32px;
    padding: 0 14px 0 10px;
    border-color: transparent;
    border-radius: var(--r-full);
    box-shadow: var(--shadow-pop);
    animation:
      rise var(--dur-2) var(--ease-out),
      appear var(--dur-2) var(--ease-out);
  }
  .latest::before {
    border-radius: inherit;
  }
  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--sky);
  }
  @media (min-width: 1280px) {
    .back {
      display: none;
    }
  }
  @media (max-width: 1279px) {
    .chat-head {
      padding-left: var(--sp-2);
      gap: var(--sp-2);
    }
  }
  @media (max-width: 767px) {
    .chat-head {
      padding: 0 var(--sp-2) 0 var(--sp-1);
      gap: var(--sp-1);
    }
    /* On a phone the chat has the screen without the top bar. The group's name is worth more
       there than its picture. */
    .chat-head > :global(.av) {
      display: none;
    }
    .side {
      top: 0;
    }
  }
  @media (max-width: 640px) {
    .line {
      padding-inline: var(--sp-4);
    }
  }
</style>
