// Wire types, mirroring internal/proto. Each message is declared once, as a valibot schema; its
// TypeScript type is inferred from that schema under the same name. An unknown field is dropped,
// as Go's encoding/json does, so a node that adds a field does not break an older client; a missing
// field is an error. A Go field tagged omitempty is optional here; a Go pointer without omitempty
// is required and may be null. json.RawMessage fields are any JSON until a contract types them.
import * as v from 'valibot';

const str = v.string();
// A number on the wire is an integer, as a Go integer field reads it (contracts/conventions.md
// section 2), except the few fields that the contracts give as a fraction.
// JSON.parse reads a number beyond the largest double as Infinity, which Go refuses.
const num = v.pipe(v.number(), v.finite());
const int = v.pipe(num, v.safeInteger());
const bool = v.boolean();
const strs = v.array(str);

/** An object with these fields. valibot's object also accepts an array, which Go refuses: with
 *  only optional fields, [] would pass. */
const record = <E extends v.ObjectEntries>(entries: E) =>
  v.pipe(
    v.unknown(),
    v.check((x) => !Array.isArray(x)),
    v.object(entries),
  );

// The signed payload `p` is kept as the exact string received: its bytes are what is signed.
export const Envelope = v.object({ p: str, k: str, s: str });
export type Envelope = v.InferOutput<typeof Envelope>;

/** A random id: 16 bytes in their canonical base64url spelling (contracts/envelope.md section 1).
 *  That is 21 characters and one that carries two bits, its other four being zero. A field that
 *  holds an id takes no other text. */
export const Id = v.pipe(str, v.regex(/^[A-Za-z0-9_-]{21}[AQgw]$/));

/** A task's name in its branch and its worktree, as the home makes it from the title: at most 40
 *  characters, groups of a to z and 0 to 9 with one hyphen between two (contracts/local.md section
 *  7). A field that holds a slug takes no other text, as internal/proto's Slug takes none. */
export const Slug = v.pipe(str, v.maxLength(40), v.regex(/^[a-z0-9]+(-[a-z0-9]+)*$/));

const PayloadHeader = { n: Id, ts: int, g: v.optional(Id), to: v.optional(str) };
const SignalHeader = { ...PayloadHeader, conn: Id };

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
  ver: int,
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

// WebSocket signalling (rendezvous.md section 2)

const iceServer = v.object({ urls: strs, username: v.optional(str), credential: v.optional(str) });
const iceConfig = { ice: v.array(iceServer), ttl: int };

export const RVChallenge = v.object({ t: v.literal('challenge'), nonce: str });
export type RVChallenge = v.InferOutput<typeof RVChallenge>;

export const RVAuth = v.object({ t: v.literal('auth'), k: str, s: str });
export type RVAuth = v.InferOutput<typeof RVAuth>;

export const RVReady = v.object({ t: v.literal('ready'), ...iceConfig });
export type RVReady = v.InferOutput<typeof RVReady>;

export const RVICERequest = v.object({ t: v.literal('ice') });
export type RVICERequest = v.InferOutput<typeof RVICERequest>;

export const RVQuotaRequest = v.object({ t: v.literal('quota') });
export type RVQuotaRequest = v.InferOutput<typeof RVQuotaRequest>;

export const RVQuotaAnswer = v.object({ t: v.literal('quota'), used: int, limit: int });
export type RVQuotaAnswer = v.InferOutput<typeof RVQuotaAnswer>;

export const RVICEAnswer = v.object({ t: v.literal('ice'), ...iceConfig });
export type RVICEAnswer = v.InferOutput<typeof RVICEAnswer>;

export const RVTo = v.object({ t: v.literal('to'), k: str, env: Envelope });
export type RVTo = v.InferOutput<typeof RVTo>;

export const RVFrom = v.object({ t: v.literal('from'), k: str, env: Envelope });
export type RVFrom = v.InferOutput<typeof RVFrom>;

export const RVOffline = v.object({ t: v.literal('offline'), k: str });
export type RVOffline = v.InferOutput<typeof RVOffline>;

export const RVWatch = v.object({ t: v.literal('watch'), k: v.pipe(strs, v.maxLength(64)) });
export type RVWatch = v.InferOutput<typeof RVWatch>;

export const RVPresence = v.object({ t: v.literal('presence'), k: str, on: bool });
export type RVPresence = v.InferOutput<typeof RVPresence>;

const roomId = v.pipe(str, v.regex(/^[0-9a-f]{32}$/));

export const RVJoin = v.object({ t: v.literal('join'), room: roomId });
export type RVJoin = v.InferOutput<typeof RVJoin>;

export const RVJoined = v.object({ t: v.literal('joined'), room: roomId });
export type RVJoined = v.InferOutput<typeof RVJoined>;

// What two devices send each other through a pairing room is their pair message (envelope.md 6).
export const RVRoom = v.object({ t: v.literal('room'), room: roomId, data: Pair });
export type RVRoom = v.InferOutput<typeof RVRoom>;

// Session (datachannel.md section 3)

// A frame with `err` is an error, whatever else it holds (contracts/conventions.md section 3).
const Success = { re: int, ok: v.literal(true), err: v.optional(v.never()) };

export const Hello = v.object({
  t: v.literal('hello'),
  id: int,
  v: int,
  kind: str,
  name: str,
  addrs: strs,
});
export type Hello = v.InferOutput<typeof Hello>;

const GroupSummary = v.object({ g: str, name: str, seq: int });

export const Welcome = v.object({
  t: v.literal('welcome'),
  ...Success,
  v: int,
  device: str,
  name: str,
  version: str,
  time: int,
  addrs: strs,
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

const RowHeader = { id: str, seq: int };

export const GoneRow = v.object({ r: str, ...RowHeader, gone: v.literal(true) });
export type GoneRow = v.InferOutput<typeof GoneRow>;

const Limits = v.object({
  rounds: int,
  wakes_per_hour: int,
  turn_minutes: int,
  failures: int,
  review_rounds: int,
  ask_depth: int,
  ask_wait_s: int,
  approval_wait_s: int,
  idle_notice_minutes: int,
});

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
  pass: int,
  pass_foundation: int,
  limits: Limits,
  paused: v.nullable(v.object({ by: str, at: int })),
  created: int,
});
export type GroupRow = v.InferOutput<typeof GroupRow>;

const MemberRow = {
  r: v.literal('member'),
  ...RowHeader,
  g: str,
  user: str,
  name: str,
  last_seen: int,
};

const Usage = record({
  window_pct: v.optional(num),
  window_resets: v.optional(int),
  day_cap_pct: v.optional(num),
  spend_today: v.optional(num),
  spend_cap: v.optional(num),
  turns_today: v.optional(int),
});

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
  since: int,
  path: str,
  usage: Usage,
});
export type AIMemberRow = v.InferOutput<typeof AIMemberRow>;

export const DeviceRow = v.object({
  r: v.literal('device'),
  ...RowHeader,
  ...DeviceChange,
  env: Envelope,
});
export type DeviceRow = v.InferOutput<typeof DeviceRow>;

const Check = v.object({ ok: bool, at: int, commit: str });

export const TaskRow = v.object({
  r: v.literal('task'),
  ...RowHeader,
  g: str,
  num: int,
  title: str,
  slug: Slug,
  origin: str,
  brief: v.optional(str),
  plan: v.optional(str),
  state: str,
  step: str,
  commit: v.optional(str),
  assignee: str,
  reviewer: v.optional(str),
  foundation: bool,
  pass: int,
  depends: strs,
  branch: v.optional(str),
  pr: v.optional(int),
  pr_url: v.optional(str),
  checks: record({ local: v.optional(Check), cloud: v.optional(Check) }),
  review: v.optional(
    v.object({ round: int, failed: int, score: int, ok: bool, by: str, commit: str, at: int }),
  ),
  wait: v.nullable(
    v.object({
      kind: str,
      msg: v.optional(str),
      who: v.optional(strs),
      since: int,
      kept: v.optional(str),
    }),
  ),
  failures: int,
  thread: v.object({
    count: int,
    rounds: int,
    last: v.optional(v.object({ by: str, kind: str, text: str, ts: int })),
  }),
  stats: v.object({ added: int, removed: int, started: int, ended: v.nullable(int) }),
  created: int,
});
export type TaskRow = v.InferOutput<typeof TaskRow>;

const optionalText = v.optional(str);
const optionalInt = v.optional(int);
const EmptyBody = record({});

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
  criteria: strs,
  scope: v.object({ in: strs, out: strs }),
  tasks: v.array(BriefTask),
  pass: int,
  requester: str,
  replaces: v.nullable(str),
});

const ApprovalBody = v.object({
  task: str,
  ai: str,
  tool: str,
  command: str,
  full: optionalInt,
  why: optionalText,
  category: str,
  approvers: strs,
});

const DecisionBody = v.object({
  task: optionalText,
  question: str,
  context: optionalText,
  options: v.array(v.object({ id: str, label: str, effect: str })),
  recommended: optionalText,
  to: strs,
});

const CallBody = v.object({ to: str, depth: int, wait: bool });
const ReportBody = v.object({
  commit: str,
  files: int,
  added: int,
  removed: int,
  pr: optionalInt,
});
const CheckItem = v.object({ name: str, ok: bool, detail: optionalText });
const ReviewItem = v.object({ text: str, veto: bool, file: optionalText, line: optionalInt });
const HandbackBody = v.object({
  files: strs,
  commits: v.array(v.object({ sha: str, subject: str })),
  added: int,
  removed: int,
});

const SystemBody = v.variant('code', [
  v.object({ code: v.literal('rounds_limit'), task: str, n: int }),
  v.object({
    code: v.literal('turn_interrupted'),
    ai: str,
    reason: str,
    task: optionalText,
    detail: optionalText,
  }),
  v.object({ code: v.picklist(['group_paused', 'group_resumed']), by: str }),
  v.object({ code: v.literal('preempted'), task: str, by: str }),
  v.object({
    code: v.literal('cap_reached'),
    ai: str,
    cap: str,
    task: optionalText,
  }),
  v.object({ code: v.literal('rate_limited'), ai: str, resets: int }),
  v.object({ code: v.literal('login_invalid'), ai: str, detail: optionalText }),
  v.object({ code: v.literal('task_failed'), task: str, reason: str }),
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
  v.object({ status: v.literal('superseded'), ...v.partial(BriefAction).entries }),
]);
const ApprovalState = v.variant('status', [
  Pending,
  v.object({ status: v.picklist(['approved', 'denied']), ...acted }),
  v.object({ status: v.picklist(['expired', 'reassigned']), at: int }),
]);
const DecisionState = v.variant('status', [
  Pending,
  v.object({ status: v.literal('decided'), ...acted, choice: optionalText, text: optionalText }),
  v.object({ status: v.literal('expired'), at: int }),
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
    CallBody,
    v.variant('status', [
      v.object({ status: v.literal('waiting') }),
      v.object({ status: v.literal('answered'), reply: str, ms: int }),
    ]),
  ),
  message('report', ReportBody, noCard),
  message(
    'checks',
    v.object({
      stage: str,
      ok: bool,
      commit: str,
      items: v.array(CheckItem),
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
      items: v.array(ReviewItem),
    }),
    noCard,
  ),
  message('handback', HandbackBody, noCard),
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
  ts: int,
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
  id: int,
  g: str,
  since: int,
  state_only: bool,
});
export type Sync = v.InferOutput<typeof Sync>;

export const Reset = v.object({ t: v.literal('reset'), g: str });
export type Reset = v.InferOutput<typeof Reset>;

export const Synced = v.object({ t: v.literal('synced'), ...Success, g: str, seq: int });
export type Synced = v.InferOutput<typeof Synced>;

export const Rows = v.object({ t: v.literal('rows'), g: str, rows: v.array(Row) });
export type Rows = v.InferOutput<typeof Rows>;

const FetchPage = { t: v.literal('fetch'), id: int, g: str, before: int, limit: int };

/** A page of a thread, or of the main line when `thread` is null. */
export const FetchThread = v.object({ ...FetchPage, thread: v.nullable(str) });
export type FetchThread = v.InferOutput<typeof FetchThread>;

/** A page of the review messages or of the notes. */
export const FetchKind = v.object({ ...FetchPage, kind: str });
export type FetchKind = v.InferOutput<typeof FetchKind>;

/** One row of the group, by its type and id: `before` is 0 and `limit` 1. */
export const FetchRow = v.object({ ...FetchPage, r: str, row: str });
export type FetchRow = v.InferOutput<typeof FetchRow>;

export const Fetched = v.object({
  t: v.literal('fetched'),
  ...Success,
  rows: v.array(Row),
  more: bool,
});
export type Fetched = v.InferOutput<typeof Fetched>;

// Commands (datachannel.md section 5)

export const Cmd = v.object({ t: v.literal('cmd'), id: int, env: Envelope });
export type Cmd = v.InferOutput<typeof Cmd>;

export const CmdResult = v.object({
  ...Success,
  num: v.optional(int),
  passed: v.optional(bool),
  round: v.optional(int),
  link: v.optional(str),
  expires: v.optional(int),
});
export type CmdResult = v.InferOutput<typeof CmdResult>;

export const ErrorResponse = v.object({
  re: int,
  err: v.object({ code: str, msg: str }),
});
export type ErrorResponse = v.InferOutput<typeof ErrorResponse>;

// Home commands (commands.md section 1): the payload `p` of the envelope in a `cmd`. A receiver
// reads `t`, and the `op`, `kind` or `action` beside it, before it chooses a shape; the Go types
// hold those as text, and the schemas below name them. Every other field is read as Go reads it.

const HomeCommand = { n: Id, ts: int, g: Id, as: v.optional(Id) };
const group = { t: v.literal('group'), ...HomeCommand };

/** The receiving node becomes the home of a new group, whose id is both `g` and `n`. */
export const GroupCreate = v.object({
  ...group,
  op: v.literal('create'),
  name: str,
  kind: str,
  repo: str,
  base: str,
  forge: str,
  checks: strs,
  member_name: str,
});
export type GroupCreate = v.InferOutput<typeof GroupCreate>;

/** Only the fields being changed; `limits`, when sent, is whole. */
export const GroupSet = v.object({
  ...group,
  op: v.literal('set'),
  name: optionalText,
  checks: v.optional(strs),
  pass: optionalInt,
  pass_foundation: optionalInt,
  limits: v.optional(Limits),
  secretary: v.optional(Id),
});
export type GroupSet = v.InferOutput<typeof GroupSet>;

export const GroupCommand = v.object({ ...group, op: v.picklist(['pause', 'resume']) });
export type GroupCommand = v.InferOutput<typeof GroupCommand>;

const member = { t: v.literal('member'), ...HomeCommand, id: Id };

export const MemberAddAI = v.object({
  ...member,
  op: v.literal('add_ai'),
  cli: str,
  name: str,
  account: str,
  secretary: bool,
  approvers: strs,
});
export type MemberAddAI = v.InferOutput<typeof MemberAddAI>;

/** A fact a host reports: the body of a system message without `ai`, which the home adds
 *  (commands.md 1.9, messages.md 2.11). */
const HostEvent = v.variant('code', [
  v.object({
    code: v.literal('turn_interrupted'),
    reason: str,
    task: v.optional(Id),
    detail: optionalText,
  }),
  v.object({ code: v.literal('cap_reached'), cap: str, task: v.optional(Id) }),
  v.object({ code: v.literal('rate_limited'), resets: int }),
  v.object({ code: v.literal('login_invalid'), detail: optionalText }),
  v.object({ code: v.literal('session_replaced'), task: v.optional(Id) }),
]);

export const MemberStatus = v.object({
  ...member,
  op: v.literal('status'),
  state: str,
  task: v.optional(Id),
  path: str,
  usage: Usage,
  event: v.optional(HostEvent),
});
export type MemberStatus = v.InferOutput<typeof MemberStatus>;

/** Only the fields being changed. */
export const MemberSet = v.object({
  ...member,
  op: v.literal('set'),
  name: optionalText,
  account: optionalText,
  allow: v.optional(strs),
  approvers: v.optional(strs),
});
export type MemberSet = v.InferOutput<typeof MemberSet>;

export const MemberCommand = v.object({ ...member, op: v.literal('remove') });
export type MemberCommand = v.InferOutput<typeof MemberCommand>;

/** What the reviewer signs of a review body; the home adds the rest and answers `passed` and
 *  `round` (commands.md 1.8). */
const ReviewSigned = v.object({ task: Id, commit: str, score: int, items: v.array(ReviewItem) });

function post<K extends string, B extends v.GenericSchema>(kind: K, body: B) {
  return v.object({
    t: v.literal('post'),
    ...HomeCommand,
    kind: v.literal(kind),
    thread: v.nullable(Id),
    main: bool,
    text: str,
    mentions: v.array(Id),
    re: v.nullable(Id),
    body,
  });
}

/** The message a person or an AI sends; its id is `n`. `body` holds what the author signs of the
 *  body of its kind (messages.md section 2). */
export const Post = v.variant('kind', [
  post('text', EmptyBody),
  post('progress', EmptyBody),
  post('question', EmptyBody),
  post('reply', EmptyBody),
  post('brief', BriefBody),
  post('summary', EmptyBody),
  post('approval', ApprovalBody),
  post('decision', DecisionBody),
  post('call', CallBody),
  post('report', ReportBody),
  post('review', ReviewSigned),
  post('handback', HandbackBody),
]);
export type Post = v.InferOutput<typeof Post>;

/** Decides a card. Which action has `arg` and `text` is in commands.md 1.4. */
export const Act = v.object({
  t: v.literal('act'),
  ...HomeCommand,
  msg: Id,
  action: str,
  arg: optionalText,
  text: optionalText,
});
export type Act = v.InferOutput<typeof Act>;

export const Jump = v.object({ t: v.literal('jump'), ...HomeCommand, msg: Id });
export type Jump = v.InferOutput<typeof Jump>;

const task = { t: v.literal('task'), ...HomeCommand, id: Id };

export const TaskStart = v.object({
  ...task,
  op: v.literal('start'),
  title: str,
  plan: str,
  re: v.nullable(Id),
  origin: str,
});
export type TaskStart = v.InferOutput<typeof TaskStart>;

export const TaskBranch = v.object({ ...task, op: v.literal('branch'), branch: str });
export type TaskBranch = v.InferOutput<typeof TaskBranch>;

/** The result of the local checks (`checked`) and of the hosted ones (`cloud`). */
export const TaskChecked = v.object({
  ...task,
  op: v.picklist(['checked', 'cloud']),
  ok: bool,
  items: v.array(CheckItem),
  commit: str,
});
export type TaskChecked = v.InferOutput<typeof TaskChecked>;

const DiffStats = v.object({ added: int, removed: int });

export const TaskSubmitted = v.object({
  ...task,
  op: v.literal('submitted'),
  commit: str,
  pr: optionalInt,
  pr_url: optionalText,
  stats: DiffStats,
});
export type TaskSubmitted = v.InferOutput<typeof TaskSubmitted>;

export const TaskMerged = v.object({
  ...task,
  op: v.literal('merged'),
  commit: str,
  merge_commit: str,
  stats: DiffStats,
});
export type TaskMerged = v.InferOutput<typeof TaskMerged>;

export const TaskConflict = v.object({
  ...task,
  op: v.literal('conflict'),
  commit: str,
  detail: str,
});
export type TaskConflict = v.InferOutput<typeof TaskConflict>;

export const TaskFail = v.object({ ...task, op: v.literal('fail'), reason: str, detail: str });
export type TaskFail = v.InferOutput<typeof TaskFail>;

export const TaskPreempted = v.object({ ...task, op: v.literal('preempted'), msg: Id });
export type TaskPreempted = v.InferOutput<typeof TaskPreempted>;

/** At least one of the two. */
export const TaskAssign = v.object({
  ...task,
  op: v.literal('assign'),
  assignee: v.optional(Id),
  reviewer: v.optional(Id),
});
export type TaskAssign = v.InferOutput<typeof TaskAssign>;

export const TaskCommand = v.object({ ...task, op: v.picklist(['pause', 'resume', 'cancel']) });
export type TaskCommand = v.InferOutput<typeof TaskCommand>;

export const Note = v.object({
  t: v.literal('note'),
  ...HomeCommand,
  id: Id,
  task: v.nullable(Id),
  text: str,
});
export type Note = v.InferOutput<typeof Note>;

export const Forget = v.object({ t: v.literal('forget'), ...HomeCommand, r: str, id: Id });
export type Forget = v.InferOutput<typeof Forget>;

const push = { t: v.literal('push'), ...HomeCommand };

/** Subscribes the signing browser to reminders (commands.md 1.7). */
export const PushOn = v.object({
  ...push,
  sub: v.object({ endpoint: str, p256dh: str, auth: str }),
  vapid: v.object({ pub: str, priv: str }),
  on: strs,
});
export type PushOn = v.InferOutput<typeof PushOn>;

/** Ends the signing browser's reminders. A `push` whose `off` is true is this command, whatever
 *  else it holds; any other is a PushOn. */
export const PushOff = v.object({ ...push, off: v.literal(true) });
export type PushOff = v.InferOutput<typeof PushOff>;

/** What a subscribed browser receives through its push service (commands.md 1.7). */
export const Reminder = v.object({
  g: Id,
  msg: Id,
  group: str,
  kind: str,
  who: str,
  title: optionalText,
});
export type Reminder = v.InferOutput<typeof Reminder>;

// Host commands (commands.md section 2): `to` is the node the command is for.

const HostCommand = { n: Id, ts: int, to: str };

export const HostJoin = v.object({
  t: v.literal('host.join'),
  ...HostCommand,
  g: Id,
  home: str,
  agent: str,
  name: str,
  secretary: bool,
});
export type HostJoin = v.InferOutput<typeof HostJoin>;

export const HostLeave = v.object({
  t: v.literal('host.leave'),
  ...HostCommand,
  g: Id,
  agent: str,
});
export type HostLeave = v.InferOutput<typeof HostLeave>;

// Inbox (datachannel.md section 6): what a home sends the host of an AI. A browser receives none
// of it; the schemas are here because every wire example is read by both test suites.

/** The payload of an item's envelope, signed by the home when it queued the item. `task` and
 *  `msg` are ids. Which fields an item of each `why` has is in the table of that section. */
export const Item = v.object({
  t: v.literal('item'),
  n: Id,
  ts: int,
  g: Id,
  to: str,
  item: int,
  ai: Id,
  why: str,
  task: v.optional(Id),
  msg: v.optional(Id),
  commit: optionalText,
  pr: optionalInt,
  jump: v.optional(v.literal(true)),
  code: optionalText,
  detail: optionalText,
});
export type Item = v.InferOutput<typeof Item>;

/** `env` holds an Item. `msg` and `task` are copies of the rows it names; `cause` is the signed
 *  act that led to the item. */
export const Inbox = v.object({
  t: v.literal('inbox'),
  g: Id,
  items: v.array(
    v.object({
      env: Envelope,
      msg: v.optional(MsgRow),
      task: v.optional(TaskRow),
      cause: v.optional(Envelope),
    }),
  ),
});
export type Inbox = v.InferOutput<typeof Inbox>;

export const Ack = v.object({ t: v.literal('ack'), g: Id, item: int });
export type Ack = v.InferOutput<typeof Ack>;

// The ctl channel (datachannel.md section 2)

/** The answer to `cmd`, which has no `t`: a frame with a `t` is read by the schema it names, and
 *  refused when that schema refuses it (contracts/conventions.md section 3). */
const CmdAnswer = v.object({ t: v.optional(v.never()), ...CmdResult.entries });

/** Every frame a client can receive on `ctl`: a response (it has `re`) or a push. */
export const Frame = v.union([
  Welcome,
  Synced,
  Fetched,
  CmdAnswer,
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
  request(req: Ask<FetchThread> | Ask<FetchKind> | Ask<FetchRow>): Promise<Fetched | ErrorResponse>;
  request(req: Ask<Cmd>): Promise<CmdResult | ErrorResponse>;
}
