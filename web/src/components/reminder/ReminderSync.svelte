<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { find, type Group } from '../../lib/groups.svelte';
  import { jumpToMessage } from '../../lib/jump';
  import type { Net } from '../../lib/net.svelte';
  import { prefs } from '../../lib/prefs.svelte';
  import { reminders } from '../../lib/push';
  import { href, route } from '../../lib/route.svelte';
  import type { ReminderTarget } from '../../sw';

  /** Keeps subscribed homes in sync across pages and opens a card chosen from a push reminder. */
  let { net }: { net: Net } = $props();

  let target = $state.raw<ReminderTarget>();

  function openTarget(next: ReminderTarget) {
    target = next;
    const chatHash = href('chat', { g: next.g });
    if (location.hash !== chatHash) location.hash = chatHash;
  }

  onMount(() => {
    const params = new URLSearchParams(location.search);
    const msg = params.get('msg');
    const g = route.page === 'chat' ? route.params.g : undefined;
    if (msg && g) {
      const kind = params.get('kind') ?? '';
      history.replaceState(null, '', `${location.pathname}${location.hash}`);
      target = { g, msg, kind };
    }
    const sw = navigator.serviceWorker;
    if (!sw) return;
    const onMessage = (event: MessageEvent) => {
      const data = event.data as ReminderTarget;
      openTarget({ g: data.g, msg: data.msg, kind: data.kind });
    };
    sw.addEventListener('message', onMessage);
    return () => sw.removeEventListener('message', onMessage);
  });

  $effect(() => {
    const active = reminders.enabled;
    const lang = prefs.lang;
    void reminders.activeKinds;
    for (const home of net.homes) {
      void home.ctl;
      void home.groups;
    }
    if (active) void reminders.sync(net.homes, undefined, lang);
  });

  $effect(() => {
    const pending = target;
    if (!pending) return;
    const group = find(pending.g);
    if (!group?.live || route.page !== 'chat' || route.params.g !== pending.g) return;
    // Tried again when the group is read anew, if the connection was lost before the home answered.
    untrack(() => void open(group, pending));
  });

  /** Jumps to the message, however old; one that the home does not have is let go. */
  async function open(group: Group, pending: ReminderTarget) {
    const answered = await group.seek(pending.msg);
    if (!answered || target !== pending) return;
    target = undefined;
    await tick();
    jumpToMessage(new Event('click'), pending.msg, pending.kind);
  }
</script>
