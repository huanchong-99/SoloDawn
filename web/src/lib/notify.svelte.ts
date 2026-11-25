// In-page notifications for live events that need or mention this browser's reader
// (planning/product-rules.md R-34, planning/contracts/commands.md section 1.7).
import { tick } from 'svelte';
import { isWatch, type Confirm } from './confirm';
import { subject, type Group, type Home } from './groups.svelte';
import { jumpToMessage } from './jump';
import { t } from './prefs.svelte';
import type { MsgRow, Row } from './proto';
import { href, route } from './route.svelte';
import { systemText } from './system';

export type NotifyKind =
  'approval' | 'decision' | 'brief' | 'confirm' | 'done' | 'mention' | 'alert';

/** One live notification shown in the shell's overlay layer. */
export interface Notice {
  readonly key: string;
  readonly group: Group;
  readonly msg: MsgRow;
  readonly kind: NotifyKind;
  readonly needsYou: boolean;
  readonly targetId: string;
}

/** Auto-dismiss delay in milliseconds for non-blocking notifications (`mention`, `done`). */
export const autoDismissMs = 8_000;

/** Maximum number of notifications shown at once. A newcomer past it drops the oldest self-closing
 *  notification, or the oldest waiting card when none is self-closing. */
export const maxNotices = 3;

const waitingKinds: Partial<Record<MsgRow['kind'], NotifyKind>> = {
  approval: 'approval',
  decision: 'decision',
  brief: 'brief',
  confirm: 'confirm',
};

const firstLine = (text: string): string => text.split('\n')[0];

function cardNotifyKind(group: Group, msg: MsgRow): NotifyKind | null | undefined {
  if (msg.kind === 'summary') return 'done';
  const kind = waitingKinds[msg.kind];
  if (!kind) return null;
  return group.waiting.includes(msg) ? kind : undefined;
}

const mentionsMe = (group: Group, msg: MsgRow): boolean =>
  group.me !== undefined && msg.mentions.includes(group.me.id);

/** Notification kind for a newly created message, or undefined when it does not notify this reader:
 *  a notice of the home that mentions them, as of a task that stopped, is an `alert`. */
export function classify(group: Group, msg: MsgRow): NotifyKind | undefined {
  if (msg.by === group.me?.id) return undefined;
  const card = cardNotifyKind(group, msg);
  if (card !== null) return card;
  if (!mentionsMe(group, msg)) return undefined;
  return msg.kind === 'system' ? 'alert' : 'mention';
}

/** The AI that a notice names, or the assignee of the task it names. */
const namedIn = (group: Group, { ai, task }: Partial<Record<'ai' | 'task', string>>) =>
  ai ?? group.task(task)?.assignee;

/** The member a message is about: of a card or a notice, the AI that its body names; of any other
 *  message, who wrote it. */
function aboutOf(group: Group, msg: MsgRow) {
  if (msg.kind === 'approval' || msg.kind === 'confirm') return msg.body.ai;
  return msg.kind === 'system' ? namedIn(group, msg.body as object) : msg.by;
}

/** Display name of the member a notification is about; of a bulletin, the AI that it names. */
export function whoOf(group: Group, msg: MsgRow): string {
  if (msg.kind === 'bulletin') return msg.body.ai;
  return group.member(aboutOf(group, msg))?.name ?? t('chat.gone');
}

/** Single-line summary shown beside the notification's kind label. */
export function titleOf(group: Group, msg: MsgRow): string {
  if (msg.kind === 'summary') {
    const task = group.task(msg.body.task);
    return task ? `T-${task.num} ${task.title}` : firstLine(msg.text);
  }
  return firstLine(msg.kind === 'system' ? systemText(group, msg) : subject(group, msg));
}

/** Main-line message id to scroll and focus when opening `msg` (the task card for a thread message). */
export function targetIdOf(group: Group, msg: MsgRow): string {
  if (msg.thread === null || msg.main) return msg.id;
  const taskMsg = group.main.find((m) => m.kind === 'task' && m.body.task === msg.thread);
  return taskMsg ? taskMsg.id : msg.thread;
}

const isMainLine = (msg: MsgRow): boolean => msg.thread === null || msg.main;

/** True when `msg` sits on the main line of the chat currently open on screen. */
export const inOpenChat = (g: string, msg: MsgRow): boolean =>
  route.page === 'chat' && route.params.g === g && isMainLine(msg);

const takenOver = (group: Group, id: string): boolean => {
  const ai = group.member(id);
  return ai?.kind === 'ai' && ai.state === 'takeover';
};

/** True for a request to watch the takeover whose terminal page is open on screen. That page shows
 *  the card: to leave it for the chat would end the takeover. */
export const onOpenTerminal = (group: Group, msg: MsgRow): msg is Confirm =>
  isWatch(msg) &&
  takenOver(group, msg.body.ai) &&
  route.page === 'terminal' &&
  route.params.g === group.g &&
  route.params.member === msg.body.ai;

const onScreen = (group: Group, msg: MsgRow): boolean =>
  inOpenChat(group.g, msg) || onOpenTerminal(group, msg);

const isLiveMsg = (row: Row, prev: number): row is MsgRow =>
  row.r === 'msg' && row.seq > prev && (row as MsgRow).pos === row.seq;

const highestSeq = (group: Group): number =>
  Math.max(0, ...[...group.rows.values()].map((row) => row.seq));

const groupListed = (allHomes: readonly Home[], group: Group): boolean =>
  allHomes.includes(group.home) && group.home.groups.includes(group);

function rowStillActive(item: Notice): boolean {
  const row = item.group.rows.get(`msg ${item.msg.id}`);
  if (!row || 'gone' in row) return false;
  return !item.needsYou || item.group.waiting.some((m) => m.id === item.msg.id);
}

const stillValid = (allHomes: readonly Home[], item: Notice): boolean =>
  groupListed(allHomes, item.group) && !onScreen(item.group, item.msg) && rowStillActive(item);

/** Reactive store of live in-page notifications across connected homes. */
export class Notifier {
  items = $state<Notice[]>([]);
  #seen = new WeakMap<Group, number>();
  #timers = new Map<string, ReturnType<typeof setTimeout>>();
  #held = new Map<string, number>();

  /** Updates visible notifications from the current `homes` and active route. */
  sync(allHomes: readonly Home[]): void {
    this.#prune(allHomes);
    for (const home of allHomes) {
      for (const group of home.groups) this.#scan(group);
    }
  }

  /** Dismisses one notification by `key`. */
  dismiss(key: string): void {
    this.#drop(key);
    this.items = this.items.filter((item) => item.key !== key);
  }

  /** Holds `key`'s auto-dismissal while focus or the pointer is on its notification. */
  hold(key: string): void {
    this.#held.set(key, (this.#held.get(key) ?? 0) + 1);
    this.#clearTimer(key);
  }

  /** Lets go of one hold on `key`; the last restarts a self-closing notification's full delay. */
  release(key: string): void {
    const held = (this.#held.get(key) ?? 1) - 1;
    if (held > 0) return void this.#held.set(key, held);
    this.#held.delete(key);
    if (this.items.some((item) => item.key === key && !item.needsYou)) this.#arm(key);
  }

  /** Clears all timers and visible notifications. */
  clear(): void {
    for (const timer of this.#timers.values()) clearTimeout(timer);
    this.#timers.clear();
    this.#held.clear();
    this.items = [];
  }

  /** Opens the notification's group chat, dismisses it, and focuses its target message. */
  async open(event: Event, item: Notice): Promise<void> {
    event.preventDefault();
    this.dismiss(item.key);
    if (route.page !== 'chat' || route.params.g !== item.group.g) {
      const moved = new Promise((resolve) =>
        addEventListener('hashchange', resolve, { once: true }),
      );
      location.hash = href('chat', { g: item.group.g });
      await moved;
      await tick();
    }
    jumpToMessage(event, item.targetId, item.msg.kind);
  }

  #prune(allHomes: readonly Home[]): void {
    const kept: Notice[] = [];
    for (const item of this.items) {
      if (stillValid(allHomes, item)) kept.push(item);
      else this.#drop(item.key);
    }
    if (kept.length !== this.items.length) this.items = kept;
  }

  #baseline(group: Group, high: number): number | undefined {
    if (group.cursor === 0 && high === 0) {
      this.#seen.delete(group);
      return undefined;
    }
    const prev = this.#seen.get(group);
    if (prev === undefined || high > group.cursor) {
      this.#seen.set(group, Math.max(group.cursor, high));
      return undefined;
    }
    return prev;
  }

  #scan(group: Group): void {
    const high = highestSeq(group);
    const prev = this.#baseline(group, high);
    if (prev === undefined) return;
    const arrivals = [...group.rows.values()]
      .filter((row): row is MsgRow => isLiveMsg(row, prev))
      .sort((a, b) => a.pos - b.pos);
    for (const msg of arrivals) this.#push(group, msg);
    this.#seen.set(group, Math.max(prev, high));
  }

  #push(group: Group, msg: MsgRow): void {
    if (onScreen(group, msg)) return;
    const kind = classify(group, msg);
    if (!kind) return;
    const needsYou = kind in waitingKinds;
    this.#append({
      key: `${group.g}:${msg.id}`,
      group,
      msg,
      kind,
      needsYou,
      targetId: targetIdOf(group, msg),
    });
  }

  #append(notice: Notice): void {
    const next = [...this.items, notice];
    if (!notice.needsYou) this.#arm(notice.key);
    if (next.length > maxNotices) {
      const selfClosing = next.findIndex((item) => !item.needsYou);
      const [dropped] = next.splice(Math.max(0, selfClosing), 1);
      this.#drop(dropped.key);
    }
    this.items = next;
  }

  #arm(key: string): void {
    this.#timers.set(
      key,
      setTimeout(() => this.dismiss(key), autoDismissMs),
    );
  }

  #drop(key: string): void {
    this.#clearTimer(key);
    this.#held.delete(key);
  }

  #clearTimer(key: string): void {
    const timer = this.#timers.get(key);
    if (timer !== undefined) {
      clearTimeout(timer);
      this.#timers.delete(key);
    }
  }
}
