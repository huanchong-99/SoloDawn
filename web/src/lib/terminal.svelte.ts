// Terminal takeover: the commands, the byte channel, the takeover of one page and what it shows
// (planning/flows/takeover.md, planning/contracts/datachannel.md sections 1 and 7,
// planning/contracts/commands.md section 2).
import { lost, newId, type Group, type Home } from './groups.svelte';
import type { Path } from './net.svelte';
import { t } from './prefs.svelte';
import type * as p from './proto';

/** A key of the phone key row (`web/design/terminal.html`): `Ctrl` is sticky, the others send `seq`. */
export interface PhoneKey {
  label: string;
  seq: string;
  ctrl?: true;
  ariaKey?: 'terminal.key.up' | 'terminal.key.down' | 'terminal.key.enter';
}

/** Where the takeover of a page is. */
export type TakeoverPhase =
  'idle' | 'starting' | 'active' | 'relay' | 'relayed' | 'ended' | 'error';

/** What the takeover bar says. */
export interface BarView {
  title: string;
  detail?: string;
  errorCode?: string;
  handback: boolean;
}

/** The `pty:<g>:<member>` data channel as the page uses it. */
export interface PtyBridge {
  send(bytes: Uint8Array<ArrayBuffer>): void;
  sendText(text: string): void;
  close(): void;
}

/** Where the computer's terminal output goes. */
export type Write = (chunk: Uint8Array | string) => void;

/** The AI's computer, reached through `host`, and its device id `to`. */
export interface Target {
  host: Home;
  to: string;
}

/** The phone key row of `web/design/terminal.html`, in its order. */
export const phoneKeys: readonly PhoneKey[] = [
  { label: 'Esc', seq: '\x1b' },
  { label: 'Tab', seq: '\t' },
  { label: 'Ctrl', seq: '', ctrl: true },
  { label: '↑', seq: '\x1b[A', ariaKey: 'terminal.key.up' },
  { label: '↓', seq: '\x1b[B', ariaKey: 'terminal.key.down' },
  { label: '/', seq: '/' },
  { label: '@', seq: '@' },
  { label: '⏎', seq: '\r', ariaKey: 'terminal.key.enter' },
];

const wordedErrors = ['busy', 'cli', 'conflict', 'home_unreachable', 'not_found', 'lost'] as const;

/** The label of an AI's terminal channel (`datachannel.md` section 1). */
export const ptyLabel = (g: string, member: string): string => `pty:${g}:${member}`;

/** The connected node with the device id `host`. */
export const hostHomeOf = (homes: readonly Home[], host: string): Home | undefined =>
  homes.find((home) => home.device === host);

/** The computer to take an AI over on: known once the AI is read and its computer is online. */
export function targetOf(
  host: Home | undefined,
  ai: p.AIMemberRow | undefined,
): Target | undefined {
  if (!host?.ctl || !ai || ai.state === 'offline') return undefined;
  return { host, to: ai.host };
}

/** The task of the terminal page: the one its address names, else the AI's. */
export function sessionTaskOf(
  group: Group | undefined,
  ai: p.AIMemberRow | undefined,
  taskParam?: string,
): p.TaskRow | undefined {
  return group?.task(taskParam ?? ai?.task);
}

/** Whether a terminal can open on the task: not before it starts, nor after it ends
 *  (`flows/takeover.md` A). */
export const hasSession = (task: p.TaskRow) => ['active', 'paused', 'failed'].includes(task.state);

/** The task that the address of an AI's terminal names: the one the AI is on. Without one its
 *  computer chooses. */
export function terminalTask(group: Group, ai: p.AIMemberRow): string | undefined {
  const task = group.task(ai.task);
  return task && hasSession(task) ? task.id : undefined;
}

/** The line under the page's title: `computer · T-<num> · branch`. */
export function headerSubtitle(
  ai: p.AIMemberRow | undefined,
  task: p.TaskRow | undefined,
  groupName = '',
): string {
  if (!ai) return groupName;
  const parts = [ai.host_name];
  if (task) parts.push(`T-${task.num}`);
  if (task?.branch) parts.push(task.branch);
  return parts.join(' · ');
}

/** The link line of the session card. */
export function linkBadgeOf(
  online: boolean,
  path: Path | undefined,
): { st: 'ok' | 'limited' | 'linking' | 'down'; text: string } {
  if (!online) return { st: 'down', text: t('terminal.link.down') };
  if (!path) return { st: 'linking', text: t('terminal.link.linking') };
  if (path.kind === 'relay') return { st: 'limited', text: t('terminal.link.relay') };
  const base = t('terminal.link.direct');
  return { st: 'ok', text: path.rtt === undefined ? base : `${base} · ${path.rtt} ms` };
}

/** The bar's sentence while the owner drives: since when, and that messages wait. */
export function activeSummary(startedAt: number, nowMs: number): string {
  const mins = Math.floor((nowMs - startedAt) / 60_000);
  return mins > 0 ? t('terminal.bar.ago').replace('{n}', String(mins)) : t('terminal.bar.now');
}

/** The wording of a refusal of `takeover.start`, or undefined for a code `Failure` shows. */
export function takeoverErrorText(code: string): string | undefined {
  const match = wordedErrors.find((known) => known === code);
  return match ? t(`terminal.error.${match}`) : undefined;
}

const staticBars = {
  starting: {
    title: 'terminal.bar.starting',
    detail: 'terminal.bar.starting.body',
    handback: true,
  },
  relay: { title: 'terminal.bar.relay', detail: 'terminal.bar.relay.body', handback: false },
  relayed: {
    title: 'terminal.bar.relayed',
    detail: 'terminal.bar.relayed.body',
    handback: false,
  },
  ended: { title: 'terminal.bar.ended', detail: 'terminal.bar.ended.body', handback: false },
  offline: { title: 'terminal.bar.offline', detail: 'terminal.bar.offline.body', handback: false },
  connecting: {
    title: 'terminal.bar.connecting',
    detail: 'terminal.bar.connecting.body',
    handback: false,
  },
  missing: { title: 'terminal.bar.missing', detail: 'terminal.bar.missing.body', handback: false },
} as const;

function idleBar(ai: p.AIMemberRow | undefined, online: boolean, path: Path | undefined) {
  if (!ai) return staticBars.missing;
  if (!online || ai.state === 'offline') return staticBars.offline;
  if (!path) return staticBars.connecting;
  return path.kind === 'relay' ? staticBars.relay : staticBars.starting;
}

/** What the takeover bar says in a phase, with the AI and the browser's path to its computer. */
export function takeoverBarState(opts: {
  phase: TakeoverPhase;
  ai: p.AIMemberRow | undefined;
  online: boolean;
  path: Path | undefined;
  errorCode: string;
  startedAt: number;
  nowMs: number;
}): BarView {
  if (opts.phase === 'active') {
    const detail = activeSummary(opts.startedAt, opts.nowMs);
    return { title: t('terminal.bar.active'), detail, handback: true };
  }
  if (opts.phase === 'error') {
    const detail = takeoverErrorText(opts.errorCode);
    const errorCode = detail ? undefined : opts.errorCode;
    return { title: t('terminal.bar.error'), detail, errorCode, handback: false };
  }
  const bar =
    opts.phase === 'idle' ? idleBar(opts.ai, opts.online, opts.path) : staticBars[opts.phase];
  return { title: t(bar.title), detail: t(bar.detail), handback: bar.handback };
}

/** A sticky `Ctrl` applied to the first character typed. */
export function applyCtrl(input: string, ctrl: boolean): { out: string; ctrl: boolean } {
  if (!ctrl || input.length === 0) return { out: input, ctrl };
  const code = input.charCodeAt(0);
  const mapped =
    code >= 0x40 && code <= 0x7e ? String.fromCharCode(code & 0x1f) + input.slice(1) : input;
  return { out: mapped, ctrl: false };
}

/** An xterm binary string, one byte per code unit, as bytes. */
export function binaryBytes(data: string): Uint8Array<ArrayBuffer> {
  return Uint8Array.from({ length: data.length }, (_, i) => data.charCodeAt(i) & 0xff);
}

/** A message of the `pty` channel as xterm writes it, whole bytes so UTF-8 is not split. */
export function toTerminalChunk(data: unknown): Uint8Array | string | undefined {
  if (data instanceof Uint8Array) return data;
  if (data instanceof ArrayBuffer) return new Uint8Array(data);
  return typeof data === 'string' ? data : undefined;
}

/** The changed files of a `handback` card. */
export function handbackFilesLabel(count: number): string {
  if (count <= 0) return t('terminal.handback.files.none');
  if (count === 1) return t('terminal.handback.files.one');
  return t('terminal.handback.files').replace('{n}', String(count));
}

/** Signs and sends `takeover.start`; answers `""` on `ok`, else the refusal's code. */
export async function startTakeover(
  host: Home,
  to: string,
  g: string,
  member: string,
  task?: string,
): Promise<string> {
  const ctl = host.ctl;
  if (!ctl) return 'lost';
  const payload = { t: 'takeover.start', n: newId(), ts: Date.now(), to, g, member, task };
  const env = await host.me.sign(JSON.stringify(payload));
  const answer = await ctl.request({ t: 'cmd', env }).catch(lost);
  return answer.err ? answer.err.code : '';
}

/** Signs and sends `takeover.end`. */
export async function endTakeover(
  host: Home | undefined,
  to: string,
  g: string,
  member: string,
): Promise<void> {
  const ctl = host?.ctl;
  if (!ctl) return;
  const env = await host.me.sign(
    JSON.stringify({ t: 'takeover.end', n: newId(), ts: Date.now(), to, g, member }),
  );
  await ctl.request({ t: 'cmd', env }).catch(lost);
}

/** Pushes `pty.resize` on `ctl` for a size of at least one column and row (`datachannel.md` 7). */
export function sendResize(
  ctl: p.Ctl | undefined,
  g: string,
  ai: string,
  cols: number,
  rows: number,
): void {
  if (!ctl?.send || cols < 1 || rows < 1) return;
  ctl.send({ t: 'pty.resize', g, ai, cols: Math.floor(cols), rows: Math.floor(rows) });
}

/** Reads and writes the `pty` channel; `onClose` runs when it closes by any hand but this one. */
export function bridgePty(channel: p.PtyChannel, onChunk: Write, onClose: () => void): PtyBridge {
  let closed = false;
  const send = (bytes: Uint8Array<ArrayBuffer>) => {
    if (!closed && channel.readyState === 'open') channel.send(bytes);
  };
  channel.binaryType = 'arraybuffer';
  channel.onmessage = ({ data }: { data: unknown }) => {
    const chunk = toTerminalChunk(data);
    if (chunk !== undefined) onChunk(chunk);
  };
  channel.onclose = () => {
    if (closed) return;
    closed = true;
    onClose();
  };
  return {
    send,
    sendText(text) {
      if (text.length > 0) send(new TextEncoder().encode(text) as Uint8Array<ArrayBuffer>);
    },
    close() {
      if (closed) return;
      closed = true;
      channel.onmessage = null;
      channel.onclose = null;
      channel.close();
    },
  };
}

/** The takeover of one terminal address (`flows/takeover.md` B and D). It starts once the AI's
 *  computer is known and the browser's path to it is direct, and refuses a relayed path. It ends
 *  on hand-back, when the computer closes the channel, or when the path becomes relayed (A7.9).
 *  A hand-back sent while `takeover.start` is unanswered could reach the computer before the
 *  terminal opens and end nothing, so `takeover.end` then follows the answer (A7.6). */
export class Takeover {
  phase = $state<TakeoverPhase>('idle');
  errorCode = $state('');
  startedAt = $state(0);
  ctrl = $state(false);
  #run = Symbol();
  #open: Target | undefined;
  #pty: PtyBridge | undefined;
  #cols = 0;
  #rows = 0;

  constructor(
    readonly g: string,
    readonly member: string,
    readonly task: string | undefined,
    readonly write: Write,
  ) {}

  /** Follows the computer and the browser's path to it. */
  follow(target: Target | undefined, path: Path | undefined) {
    if (!target || !path) return;
    if (path.kind === 'relay') this.#relayed();
    else if (this.phase === 'idle') void this.#start(target);
  }

  /** Ends the takeover. */
  handBack() {
    this.#run = Symbol();
    if (this.phase === 'starting' || this.phase === 'active') this.#close('ended');
  }

  /** The terminal's size in characters, sent while the terminal is open and when it opens. */
  resize(cols: number, rows: number) {
    this.#cols = cols;
    this.#rows = rows;
    this.#sendSize();
  }

  /** What the person types; a sticky `Ctrl` applies to its first character. */
  type(text: string) {
    const next = applyCtrl(text, this.ctrl);
    this.ctrl = next.ctrl;
    this.#pty?.sendText(next.out);
  }

  /** What xterm reads as binary, such as some mouse reports. */
  typeBinary(raw: string) {
    this.#pty?.send(binaryBytes(raw));
  }

  /** A key of the phone key row. */
  press(key: PhoneKey) {
    if (key.ctrl) this.ctrl = !this.ctrl;
    else this.type(key.seq);
  }

  async #start(target: Target) {
    const run = (this.#run = Symbol());
    this.phase = 'starting';
    const code = await startTakeover(target.host, target.to, this.g, this.member, this.task);
    if (run !== this.#run) {
      if (code === '') void endTakeover(target.host, target.to, this.g, this.member);
      return;
    }
    if (code === '') this.#opened(target);
    else this.#refused(code);
  }

  #opened(target: Target) {
    const ctl = target.host.ctl;
    const channel = ctl?.channel ? ctl.channel(ptyLabel(this.g, this.member)) : undefined;
    if (!channel) return this.#refused('lost');
    this.#open = target;
    this.#pty = bridgePty(channel, this.write, () => this.#closed());
    this.startedAt = Date.now();
    this.phase = 'active';
    this.#sendSize();
  }

  #refused(code: string) {
    this.errorCode = code;
    this.phase = code === 'relay' ? 'relay' : 'error';
  }

  #relayed() {
    if (this.phase === 'relay') this.phase = 'idle';
    if (this.phase !== 'active') return;
    // The computer closes the channel on a relayed path itself: that is its hand-back.
    this.#open = undefined;
    this.#close('relayed');
  }

  #close(phase: TakeoverPhase) {
    const open = this.#open;
    this.#closed();
    this.phase = phase;
    if (open) void endTakeover(open.host, open.to, this.g, this.member);
  }

  #closed() {
    this.#pty?.close();
    this.#pty = undefined;
    this.#open = undefined;
    this.phase = 'ended';
  }

  #sendSize() {
    const open = this.#open;
    if (open) sendResize(open.host.ctl, this.g, this.member, this.#cols, this.#rows);
  }
}
