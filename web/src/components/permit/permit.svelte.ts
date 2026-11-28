import type { CmdResult, ErrorResponse, HostInfo, HostPermitRequest } from '../../lib/proto';

export type Permissions = NonNullable<HostInfo['permissions']>;
export type RowId = 'local_network' | 'incoming' | 'full_disk';
export type LanState = 'ask' | 'settings';
export type AskGroup = 'mac' | 'win' | 'both' | 'none';
export type PermitAsk = (
  body?: Omit<HostPermitRequest, 't' | 'id'>,
) => Promise<CmdResult | ErrorResponse>;

export interface Opened {
  local_network?: LanState;
  incoming?: boolean;
  full_disk?: boolean;
}

export interface RowAction {
  label: 'permit.lan.ask' | 'permit.in.open' | 'permit.files.all';
  primary: boolean;
}

export interface CoachButton {
  label: 'permit.lan.nowindow' | 'permit.again' | 'permit.hide';
  variant: 'link' | 'ghost';
  action: 'nowindow' | 'again' | 'hide';
}

export interface PermitCoach {
  sketch?: 'alert' | 'drag';
  line: 'permit.lan.coach' | 'permit.lan.settings' | 'permit.in.settings' | 'permit.files.coach';
  more?: 'permit.files.plus';
  buttons: readonly CoachButton[];
}

export interface PermitRow {
  id: RowId;
  title: 'permit.lan' | 'permit.in' | 'permit.files';
  ok: boolean;
  mark: 'ok' | 'need' | 'free';
  line:
    | 'permit.lan.ok'
    | 'permit.lan.todo'
    | 'permit.in.ok'
    | 'permit.in.todo'
    | 'permit.files.ask'
    | 'permit.files.full';
  hint?: 'permit.files.hint';
  action?: RowAction;
  coach?: PermitCoach;
}

export interface PlatformSource {
  userAgentData?: { platform?: string };
  platform?: string;
  userAgent?: string;
}

export type GateView =
  | { state: 'idle' }
  | { state: 'waiting' }
  | { state: 'asking' }
  | { state: 'refused'; settings: boolean };

export type GateOutcome =
  | { kind: 'ok' }
  | { kind: 'permission' }
  | { kind: 'error'; err: ErrorResponse['err'] }
  | { kind: 'cancelled' };

export const GATE_ASK_DELAY_MS = 500;

const BTN_NOWINDOW: CoachButton = {
  label: 'permit.lan.nowindow',
  variant: 'link',
  action: 'nowindow',
};
const BTN_AGAIN: CoachButton = { label: 'permit.again', variant: 'ghost', action: 'again' };
const BTN_HIDE: CoachButton = { label: 'permit.hide', variant: 'ghost', action: 'hide' };

interface ToggleSpec {
  id: 'incoming' | 'full_disk';
  title: 'permit.in' | 'permit.files';
  okLine: 'permit.in.ok' | 'permit.files.full';
  todoMark: 'need' | 'free';
  todoLine: 'permit.in.todo' | 'permit.files.ask';
  hint?: 'permit.files.hint';
  actionLabel: 'permit.in.open' | 'permit.files.all';
  coach: PermitCoach;
}

const INCOMING_SPEC: ToggleSpec = {
  id: 'incoming',
  title: 'permit.in',
  okLine: 'permit.in.ok',
  todoMark: 'need',
  todoLine: 'permit.in.todo',
  actionLabel: 'permit.in.open',
  coach: {
    line: 'permit.in.settings',
    buttons: [BTN_AGAIN],
  },
};

const FULL_DISK_SPEC: ToggleSpec = {
  id: 'full_disk',
  title: 'permit.files',
  okLine: 'permit.files.full',
  todoMark: 'free',
  todoLine: 'permit.files.ask',
  hint: 'permit.files.hint',
  actionLabel: 'permit.files.all',
  coach: {
    sketch: 'drag',
    line: 'permit.files.coach',
    more: 'permit.files.plus',
    buttons: [BTN_AGAIN, BTN_HIDE],
  },
};

function lanRow(ok: boolean, opened?: LanState): PermitRow {
  if (ok) {
    return {
      id: 'local_network',
      title: 'permit.lan',
      ok: true,
      mark: 'ok',
      line: 'permit.lan.ok',
    };
  }
  const coach: PermitCoach | undefined =
    opened === 'ask'
      ? { sketch: 'alert', line: 'permit.lan.coach', buttons: [BTN_NOWINDOW, BTN_AGAIN] }
      : opened === 'settings'
        ? { line: 'permit.lan.settings', buttons: [BTN_AGAIN] }
        : undefined;
  return {
    id: 'local_network',
    title: 'permit.lan',
    ok: false,
    mark: 'need',
    line: 'permit.lan.todo',
    action: coach ? undefined : { label: 'permit.lan.ask', primary: true },
    coach,
  };
}

function toggleRow(ok: boolean, open: boolean, spec: ToggleSpec): PermitRow {
  if (ok) return { id: spec.id, title: spec.title, ok: true, mark: 'ok', line: spec.okLine };
  return {
    id: spec.id,
    title: spec.title,
    ok: false,
    mark: spec.todoMark,
    line: spec.todoLine,
    hint: spec.hint,
    action: open ? undefined : { label: spec.actionLabel, primary: false },
    coach: open ? spec.coach : undefined,
  };
}

/** The three permission rows and their action or open coach (design §2.3). */
export function rows(permissions: Permissions, opened: Opened = {}): PermitRow[] {
  return [
    lanRow(permissions.local_network, opened.local_network),
    toggleRow(permissions.incoming, Boolean(opened.incoming), INCOMING_SPEC),
    toggleRow(permissions.full_disk, Boolean(opened.full_disk), FULL_DISK_SPEC),
  ];
}

/** Whether the card badge waits on the owner (`local_network` or `incoming` is false; Files is optional). */
export function needs(permissions: Permissions): boolean {
  return !permissions.local_network || !permissions.incoming;
}

/** Picks the best available platform string from `navigator`-shaped metadata. */
export function browserPlatform(nav: PlatformSource): string {
  if (nav.userAgentData?.platform) return nav.userAgentData.platform;
  if (nav.userAgent && /iphone|ipad|ipod|android|mobile/i.test(nav.userAgent)) {
    return nav.userAgent;
  }
  return nav.platform || nav.userAgent || '';
}

/** Which computer groups the onboarding pre-pairing note displays for `platform`. */
export function asks(platform: string): AskGroup {
  const value = platform.toLowerCase();
  if (/iphone|ipad|ipod|android|cros|mobile/.test(value)) return 'both';
  if (/\bwin/.test(value)) return 'win';
  if (/\bmac|darwin/.test(value)) return 'mac';
  if (/\blinux/.test(value)) return 'none';
  return 'both';
}

/** State machine for the folder dialog's `host.permit` permission gate (design §2.5). */
export class FolderGateState {
  value = $state<GateView>({ state: 'idle' });
  #timer: ReturnType<typeof setTimeout> | undefined;
  #gen: object = {};

  get busy(): boolean {
    return this.value.state === 'waiting' || this.value.state === 'asking';
  }

  /** Cancels any pending check and resets the gate back to `idle`. */
  reset(): void {
    this.#gen = {};
    this.#clearTimer();
    this.value = { state: 'idle' };
  }

  /** Switches an open refusal prompt into the Files and Folders guidance text. */
  openedSettings(): void {
    if (this.value.state === 'refused') this.value = { state: 'refused', settings: true };
  }

  /** Sends `{ path }` through `ask`, showing `asking` when macOS takes over 500 ms. */
  async check(ask: PermitAsk, path: string): Promise<GateOutcome> {
    const token = {};
    this.#gen = token;
    this.#clearTimer();
    this.value = { state: 'waiting' };
    this.#timer = setTimeout(() => {
      this.value = { state: 'asking' };
    }, GATE_ASK_DELAY_MS);
    const res = await ask({ path });
    if (this.#gen !== token) return { kind: 'cancelled' };
    this.#clearTimer();
    return this.#settle(res);
  }

  #settle(res: CmdResult | ErrorResponse): GateOutcome {
    if (res.err) {
      if (res.err.code === 'permission') {
        this.value = { state: 'refused', settings: false };
        return { kind: 'permission' };
      }
      this.value = { state: 'idle' };
      return { kind: 'error', err: res.err };
    }
    this.value = { state: 'idle' };
    return { kind: 'ok' };
  }

  #clearTimer(): void {
    clearTimeout(this.#timer);
    this.#timer = undefined;
  }
}

/** Creates a folder gate state machine instance. */
export function gate(): FolderGateState {
  return new FolderGateState();
}
