<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { find, type Group } from '../../lib/groups.svelte';
  import { jumpToMessage } from '../../lib/jump';
  import type { Net } from '../../lib/net.svelte';
  import { onOpenTerminal } from '../../lib/notify.svelte';
  import { prefs } from '../../lib/prefs.svelte';
  import type { MsgRow } from '../../lib/proto';
  import { reminders } from '../../lib/push';
  import { href, route } from '../../lib/route.svelte';
  import type { ReminderTarget } from '../../sw';

  /** Keeps subscribed homes in sync across pages and opens a card chosen from a push reminder. */
  let { net }: { net: Net } = $props();

  /** The card to open in the chat, or on the terminal page it was chosen on if that shows it. */
  let target = $state.raw<ReminderTarget & { page?: 'terminal' }>();

  /** Whether the card is on the terminal page that is open. */
  function here({ g, msg }: ReminderTarget) {
    const group = find(g);
    const row = group?.rows.get(`msg ${msg}`) as MsgRow | undefined;
    return group && row && onOpenTerminal(group, row);
  }

  /** Whether the card may be one of the terminal page that is open: it is not held yet. */
  const maybeHere = ({ g, msg, kind }: ReminderTarget) =>
    kind === 'confirm' &&
    route.page === 'terminal' &&
    route.params.g === g &&
    !find(g)?.rows.has(`msg ${msg}`);

  function openTarget(next: ReminderTarget) {
    if (here(next)) return jumpToMessage(new Event('click'), next.msg, next.kind);
    if (maybeHere(next)) target = { ...next, page: 'terminal' };
    else openChat(next);
  }

  function openChat(next: ReminderTarget) {
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

  const opensOn = ({ g, page }: NonNullable<typeof target>) =>
    (route.page === 'chat' || (route.page === 'terminal' && page)) && route.params.g === g;

  $effect(() => {
    const pending = target;
    if (!pending) return;
    const group = find(pending.g);
    if (!group?.live || !opensOn(pending)) return;
    // Tried again when the group is read anew, if the connection was lost before the home answered.
    untrack(() => void open(group, pending));
  });

  /** Jumps to the message, however old; one that the home does not have is let go. */
  async function open(group: Group, pending: ReminderTarget) {
    const answered = await group.seek(pending.msg);
    if (!answered || target !== pending) return;
    if (route.page === 'terminal' && !here(pending)) return openChat(pending);
    target = undefined;
    await tick();
    jumpToMessage(new Event('click'), pending.msg, pending.kind);
  }
</script>
