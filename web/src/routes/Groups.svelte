<script lang="ts">
  import { onMount } from 'svelte';
  import Chat from '../components/chat/Chat.svelte';
  import CreateGroup from '../components/groups/CreateGroup.svelte';
  import GroupList from '../components/groups/GroupList.svelte';
  import Waiting from '../components/groups/Waiting.svelte';
  import { find, Home, homes } from '../lib/groups.svelte';
  import { t } from '../lib/prefs.svelte';

  // The group that the address names is open: #/groups/<group id>.
  let hash = $state(location.hash);
  const open = $derived(find(hash.split('/')[2]));
  let creating = $state(false);

  // Development only, until a real connection is handed to the pages: one home answered from the
  // node's wire examples. The build leaves this out, and the examples with it.
  onMount(async () => {
    if (import.meta.env.MODE !== 'development' || location.search.includes('nomock')) return;
    const [{ mockCtl, goldens }, { loadDevice }] = await Promise.all([
      import('../lib/mock'),
      import('../lib/identity'),
    ]);
    const device = await loadDevice({ crypto, idb: () => indexedDB });
    const user: string = JSON.parse(goldens['row-member-human']).user;
    const home = new Home({ user, sign: (payload) => device.sign(payload) });
    const ctl = mockCtl(home.push);
    const welcome = await ctl.request({
      t: 'hello',
      v: 1,
      kind: 'browser',
      name: 'development',
      addrs: [],
    });
    if ('err' in welcome) return;
    homes.push(home);
    home.online(ctl, welcome);
  });
</script>

<svelte:window onhashchange={() => (hash = location.hash)} />

<div class="groups" class:open>
  <section class="list" aria-label={t('groups.list')}>
    <Waiting bar />
    <GroupList {open} oncreate={() => (creating = true)} />
  </section>
  {#if open}
    {#key open}<Chat group={open} />{/key}
  {:else}
    <div class="rest"><Waiting /></div>
  {/if}
</div>
<CreateGroup bind:open={creating} />

<style>
  /* Wide: the list beside what is open. Narrow: one of the two. (design notes, 3.4) */
  .groups {
    display: grid;
    grid-template-columns: minmax(300px, 380px) minmax(0, 1fr);
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
  .list {
    border-right: 1px solid var(--line);
    background: var(--surface);
    overflow: auto;
  }
  .rest {
    overflow: auto;
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
  /* On a phone an open chat also takes the place of the top bar and the bottom navigation: its
     own head is the bar, and the room goes to the messages and the field (design notes, 3.4). */
  @media (max-width: 767px) {
    :global(.app:has(.groups.open)) {
      grid-template: 'main' minmax(0, 1fr) / minmax(0, 1fr);
    }
    :global(.app:has(.groups.open) > :is(.bar, .rail)) {
      display: none;
    }
  }
</style>
