<script lang="ts">
  import { flip } from 'svelte/animate';
  import { prefersReducedMotion } from 'svelte/motion';
  import { homes, recent, subject, type Group } from '../../lib/groups.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { now, shortWhen } from '../../lib/when';
  import Avatar from '../Avatar.svelte';
  import Icon from '../Icon.svelte';

  /** `open` is the group whose chat is shown; `oncreate` opens the create-group dialog. */
  let { open, oncreate }: { open?: Group; oncreate: () => void } = $props();

  const groups = $derived(recent());
  // A group is created on a node of the person's own that can be reached.
  const canCreate = $derived(homes.some((home) => home.owner && home.ctl));

  /** Who wrote the last message of the main line, and what it says. */
  function preview(group: Group) {
    const last = group.main.at(-1);
    if (!last) return t('groups.none');
    const by = last.by === 'system' ? t('chat.system') : group.member(last.by)?.name;
    const said = subject(last) || t(`chat.kind.${last.kind}`);
    return `${by ?? t('chat.gone')}${t('groups.colon')}${said}`;
  }
</script>

<header class="head">
  <div class="grow">
    <h2 class="sec-title">{t('groups.projects')}</h2>
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
      <li animate:flip={{ duration: prefersReducedMotion.current ? 0 : 320 }}>
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
              <span class="gdown cut"><i></i>{group.home.name} · {t('groups.down')}</span>
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
  <div class="empty">
    <span class="horizon"></span>
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
    padding: var(--sp-4) var(--sp-4) var(--sp-2);
  }
  .glist {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--sp-2);
  }
  /* A row is one link. Its hover and current states are a layer under it, as for a button. */
  .gitem {
    position: relative;
    isolation: isolate;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    gap: var(--sp-3);
    align-items: center;
    padding: 10px;
    border-radius: var(--r-md);
    color: var(--text);
    animation:
      rise var(--dur-3) var(--ease-out) calc(var(--i) * 30ms) backwards,
      appear var(--dur-2) var(--ease-out) calc(var(--i) * 30ms) backwards;
  }
  .gitem::before {
    content: '';
    position: absolute;
    inset: 0;
    z-index: -1;
    border-radius: inherit;
    background: var(--surface-2);
    opacity: 0;
    transition: opacity var(--dur-1) var(--ease);
  }
  .gitem:hover {
    text-decoration: none;
  }
  .gitem[aria-current]::before,
  .gitem:active::before {
    opacity: 1;
  }
  @media (hover: hover) {
    .gitem:hover::before {
      opacity: 1;
    }
  }
  .gbody {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
    line-height: 1.4;
  }
  .gprev {
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  .gdown {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--danger);
    font-size: var(--fs-xs);
  }
  /* Cannot reach the home: a hollow circle, told by its shape as well as its colour. */
  .gdown i {
    flex: none;
    width: 8px;
    height: 8px;
    border: 2px solid currentColor;
    border-radius: 50%;
  }
  .gside {
    display: grid;
    gap: 4px;
    justify-items: end;
    align-content: start;
    align-self: start;
    color: var(--text-3);
    font-size: var(--fs-xs);
  }
  .gside time {
    text-transform: capitalize;
  }
  /* The count lands with a spring each time it changes. */
  .gside .badge {
    animation: pop var(--dur-4) var(--spring);
  }
  @keyframes pop {
    from {
      transform: scale(0.4);
    }
  }
  .empty {
    display: grid;
    justify-items: start;
    gap: var(--sp-3);
    margin: var(--sp-4);
    padding: var(--sp-6) var(--sp-5);
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
  /* The one line of first light: here is where a first group will be. */
  .horizon {
    width: 100%;
    height: 2px;
    border-radius: 1px;
    background: var(--horizon);
  }
</style>
