<script lang="ts">
  import { flip } from 'svelte/animate';
  import { prefersReducedMotion } from 'svelte/motion';
  import { homes, recent, type Group } from '../../lib/groups.svelte';
  import type { Net } from '../../lib/net.svelte';
  import { titleOf } from '../../lib/notify.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import type { MsgRow } from '../../lib/proto';
  import { now, shortWhen } from '../../lib/when';
  import Avatar from '../Avatar.svelte';
  import Icon from '../Icon.svelte';

  /** `open` is the group whose chat is shown; `oncreate` opens the create-group dialog. */
  let { open, net, oncreate }: { open?: Group; net: Net; oncreate: () => void } = $props();

  const groups = $derived(recent());
  // A group is created on a node of the person's own that can be reached.
  const canCreate = $derived(homes.some((home) => home.owner && home.ctl));

  /** Who wrote a message: for a bulletin the AI it names, not the person whose computer sent it. */
  function author(group: Group, msg: MsgRow) {
    if (msg.kind === 'bulletin') return msg.body.ai;
    return msg.by === 'system' ? t('chat.system') : group.nameOf(msg.by);
  }

  /** Who wrote the last message of the main line, and what it says; nothing of a group not read. */
  function preview(group: Group) {
    const last = group.main.at(-1);
    if (!last) return group.info ? t('groups.none') : '';
    const said = titleOf(group, last) || t(`chat.kind.${last.kind}`);
    return `${author(group, last)}${t('groups.colon')}${said}`;
  }
</script>

<header class="head">
  <div class="grow">
    <h2 class="sec-title">{t('groups.all')}</h2>
    <p class="hint">{t('groups.lead')}</p>
  </div>
  <button class="btn btn-sm" onclick={oncreate} disabled={!canCreate}>
    <Icon name="plus" sm /><span>{t('groups.new')}</span>
  </button>
</header>

{#if groups.length}
  <ul class="glist">
    {#each groups as group, i (group.g)}
      {@const last = group.main.at(-1)}
      <li animate:flip={{ duration: prefersReducedMotion.current ? 0 : 240 }}>
        <a
          class="gitem"
          href={`#/groups/${group.g}`}
          aria-current={group === open ? 'page' : undefined}
          style:--i={Math.min(i, 8)}
        >
          <Avatar id={group.g} name={group.name} group />
          <span class="gbody">
            <b class="cut">{group.name}</b>
            <span class="gprev cut">{preview(group)}</span>
            {#if !group.home.ctl}
              <span class="gdown cut" class:away={net.otherTab}>
                <i></i>{net.otherTab
                  ? t('groups.otherTab')
                  : [group.home.name, t('groups.down')].filter(Boolean).join(' · ')}
              </span>
            {:else if group.info?.paused}
              <span class="st" data-st="paused"><i></i>{t('stop.list')}</span>
            {/if}
          </span>
          <span class="gside">
            {#if last}
              <time datetime={new Date(last.ts).toISOString()}>
                {shortWhen(last.ts, now(), prefs.lang)}
              </time>
            {/if}
            {#if group.waiting.length}
              {#key group.waiting.length}
                <!-- Part of the link's name: "Waiting on you 2". -->
                <span class="badge solid count" data-tone="dawn">
                  <span class="sr">{t('groups.waiting')}</span>
                  {group.waiting.length}
                </span>
              {/key}
            {/if}
          </span>
        </a>
      </li>
    {/each}
  </ul>
{:else}
  <div class="empty blank">
    <span class="blank-mark"><Icon name="chat" /></span>
    <h3>{t('groups.empty.title')}</h3>
    <p>{t(canCreate ? 'groups.empty.body' : 'groups.empty.offline')}</p>
    {#if canCreate}
      <button class="btn btn-primary" onclick={oncreate}>
        <Icon name="plus" sm /><span>{t('groups.new')}</span>
      </button>
    {:else}
      <a class="btn" href="#/nodes">{t('groups.empty.nodes')}</a>
    {/if}
  </div>
{/if}

<style>
  .head {
    display: flex;
    align-items: center;
    gap: var(--sp-3);
    padding: var(--sp-4) var(--sp-4) var(--sp-2) var(--sp-5);
  }
  .head h2 {
    color: var(--text);
    font-size: var(--fs-md);
    letter-spacing: normal;
    text-transform: none;
  }
  .glist {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--sp-1) var(--sp-2) var(--sp-4);
  }
  /* A row is one link, 64px. Its hover is a layer under it, as for a button; the open group is
     a raised, neutral row: never the accent, which would tire the eye in a list. */
  .gitem {
    position: relative;
    isolation: isolate;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    gap: var(--sp-3);
    align-items: center;
    min-height: 64px;
    padding: 10px var(--sp-3);
    border-radius: 12px;
    color: var(--text);
    animation:
      rise var(--dur-3) var(--ease-out) calc(var(--i) * var(--stagger)) backwards,
      appear var(--dur-2) var(--ease-out) calc(var(--i) * var(--stagger)) backwards;
  }
  .gitem::before,
  .gitem::after {
    content: '';
    position: absolute;
    inset: 0;
    z-index: -1;
    border-radius: inherit;
    opacity: 0;
    transition: opacity var(--dur-1) var(--ease);
  }
  .gitem::before {
    background: var(--surface-3);
  }
  .gitem::after {
    background: var(--raised);
    box-shadow:
      var(--shadow-1),
      0 0 0 1px var(--line);
  }
  .gitem:hover {
    text-decoration: none;
  }
  .gitem:active::before {
    opacity: 0.7;
  }
  .gitem[aria-current]::after {
    opacity: 1;
  }
  @media (hover: hover) {
    .gitem:hover::before {
      opacity: 0.7;
    }
  }
  @media (forced-colors: active) {
    .gitem[aria-current] {
      outline: 2px solid Highlight;
      outline-offset: -2px;
    }
  }
  .gbody {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    line-height: 1.4;
  }
  .gbody b {
    font-weight: 500;
  }
  .gprev {
    color: var(--text-3);
    font-size: var(--fs-sm);
  }
  .gdown {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--danger);
    font-size: var(--fs-xs);
  }
  .gbody .st {
    color: var(--text-3);
    font-size: var(--fs-xs);
  }
  .gdown.away {
    color: var(--text-2);
  }
  /* Cannot reach the home: a hollow circle, told by its shape as well as its colour. */
  .gdown i {
    flex: none;
    width: 8px;
    height: 8px;
    border: 1.5px solid currentColor;
    border-radius: 50%;
  }
  .gside {
    display: grid;
    gap: 6px;
    justify-items: end;
    align-content: start;
    align-self: start;
    padding-top: 2px;
    color: var(--text-3);
    font-size: var(--fs-xs);
    line-height: 1.4;
  }
  .gside time {
    text-transform: capitalize;
  }
  /* The count lands with a spring each time it changes. */
  .gside .badge {
    animation: pop var(--dur-4) var(--spring);
  }
  .empty {
    margin-top: var(--sp-8);
  }
</style>
