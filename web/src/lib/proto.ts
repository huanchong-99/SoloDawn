// Wire types, mirroring internal/proto. Each message is declared once, as a valibot schema; its
// TypeScript type is inferred from that schema under the same name. An unknown field is dropped,
// as Go's encoding/json does, so a node that adds a field does not break an older client; a missing
// field is an error. A Go field tagged omitempty is optional here; a Go pointer without omitempty
// is required and may be null. json.RawMessage fields are any JSON until a contract types them.
import * as v from 'valibot';

const str = v.string();
const num = v.number();
const bool = v.boolean();
const strs = v.array(str);

// The signed payload `p` is kept as the exact string received: its bytes are what is signed.
export const Envelope = v.object({ p: str, k: str, s: str });
export type Envelope = v.InferOutput<typeof Envelope>;

const PayloadHeader = { n: str, ts: num, g: v.optional(str), to: v.optional(str) };
const SignalHeader = { ...PayloadHeader, conn: str };

export const Offer = v.object({
  t: v.literal('offer'),
  ...SignalHeader,
  sdp: str,
  probe: bool,
});
export type Offer = v.InferOutput<typeof Offer>;

export const Answer = v.object({ t: v.literal('answer'), ...SignalHeader, sdp: str });
export type Answer = v.InferOutput<typeof Answer>;

export const ICECandidate = v.object({
  t: v.literal('ice'),
  ...SignalHeader,
  cand: str,
  mid: str,
});
export type ICECandidate = v.InferOutput<typeof ICECandidate>;

export const Pair = v.object({
  t: v.literal('pair'),
  k: str,
  kind: str,
  name: str,
  role: str,
  mac: str,
});
export type Pair = v.InferOutput<typeof Pair>;

const DeviceChange = {
  user: str,
  ver: num,
  op: str,
  dev: str,
  kind: v.optional(str),
  name: v.optional(str),
};

/** An entry of a person's device log: the payload of one of its envelopes. */
export const DeviceEntry = v.object({
  t: v.literal('device'),
  ...PayloadHeader,
  ...DeviceChange,
});
export type DeviceEntry = v.InferOutput<typeof DeviceEntry>;

// Session (datachannel.md section 3)

const Success = { re: num, ok: v.literal(true) };

export const Hello = v.object({
  t: v.literal('hello'),
  id: num,
  v: num,
  kind: str,
  name: str,
  addrs: strs,
});
export type Hello = v.InferOutput<typeof Hello>;

const GroupSummary = v.object({ g: str, name: str, seq: num });

export const Welcome = v.object({
  t: v.literal('welcome'),
  ...Success,
  v: num,
  device: str,
  name: str,
  version: str,
  time: num,
  groups: v.array(GroupSummary),
  owner: bool,
});
export type Welcome = v.InferOutput<typeof Welcome>;

/** The whole list of readable groups again, whenever it changes. No Go type or example yet (B-10). */
export const Groups = v.object({ t: v.literal('groups'), groups: v.array(GroupSummary) });
export type Groups = v.InferOutput<typeof Groups>;

export const Devlog = v.object({
  t: v.literal('devlog'),
  user: str,
  entries: v.array(Envelope),
});
export type Devlog = v.InferOutput<typeof Devlog>;

// Rows (rows.md)

const RowHeader = { id: str, seq: num };

export const GoneRow = v.object({ r: str, ...RowHeader, gone: v.literal(true) });
export type GoneRow = v.InferOutput<typeof GoneRow>;

export const GroupRow = v.object({
  r: v.literal('group'),
  ...RowHeader,
  name: str,
  kind: str,
  repo: v.optional(str),
  base: str,
  forge: str,
  home: str,
  secretary: v.optional(str),
  checks: strs,
  pass: num,
  pass_foundation: num,
  limits: v.object({
    rounds: num,
    wakes_per_hour: num,
    turn_minutes: num,
    failures: num,
    review_rounds: num,
    ask_depth: num,
    ask_wait_s: num,
    approval_wait_s: num,
    idle_notice_minutes: num,
  }),
  paused: v.nullable(v.object({ by: str, at: num })),
  created: num,
});
export type GroupRow = v.InferOutput<typeof GroupRow>;

const MemberRow = {
  r: v.literal('member'),
  ...RowHeader,
  g: str,
  user: str,
  name: str,
  last_seen: num,
};

export const HumanMemberRow = v.object({
  ...MemberRow,
  kind: v.literal('human'),
  role: str,
  online: bool,
});
export type HumanMemberRow = v.InferOutput<typeof HumanMemberRow>;

export const AIMemberRow = v.object({
  ...MemberRow,
  kind: v.literal('ai'),
  cli: str,
  host: str,
  host_name: str,
  secretary: bool,
  account: str,
  allow: strs,
  approvers: strs,
  state: str,
  task: v.optional(str),
  since: num,
  path: str,
  usage: v.object({
    window_pct: v.optional(num),
    window_resets: v.optional(num),
    day_cap_pct: v.optional(num),
    spend_today: v.optional(num),
    spend_cap: v.optional(num),
    turns_today: v.optional(num),
  }),
});
export type AIMemberRow = v.InferOutput<typeof AIMemberRow>;

export const DeviceRow = v.object({
  r: v.literal('device'),
  ...RowHeader,
  ...DeviceChange,
  env: Envelope,
});
export type DeviceRow = v.InferOutput<typeof DeviceRow>;

const Check = v.object({ ok: bool, at: num, commit: str });

export const TaskRow = v.object({
  r: v.literal('task'),
  ...RowHeader,
  g: str,
  num: num,
  title: str,
  origin: str,
  brief: v.optional(str),
  state: str,
  step: str,
  commit: v.optional(str),
  assignee: str,
  reviewer: v.optional(str),
  foundation: bool,
  pass: num,
  depends: strs,
  branch: v.optional(str),
  pr: v.optional(num),
  pr_url: v.optional(str),
  checks: v.object({ local: v.optional(Check), cloud: v.optional(Check) }),
  review: v.optional(v.object({ round: num, score: num, ok: bool, by: str, commit: str, at: num })),
  wait: v.nullable(
    v.object({ kind: str, msg: v.optional(str), who: v.optional(strs), since: num }),
  ),
  failures: num,
  thread: v.object({
    count: num,
    rounds: num,
    last: v.optional(v.object({ by: str, kind: str, text: str, ts: num })),
  }),
  stats: v.object({ added: num, removed: num, started: num, ended: v.nullable(num) }),
  created: num,
});
export type TaskRow = v.InferOutput<typeof TaskRow>;

const int = v.pipe(num, v.safeInteger());
const nonEmptyStrings = v.pipe(strs, v.nonEmpty());
const optionalText = v.optional(str);
const optionalInt = v.optional(int);
// valibot's object({}) also accepts arrays; the body on the wire must be an object.
const EmptyBody = v.pipe(
  v.unknown(),
  v.check((x) => !Array.isArray(x)),
  v.object({}),
);

const BriefTask = v.object({
  title: str,
  assignee: str,
  reviewer: str,
  foundation: bool,
  depends: v.array(int),
});

const BriefBody = v.object({
  title: str,
  goal: str,
  criteria: nonEmptyStrings,
  scope: v.object({ in: strs, out: strs }),
  tasks: v.pipe(
    v.array(BriefTask),
    v.nonEmpty(),
    v.checkItems((task, i) => task.depends.every((d) => d >= 0 && d < i)),
  ),
  pass: int,
  requester: str,
  replaces: v.nullable(str),
});

const ApprovalBody = v.object({
  task: str,
  ai: str,
  tool: str,
  command: str,
  why: optionalText,
  category: v.picklist(['command', 'file', 'network', 'other']),
  approvers: nonEmptyStrings,
});

const DecisionBody = v.object({
  task: optionalText,
  question: str,
  context: optionalText,
  options: v.array(v.object({ id: str, label: str, effect: str })),
  recommended: optionalText,
  to: nonEmptyStrings,
});

const SystemBody = v.variant('code', [
  v.object({ code: v.literal('rounds_limit'), task: str, n: int }),
  v.object({
    code: v.literal('turn_interrupted'),
    ai: str,
    reason: v.picklist(['crash', 'timeout', 'stop']),
    task: optionalText,
    detail: optionalText,
  }),
  v.object({ code: v.picklist(['group_paused', 'group_resumed']), by: str }),
  v.object({ code: v.literal('preempted'), task: str, by: str }),
  v.object({
    code: v.literal('cap_reached'),
    ai: str,
    cap: v.picklist(['day', 'task']),
    task: optionalText,
  }),
  v.object({ code: v.literal('rate_limited'), ai: str, resets: int }),
  v.object({ code: v.literal('login_invalid'), ai: str, detail: optionalText }),
  v.object({
    code: v.literal('task_failed'),
    task: str,
    reason: v.picklist(['failures', 'review_rounds']),
  }),
  v.object({ code: v.literal('task_merged'), task: str, commit: str, merge_commit: str }),
  v.object({ code: v.literal('no_secretary') }),
  v.object({ code: v.literal('session_replaced'), ai: str, task: optionalText }),
]);

const Pending = v.object({ status: v.literal('pending') });
const acted = { by: str, at: int, act: Envelope };
const BriefAction = v.object({ ...acted, note: optionalText });
const BriefState = v.variant('status', [
  Pending,
  v.object({ status: v.literal('confirmed'), ...acted }),
  v.object({ status: v.literal('revise'), ...BriefAction.entries }),
  v.pipe(
    v.object({ status: v.literal('superseded'), ...v.partial(BriefAction).entries }),
    v.check((s) => Object.keys(s).length === 1 || v.is(BriefAction, s)),
  ),
]);
const ApprovalState = v.variant('status', [
  Pending,
  v.object({ status: v.picklist(['approved', 'denied']), ...acted }),
  v.object({ status: v.picklist(['expired', 'reassigned']), at: int }),
]);
const DecisionState = v.variant('status', [
  Pending,
  v.pipe(
    v.object({ status: v.literal('decided'), ...acted, choice: optionalText, text: optionalText }),
    v.check((s) => s.choice !== undefined || s.text !== undefined),
  ),
]);

const MsgFields = {
  r: v.literal('msg'),
  id: str,
  seq: int,
  pos: int,
  g: str,
  thread: v.nullable(str),
  main: bool,
  by: str,
  ts: int,
  text: str,
  mentions: strs,
  re: v.nullable(str),
  env: v.optional(Envelope),
};

function message<K extends string, B extends v.GenericSchema, S extends v.GenericSchema>(
  kind: K,
  body: B,
  state: S,
) {
  return v.object({ ...MsgFields, kind: v.literal(kind), body, state });
}

const noCard = v.null();
export const MsgRow = v.variant('kind', [
  message('text', EmptyBody, noCard),
  message('progress', EmptyBody, noCard),
  message('question', EmptyBody, noCard),
  message('reply', EmptyBody, noCard),
  message('brief', BriefBody, BriefState),
  message('task', v.object({ task: str }), noCard),
  message('approval', ApprovalBody, ApprovalState),
  message('decision', DecisionBody, DecisionState),
  message(
    'summary',
    v.object({
      task: str,
      score: int,
      rounds: int,
      minutes: int,
      added: int,
      removed: int,
      pr: optionalInt,
    }),
    noCard,
  ),
  message(
    'call',
    v.object({ to: str, depth: int, wait: bool }),
    v.variant('status', [
      v.object({ status: v.literal('waiting') }),
      v.object({ status: v.literal('answered'), reply: str, ms: int }),
    ]),
  ),
  message(
    'report',
    v.object({ commit: str, files: int, added: int, removed: int, pr: optionalInt }),
    noCard,
  ),
  message(
    'checks',
    v.object({
      stage: v.picklist(['local', 'cloud', 'merge']),
      ok: bool,
      commit: str,
      items: v.array(v.object({ name: str, ok: bool, detail: optionalText })),
    }),
    noCard,
  ),
  message(
    'review',
    v.object({
      task: str,
      num: int,
      title: str,
      commit: str,
      round: int,
      score: int,
      pass: int,
      ok: bool,
      items: v.array(v.object({ text: str, veto: bool, file: optionalText, line: optionalInt })),
    }),
    noCard,
  ),
  message(
    'handback',
    v.object({
      files: strs,
      commits: v.array(v.object({ sha: str, subject: str })),
      added: int,
      removed: int,
    }),
    noCard,
  ),
  message('system', SystemBody, noCard),
]);
export type MsgRow = v.InferOutput<typeof MsgRow>;

export const NoteRow = v.object({
  r: v.literal('note'),
  ...RowHeader,
  pos: int,
  g: str,
  task: v.nullable(str),
  text: str,
  by: str,
  ts: num,
});
export type NoteRow = v.InferOutput<typeof NoteRow>;

export const Row = v.union([
  GoneRow,
  GroupRow,
  HumanMemberRow,
  AIMemberRow,
  DeviceRow,
  TaskRow,
  MsgRow,
  NoteRow,
]);
export type Row = v.InferOutput<typeof Row>;

// Reading a group (datachannel.md section 4)

export const Sync = v.object({
  t: v.literal('sync'),
  id: num,
  g: str,
  since: num,
  state_only: bool,
});
export type Sync = v.InferOutput<typeof Sync>;

export const Reset = v.object({ t: v.literal('reset'), g: str });
export type Reset = v.InferOutput<typeof Reset>;

export const Synced = v.object({ t: v.literal('synced'), ...Success, g: str, seq: num });
export type Synced = v.InferOutput<typeof Synced>;

export const Rows = v.object({ t: v.literal('rows'), g: str, rows: v.array(Row) });
export type Rows = v.InferOutput<typeof Rows>;

const FetchPage = { t: v.literal('fetch'), id: num, g: str, before: num, limit: num };

/** A page of a thread, or of the main line when `thread` is null. */
export const FetchThread = v.object({ ...FetchPage, thread: v.nullable(str) });
export type FetchThread = v.InferOutput<typeof FetchThread>;

/** A page of the review messages or of the notes. */
export const FetchKind = v.object({ ...FetchPage, kind: str });
export type FetchKind = v.InferOutput<typeof FetchKind>;

export const Fetched = v.object({
  t: v.literal('fetched'),
  ...Success,
  rows: v.array(Row),
  more: bool,
});
export type Fetched = v.InferOutput<typeof Fetched>;

// Commands (datachannel.md section 5)

export const Cmd = v.object({ t: v.literal('cmd'), id: num, env: Envelope });
export type Cmd = v.InferOutput<typeof Cmd>;

export const CmdResult = v.object({
  ...Success,
  num: v.optional(num),
  passed: v.optional(bool),
  round: v.optional(num),
  link: v.optional(str),
  expires: v.optional(num),
});
export type CmdResult = v.InferOutput<typeof CmdResult>;

export const ErrorResponse = v.object({
  re: num,
  err: v.object({ code: str, msg: str }),
});
export type ErrorResponse = v.InferOutput<typeof ErrorResponse>;

// The ctl channel (datachannel.md section 2)

/** Every frame a client can receive on `ctl`: a response (it has `re`) or a push. */
export const Frame = v.union([
  Welcome,
  Synced,
  Fetched,
  CmdResult,
  ErrorResponse,
  Rows,
  Reset,
  Devlog,
  Groups,
]);
export type Frame = v.InferOutput<typeof Frame>;
export type Push = Rows | Reset | Devlog | Groups;

/** The one place where received JSON text becomes a typed value. Throws when the text is not
 *  JSON or does not match the schema; everything after it trusts the type. */
export function decode<S extends v.GenericSchema>(schema: S, text: string): v.InferOutput<S> {
  return v.parse(schema, JSON.parse(text));
}

/** A request as its sender writes it; the connection numbers it. */
type Ask<R> = Omit<R, 'id'>;

/** A `ctl` connection: each request is answered by the response that names its id in `re`, or
 *  by an error. Pushes go to the handler given when the connection was made. Implemented by
 *  `mock.ts`, and by the real connection later. */
export interface Ctl {
  request(req: Ask<Hello>): Promise<Welcome | ErrorResponse>;
  request(req: Ask<Sync>): Promise<Synced | ErrorResponse>;
  request(req: Ask<FetchThread> | Ask<FetchKind>): Promise<Fetched | ErrorResponse>;
  request(req: Ask<Cmd>): Promise<CmdResult | ErrorResponse>;
}
