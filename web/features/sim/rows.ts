// The rows the demo world writes, each made from the node's wire example of its kind with the
// fields of the moment, so that every row has the shape the client reads (contracts/rows.md,
// contracts/messages.md). A row the home writes for itself has no signed envelope.
import { newId } from '../../src/lib/groups.svelte';
import { goldens } from '../../src/lib/mock';
import type * as p from '../../src/lib/proto';
import { example } from '../home';

export type Fields = Record<string, unknown>;
/** A row as the scripts handle it: any of contracts/rows.md, read field by field. */
export type Row = Fields & { r: string; id: string };

/** A wire example with some fields changed, without the example's envelope. */
export function row(name: string, changes: Fields = {}): Row {
  const { env: _env, ...made } = example(name, changes);
  return made as unknown as Row;
}

/** What a new message says: its author, text and where it stands. */
export interface Say {
  g: string;
  by: string;
  text?: string;
  thread?: string | null;
  main?: boolean;
  mentions?: string[];
  re?: string | null;
  id?: string;
  ts?: number;
}

/** The fields that are given, without those set to undefined, so that defaults stay. */
const given = <T extends object>(fields: T) =>
  Object.fromEntries(Object.entries(fields).filter(([, value]) => value !== undefined)) as T;

/** A new message of a kind, from the example `row-msg-<kind>`, or of a card `row-msg-<kind>-pending`. */
export function msg(kind: string, say: Say, body: Fields = {}, state: Fields | null = null) {
  const plain = `row-msg-${kind}`;
  const defaults = { id: newId(), text: '', thread: null, main: false, mentions: [], re: null };
  const fields = { ...defaults, ts: Date.now(), ...given(say), body, state };
  return row(plain in goldens ? plain : `${plain}-pending`, fields);
}

/** A person of a group. */
export const person = (g: string, id: string, user: string, name: string, role: string) =>
  row('row-member-human', { g, id, user, name, role, online: true, last_seen: Date.now() });

/** What an AI member is: where it runs and for whom. */
export interface AISpec {
  id: string;
  name: string;
  cli: string;
  user: string;
  host: string;
  hostName: string;
  /** Its folder on `host`. */
  workspace: string;
  secretary?: boolean;
  account?: string;
  model?: string;
  effort?: string;
  tier?: string;
  skills?: string;
}

const profile = {
  secretary: false,
  account: 'personal',
  model: '',
  effort: '',
  tier: 'standard',
  skills: '',
};

/** An AI of a group, idle, with its profile. */
export function ai(g: string, spec: AISpec, state = 'waiting'): Row {
  const { hostName, ...rest } = { ...profile, ...given(spec) };
  const now = Date.now();
  const usage = { window_pct: 18, turns_today: 6 };
  const live = { state, task: undefined, since: now, last_seen: now, path: 'direct', usage };
  return row('row-member-ai', {
    g,
    ...rest,
    host_name: hostName,
    allow: [],
    approvers: [spec.user],
    ...live,
  });
}

/** What a new group is. */
export interface GroupSpec {
  g: string;
  name: string;
  home: string;
  secretary?: string;
  created?: number;
}

/** A group's own row: one kind of group, with no repository and no check. */
export function group(spec: GroupSpec): Row {
  const { g, ...rest } = { created: Date.now(), ...given(spec) };
  return row('row-group', { ...rest, id: g, secretary: spec.secretary, paused: null });
}

/** What a new task is: its number, title, roles and the brief it came from. */
export interface TaskSpec {
  g: string;
  id?: string;
  num: number;
  title: string;
  assignee: string;
  reviewer?: string;
  brief?: string;
  depends?: string[];
  pass?: number;
  origin?: string;
}

/** What a task has before any step: no report, no review. */
const fresh = {
  plan: undefined,
  step: 'work',
  report: undefined,
  asked: undefined,
  foundation: false,
  review: undefined,
  failures: 0,
  stuck: undefined,
  thread: { count: 0, rounds: 0 },
};

/** A task that has just been created, at step `work`: active, or queued for what `waits` says, a
 *  task it depends on or its folder (contracts/task-machine.md section 2). */
export function task(spec: TaskSpec, waits?: 'depends' | 'busy'): Row {
  const now = Date.now();
  const defaults = { id: newId(), origin: 'brief', pass: 90, depends: [] };
  const roles = { brief: spec.brief, reviewer: spec.reviewer };
  return row('row-task', {
    ...fresh,
    ...defaults,
    ...given(spec),
    ...roles,
    state: waits ? 'queued' : 'active',
    wait: waits ? { kind: waits, since: now } : null,
    stats: { started: now, ended: null },
    created: now,
  });
}

/** A system message of a code with its fields. */
export const system = (g: string, body: Fields, thread: string | null = null) =>
  msg('system', { g, by: 'system', thread }, body);
