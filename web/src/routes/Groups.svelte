<script lang="ts">
  import { untrack } from 'svelte';
  import Chat from '../components/chat/Chat.svelte';
  import CreateGroup from '../components/groups/CreateGroup.svelte';
  import GroupList from '../components/groups/GroupList.svelte';
  import Waiting from '../components/groups/Waiting.svelte';
  import Icon from '../components/Icon.svelte';
  import { creating as createAsk, find, takeCreate } from '../lib/groups.svelte';
  import type { Net } from '../lib/net.svelte';
  import { t } from '../lib/prefs.svelte';
  import { href } from '../lib/route.svelte';

  /** The group list (`#/groups`), and the chat of the group the address names (`#/groups/<g>`). */
  let { params, net }: { params: { g?: string }; net: Net } = $props();

  const open = $derived(find(params.g));
  let creating = $state(takeCreate());
  $effect(() => {
    if (createAsk.open && untrack(takeCreate)) creating = true;
  });
  // The address names a chat: the page is laid out for it, whether or not the group has been read.
</script>

<div class="groups" class:open={params.g !== undefined}>
  <section class="list" aria-label={t('groups.list')}>
    <Waiting bar />
    <GroupList {open} {net} oncreate={() => (creating = true)} />
  </section>
  {#if open}
    {#key open}<Chat group={open} {net} />{/key}
  {:else if params.g !== undefined}
    <!-- A group this browser has not read: not yet, or it is homed on no computer it reaches. -->
    <div class="missing">
      <a class="icon-btn back" href={href('groups')} aria-label={t('chat.back')}>
        <Icon name="back" />
      </a>
      <p>{t('groups.missing')}</p>
    </div>
  {:else}
    <div class="rest"><Waiting /></div>
  {/if}
</div>
<CreateGroup bind:open={creating} />

<style>
  /* Wide: the list beside what is open. Narrow: one of the two. (design notes, 3.4) */
  .groups {
    display: grid;
    grid-template-columns: minmax(300px, 360px) minmax(0, 1fr);
    height: 100%;
  }
  .groups > :global(*) {
    min-width: 0;
    min-height: 0;
    /* The page arrives from the side of its entry in the navigation, as every page does. */
    animation:
      arrive var(--dur-3) var(--ease-out) backwards,
      appear var(--dur-2) var(--ease-out);
  }
  .groups > :global(:nth-child(2)) {
    animation-delay: var(--stagger), 0s;
  }
  /* The list recedes: it stands on the page colour, as the navigation does. */
  .list {
    border-right: 1px solid var(--line);
    overflow: auto;
  }
  .rest {
    overflow: auto;
  }
  /* Where the chat will be: the way back, and a quiet word that the group is not here yet. */
  .missing {
    display: grid;
    align-content: start;
    justify-items: start;
    gap: var(--sp-4);
    padding: var(--sp-2) var(--sp-4);
    background: var(--surface);
  }
  .missing p {
    max-width: 420px;
    padding: var(--sp-3) var(--sp-4);
    border-radius: var(--r-md);
    background: var(--surface-2);
    color: var(--text-2);
    font-size: var(--fs-ui);
    animation: appear var(--dur-2) var(--ease-out);
  }
  @media (min-width: 1280px) {
    .missing .back {
      display: none;
    }
    .missing {
      place-content: center;
      justify-items: center;
    }
  }
  .groups.open {
    grid-template-columns: var(--list-w) minmax(0, 1fr);
  }
  /* The waiting bar is for the page that has no room for the overview beside the list. */
  @media (min-width: 900px) {
    .list > :global(.needbar) {
      display: none;
    }
  }
  @media (max-width: 899px) {
    .groups:not(.open) {
      grid-template-columns: minmax(0, 1fr);
    }
    .groups:not(.open) .rest {
      display: none;
    }
    .groups:not(.open) .list {
      border: 0;
    }
  }
  /* Below 1280px an open chat has the page to itself; its back button leads to the list. */
  @media (max-width: 1279px) {
    .groups.open {
      grid-template-columns: minmax(0, 1fr);
    }
    .groups.open .list {
      display: none;
    }
  }
</style>
