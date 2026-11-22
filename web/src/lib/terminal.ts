// Terminal takeover commands, byte channel bridge and display projections
// (planning/flows/takeover.md, planning/contracts/datachannel.md sections 1 and 7,
// planning/contracts/commands.md section 2).
import { lost, newId, type Group, type Home } from './groups.svelte';
import type { Path } from './net.svelte';
import { t } from './prefs.svelte';
import type * as p from './proto';

/** A key in the narrow-screen terminal toolbar (`web/design/terminal.html`). */
export interface PhoneKey {
  label: string;
  seq?: string;
  ctrl?: true;
  ariaKey?: 'terminal.key.up' | 'terminal.key.down' | 'terminal.key.enter';
}

/** Lifecycle state of a terminal takeover view. */
export type TakeoverPhase =
  'idle' | 'starting' | 'active' | 'relay' | 'relayed' | 'ended' | 'error';

/** Content rendered in the top `.takeover` status bar. */
export interface BarView {
  title: string;
  detail?: string;
  errorCode?: string;
  handback: boolean;
}

/** Active bridge over a `pty:<g>:<member>` WebRTC data channel. */
export interface PtyBridge {
  send(bytes: Uint8Array<ArrayBuffer>): void;
  sendText(text: string): void;
  close(): void;
}

/** Keys shown in the phone toolbar, matching `web/design/terminal.html` in exact order. */
export const phoneKeys: readonly PhoneKey[] = [
  { label: 'Esc', seq: '\x1b' },
  { label: 'Tab', seq: '\t' },
  { label: 'Ctrl', ctrl: true },
  { label: '↑', seq: '\x1b[A', ariaKey: 'terminal.key.up' },
  { label: '↓', seq: '\x1b[B', ariaKey: 'terminal.key.down' },
  { label: '/', seq: '/' },
  { label: '@', seq: '@' },
  { label: '⏎', seq: '\r', ariaKey: 'terminal.key.enter' },
];

const wordedErrors = ['busy', 'cli', 'conflict', 'home_unreachable', 'not_found', 'lost'] as const;

/** Data-channel label for an AI member's interactive terminal (`datachannel.md` section 1). */
export const ptyLabel = (g: string, member: string): string => `pty:${g}:${member}`;

/** Finds the connected or paired `Home` for an AI's `host` device id. */
export function hostHomeOf(
  allHomes: readonly Home[],
  group: Group | undefined,
  host: string,
): Home | undefined {
  const found = allHomes.find((h) => h.device === host || h.hostInfo?.device === host);
  if (found) return found;
  return group?.info?.home === host ? group.home : undefined;
}

/** Resolves the task row shown on the terminal page from the route parameter or the AI's row. */
export function sessionTaskOf(
  group: Group | undefined,
  ai: p.AIMemberRow | undefined,
  taskParam?: string,
): p.TaskRow | undefined {
  return group?.task(taskParam ?? ai?.task);
}

/** Reads the held inbox item count for `(g, member)` from the host's latest `host.info`. */
export function queuedCountOf(info: p.HostInfoPush | undefined, g: string, member: string): number {
  for (const agent of info?.agents ?? []) {
    const match = agent.groups.find((entry) => entry.g === g && entry.member === member);
    if (match) return match.queued;
  }
  return 0;
}

/** Builds the top-bar subtitle `host · T-num · branch` for the terminal header. */
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

/** Formats the link status badge inside the session details card. */
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

function elapsedText(startedAt: number, nowMs: number): string {
  const mins = Math.floor((nowMs - startedAt) / 60_000);
  return mins > 0 ? t('terminal.bar.ago').replace('{n}', String(mins)) : t('terminal.bar.now');
}

/** Formats the active takeover bar sentence with elapsed minutes and held message count. */
export function activeSummary(startedAt: number, nowMs: number, queued: number): string {
  const since = elapsedText(startedAt, nowMs);
  if (queued <= 0) return since;
  const held =
    queued === 1
      ? t('terminal.bar.queued.one')
      : t('terminal.bar.queued.many').replace('{n}', String(queued));
  return `${since} ${held}`;
}

/** Words `takeover.start` refusal codes, returning `undefined` for generic `Failure` codes. */
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
} as const;

function accessBarState(ai: p.AIMemberRow | undefined, owned: boolean): BarView | undefined {
  if (!ai) {
    return {
      title: t('terminal.bar.missing'),
      detail: t('terminal.bar.missing.body'),
      handback: false,
    };
  }
  if (!owned) {
    return {
      title: t('terminal.bar.forbidden'),
      detail: t('terminal.bar.forbidden.body'),
      handback: false,
    };
  }
  return undefined;
}

function idleBarState(
  ai: p.AIMemberRow | undefined,
  owned: boolean,
  online: boolean,
  path: Path | undefined,
): BarView {
  const denied = accessBarState(ai, owned);
  if (denied) return denied;
  if (!online || ai!.state === 'offline') {
    return {
      title: t('terminal.bar.offline'),
      detail: t('terminal.bar.offline.body'),
      handback: false,
    };
  }
  if (!path) {
    return {
      title: t('terminal.bar.connecting'),
      detail: t('terminal.bar.connecting.body'),
      handback: false,
    };
  }
  const preset = path.kind === 'relay' ? staticBars.relay : staticBars.starting;
  return { title: t(preset.title), detail: t(preset.detail), handback: preset.handback };
}

/** Derives the `.takeover` status bar content for the current takeover phase and link state. */
export function takeoverBarState(opts: {
  phase: TakeoverPhase;
  ai: p.AIMemberRow | undefined;
  owned: boolean;
  online: boolean;
  path: Path | undefined;
  errorCode: string;
  startedAt: number;
  nowMs: number;
  queued: number;
}): BarView {
  if (opts.phase === 'idle') return idleBarState(opts.ai, opts.owned, opts.online, opts.path);
  if (opts.phase === 'active') {
    return {
      title: t('terminal.bar.active'),
      detail: activeSummary(opts.startedAt, opts.nowMs, opts.queued),
      handback: true,
    };
  }
  if (opts.phase === 'error') {
    const detail = takeoverErrorText(opts.errorCode);
    return detail
      ? { title: t('terminal.bar.error'), detail, handback: false }
      : { title: t('terminal.bar.error'), errorCode: opts.errorCode, handback: false };
  }
  const preset = staticBars[opts.phase];
  return { title: t(preset.title), detail: t(preset.detail), handback: preset.handback };
}

/** Applies a sticky `Ctrl` modifier to the first character of `input` when active. */
export function applyCtrl(input: string, ctrl: boolean): { out: string; ctrl: boolean } {
  if (!ctrl || input.length === 0) return { out: input, ctrl };
  const code = input.charCodeAt(0);
  const mapped =
    code >= 0x40 && code <= 0x7e ? String.fromCharCode(code & 0x1f) + input.slice(1) : input;
  return { out: mapped, ctrl: false };
}

/** Converts an xterm binary string (each code unit one byte) into a `Uint8Array`. */
export function binaryBytes(data: string): Uint8Array<ArrayBuffer> {
  const out = new Uint8Array(data.length);
  for (let i = 0; i < data.length; i += 1) out[i] = data.charCodeAt(i) & 0xff;
  return out;
}

/** Normalizes a `PtyChannel` message payload for `xterm.write` without splitting UTF-8 sequences. */
export function toTerminalChunk(data: unknown): Uint8Array | string | undefined {
  if (data instanceof Uint8Array) return data;
  if (data instanceof ArrayBuffer) return new Uint8Array(data);
  return typeof data === 'string' ? data : undefined;
}

/** Formats the changed-file count on a `handback` message card. */
export function handbackFilesLabel(count: number): string {
  if (count <= 0) return t('terminal.handback.files.none');
  if (count === 1) return t('terminal.handback.files.one');
  return t('terminal.handback.files').replace('{n}', String(count));
}

/** Signs and sends `takeover.start`, returning `""` on `ok` or the refusal code. */
export async function startTakeover(
  host: Home | undefined,
  to: string,
  g: string,
  member: string,
  task?: string,
): Promise<string> {
  const ctl = host?.ctl;
  if (!ctl) return 'lost';
  const payload = {
    t: 'takeover.start',
    n: newId(),
    ts: Date.now(),
    to,
    g,
    member,
    ...(task ? { task } : {}),
  };
  const env = await host.me.sign(JSON.stringify(payload));
  const answer = await ctl.request({ t: 'cmd', env }).catch(lost);
  return answer.err ? answer.err.code : '';
}

/** Signs and sends `takeover.end` on hand-back or page unload. */
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

/** Pushes `pty.resize` on `ctl` when dimensions are positive (`datachannel.md` section 7). */
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

export interface PtyHandlers {
  onOpen?: () => void;
  onChunk: (chunk: Uint8Array | string) => void;
  onClose: () => void;
}

/** Wires a `PtyChannel` to xterm input/output callbacks and clean local teardown. */
export function bridgePty(channel: p.PtyChannel, handlers: PtyHandlers): PtyBridge {
  let closed = false;
  const send = (bytes: Uint8Array<ArrayBuffer>) => {
    if (!closed && channel.readyState === 'open') channel.send(bytes);
  };
  channel.binaryType = 'arraybuffer';
  channel.onopen = () => handlers.onOpen?.();
  channel.onmessage = ({ data }: { data: unknown }) => {
    const chunk = toTerminalChunk(data);
    if (chunk !== undefined) handlers.onChunk(chunk);
  };
  channel.onclose = () => {
    if (closed) return;
    closed = true;
    handlers.onClose();
  };
  if (channel.readyState === 'open') handlers.onOpen?.();
  return {
    send,
    sendText(text) {
      if (text.length > 0) send(new TextEncoder().encode(text) as Uint8Array<ArrayBuffer>);
    },
    close() {
      if (closed) return;
      closed = true;
      channel.onopen = null;
      channel.onmessage = null;
      channel.onclose = null;
      channel.close();
    },
  };
}
