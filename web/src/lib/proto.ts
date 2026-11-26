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
  exp: v.optional(int),
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

/** The whole list of readable groups again, whenever it changes, entries as in `welcome`. */
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

const ForgeTemplates = v.object({ push: strs, open: strs, checks: strs, merge: strs });

export const GroupRow = v.object({
  r: v.literal('group'),
  ...RowHeader,
  name: str,
  kind: str,
  repo: v.optional(str),
  base: str,
  forge: str,
  templates: v.optional(ForgeTemplates),
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

/** An agent's usage caps; a cap that is not there is not set (commands.md section 2). `max` may be
 *  a fraction. */
const Cap = v.object({ unit: str, max: num });
const Caps = record({ day: v.optional(Cap), task: v.optional(Cap) });

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
  tool: v.optional(str),
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
  /** The profile (contracts/step-3/profiles.md section 1): `""` is the CLI's own default. */
  model: str,
  effort: str,
  tier: str,
  skills: str,
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
  asked: v.optional(strs),
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
  /** Failures of one kind in a row; `ask` while a submission is refused (profiles.md section 4). */
  stuck: v.optional(v.object({ kind: str, n: int, ask: bool })),
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
  preset: optionalText,
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
const ConfirmBody = v.object({ what: str, ai: str, to: strs });
const BulletinBody = v.object({ from: str, group: str, ai: str });
const BulletinSigned = v.object({ from: Id, group: str, ai: str });
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
  v.object({ code: v.literal('member_joined'), member: str, by: str }),
  v.object({ code: v.literal('member_removed'), user: str, name: str, by: optionalText }),
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
const ConfirmState = v.variant('status', [
  Pending,
  v.object({ status: v.picklist(['confirmed', 'denied']), ...acted }),
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
  message('confirm', ConfirmBody, ConfirmState),
  message('bulletin', BulletinBody, noCard),
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
      v.object({ status: v.literal('denied'), at: int }),
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
  type: optionalText,
  name: optionalText,
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

/** A page of the review messages, context notes, presets or knowledge-base documents, or a
 *  knowledge-base search when `q` is set (contracts/step-3/knowledge.md section 3). */
export const FetchKind = v.object({ ...FetchPage, kind: str, q: optionalText });
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
  id: v.optional(str),
  replaced: v.optional(bool),
});
export type CmdResult = v.InferOutput<typeof CmdResult>;

export const ErrorResponse = v.object({
  re: int,
  err: v.object({ code: str, msg: str, home: optionalText }),
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
  repo: optionalText,
  base: str,
  forge: str,
  checks: strs,
  member_name: str,
  templates: v.optional(ForgeTemplates),
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
  templates: v.optional(ForgeTemplates),
});
export type GroupSet = v.InferOutput<typeof GroupSet>;

export const GroupCommand = v.object({ ...group, op: v.picklist(['pause', 'resume']) });
export type GroupCommand = v.InferOutput<typeof GroupCommand>;

const member = { t: v.literal('member'), ...HomeCommand, id: Id };

/** The four fields of the profile in a command, each optional (profiles.md section 2). */
const ProfileSet = {
  model: optionalText,
  effort: optionalText,
  tier: optionalText,
  skills: optionalText,
};

export const MemberAddAI = v.object({
  ...member,
  op: v.literal('add_ai'),
  cli: str,
  tool: optionalText,
  name: str,
  account: str,
  secretary: bool,
  approvers: strs,
  ...ProfileSet,
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
  tool: optionalText,
  allow: v.optional(strs),
  approvers: v.optional(strs),
  role: optionalText,
  ...ProfileSet,
});
export type MemberSet = v.InferOutput<typeof MemberSet>;

export const MemberCommand = v.object({ ...member, op: v.literal('remove') });
export type MemberCommand = v.InferOutput<typeof MemberCommand>;

/** Asks the home for an invitation link for its group; answered with `link` and `expires` in
 *  `CmdResult` (contracts/step-2/people.md section 2.1). */
export const Invite = v.object({ t: v.literal('invite'), ...HomeCommand });
export type Invite = v.InferOutput<typeof Invite>;

/** Admits the person of a joining connection to the invitation's group (people.md section 2.2). */
export const Join = v.object({ t: v.literal('join'), ...HomeCommand, name: str });
export type Join = v.InferOutput<typeof Join>;

/** What the reviewer signs of a review body; the home adds the rest and answers `passed` and
 *  `round` (commands.md 1.8). */
const ReviewSigned = v.object({ task: Id, commit: str, score: int, items: v.array(ReviewItem) });

/** What a person signs to ask to watch an AI's terminal; the home adds `to`
 *  (contracts/step-2/confirm.md section 3). */
const ConfirmSigned = v.object({ what: str, ai: Id });

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
  post('confirm', ConfirmSigned),
  post('bulletin', BulletinSigned),
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
  type: optionalText,
  name: optionalText,
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
  code: optionalText,
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
  account: optionalText,
  tool: optionalText,
  ...ProfileSet,
});
export type HostJoin = v.InferOutput<typeof HostJoin>;

export const HostLeave = v.object({
  t: v.literal('host.leave'),
  ...HostCommand,
  g: Id,
  agent: str,
  /** Required when the agent has more than one member in `g`. */
  member: v.optional(Id),
});
export type HostLeave = v.InferOutput<typeof HostLeave>;

/** Records the home of a group on one of a member's nodes before any AI on that node joins it
 *  (contracts/step-2/people.md section 7). */
export const HomeAdd = v.object({
  t: v.literal('home.add'),
  ...HostCommand,
  g: Id,
  home: str,
  name: str,
  addrs: strs,
});
export type HomeAdd = v.InferOutput<typeof HomeAdd>;

/** Only the settings being changed. `caps` holds the caps of each agent being changed, whole: a
 *  cap that is not there is no longer set; an agent that is not there keeps its caps. */
export const SettingsSet = v.object({
  t: v.literal('settings.set'),
  ...HostCommand,
  retention_days: optionalInt,
  idle_minutes: optionalInt,
  max_open: optionalInt,
  default_level: optionalInt,
  all_level3: v.optional(bool),
  approvers: v.optional(strs),
  caps: v.optional(record({ claude: v.optional(Caps), codex: v.optional(Caps) })),
});
export type SettingsSet = v.InferOutput<typeof SettingsSet>;

/** Declares the account type of a CLI found on the node: `personal`, `team` or `api_key`. */
export const AgentSet = v.object({
  t: v.literal('agent.set'),
  ...HostCommand,
  agent: str,
  account: str,
});
export type AgentSet = v.InferOutput<typeof AgentSet>;

export const PermSet = v.object({
  t: v.literal('perm.set'),
  ...HostCommand,
  agent: str,
  g: Id,
  level: int,
});
export type PermSet = v.InferOutput<typeof PermSet>;

/** The host commands that carry nothing but their header. `pair.browser` is answered with
 *  `link` and `expires`. */
export const HostCommandOnly = v.object({
  t: v.picklist(['pair.browser', 'host.offline', 'host.online']),
  ...HostCommand,
});
export type HostCommandOnly = v.InferOutput<typeof HostCommandOnly>;

const takeover = { ...HostCommand, g: Id, member: Id };

/** May name the task to work on (planning/flows/takeover.md, section A). */
export const TakeoverStart = v.object({
  t: v.literal('takeover.start'),
  ...takeover,
  task: v.optional(Id),
});
export type TakeoverStart = v.InferOutput<typeof TakeoverStart>;

export const TakeoverEnd = v.object({ t: v.literal('takeover.end'), ...takeover });
export type TakeoverEnd = v.InferOutput<typeof TakeoverEnd>;

export const WatchStart = v.object({ t: v.literal('watch.start'), id: int, g: Id, member: Id });
export type WatchStart = v.InferOutput<typeof WatchStart>;
export const WatchEnd = v.object({
  t: v.literal('watch.end'),
  ...HostCommand,
  g: Id,
  member: Id,
  user: str,
});
export type WatchEnd = v.InferOutput<typeof WatchEnd>;

// The host surface (datachannel.md section 7): what the owner's browser reads from a node.

/** `refresh` skips what was kept of the detection and the model lists (drivers.md 10.1). */
export const HostInfoRequest = v.object({
  t: v.literal('host.info'),
  id: int,
  refresh: v.optional(bool),
});
export type HostInfoRequest = v.InferOutput<typeof HostInfoRequest>;

/** The capability table a CLI driver declares (contracts/step-3/drivers.md sections 4 and 6). */
export const Driver = v.object({
  accounts: strs,
  takeover: str,
});
export type Driver = v.InferOutput<typeof Driver>;

const HostState = {
  device: str,
  name: str,
  os: str,
  version: str,
  exe: str,
  open: int,
  max_open: int,
  offline: bool,
  /** After an update check: `pending` or `mismatch` (contracts/local.md section 9). */
  update: v.nullable(v.object({ version: str, status: str })),
  homes: v.array(v.object({ g: Id, home: str, name: str })),
  /** The quota answer of the rendezvous; `null` while the node has none. */
  relay: v.nullable(v.object({ used: int, limit: int })),
  agents: v.array(
    v.object({
      agent: str,
      cli_version: str,
      /** `null` until the owner has declared it. */
      account: v.nullable(str),
      login_ok: bool,
      caps: Caps,
      driver: Driver,
      /** The models the CLI lists, with the suggested tier (drivers.md 10.3). */
      models: v.array(
        v.object({
          model: str,
          name: str,
          efforts: strs,
          fast: v.optional(v.literal(true)),
          tier: str,
        }),
      ),
      groups: v.array(
        v.object({
          g: Id,
          member: Id,
          level: int,
          state: str,
          task: v.optional(Id),
          queued: int,
          watchers: strs,
        }),
      ),
    }),
  ),
  settings: v.object({
    retention_days: int,
    idle_minutes: int,
    default_level: int,
    all_level3: bool,
    approvers: strs,
  }),
};

/** The answer to `host.info`. */
export const HostInfo = v.object({ t: v.literal('host.info'), ...Success, ...HostState });
export type HostInfo = v.InferOutput<typeof HostInfo>;

/** `host.info` again after one of its fields changed: a push, the whole object, without `re`. */
export const HostInfoPush = v.object({ t: v.literal('host.info'), ...HostState });
export type HostInfoPush = v.InferOutput<typeof HostInfoPush>;

/** The size of a takeover's terminal, pushed by the browser; `ai` is the member taken over. */
export const PTYResize = v.object({
  t: v.literal('pty.resize'),
  g: Id,
  ai: Id,
  cols: int,
  rows: int,
});
export type PTYResize = v.InferOutput<typeof PTYResize>;

/** The payload of the signed envelope a node multicasts on its local network (rendezvous.md
 *  section 4). A browser receives none; the schema is here because both test suites read it. */
export const Beacon = v.object({ t: v.literal('beacon'), n: Id, ts: int, port: int });
export type Beacon = v.InferOutput<typeof Beacon>;

// Inbox (datachannel.md section 6): what a home sends the host of an AI. A browser receives none
// of it; the schemas are here because every wire example is read by both test suites.

/** An inbox item for an AI or its host, recorded when the home queued it. `item` numbers it for
 *  `ack` (`items[].item.item`); `ai`, `task` and `msg` are ids. Which fields an item of each `why`
 *  has is in the table of that section. */
export const Item = v.object({
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

/** `item` is the plain item recorded when the home queued it. `msg` and `task` are copies of the
 *  rows it names, taken when the item is sent. */
export const Inbox = v.object({
  t: v.literal('inbox'),
  g: Id,
  items: v.array(
    v.object({
      item: Item,
      msg: v.optional(MsgRow),
      task: v.optional(TaskRow),
    }),
  ),
});
export type Inbox = v.InferOutput<typeof Inbox>;

export const Ack = v.object({ t: v.literal('ack'), g: Id, item: int });
export type Ack = v.InferOutput<typeof Ack>;

const TraceKey = v.object({ name: str, device: str });

const TraceInbox = v.object({
  to: str,
  inbox: Inbox,
  acks: v.array(Ack),
});

const TraceStep = v.object({
  from: str,
  cmd: Cmd,
  answer: CmdResult,
  rows: Rows,
  inboxes: v.array(TraceInbox),
});

/** A complete signed transcript of one confirmed task from request to summary
 *  (flows/trace-task.md): `trace-remote.json` and `trace-local.json`. */
export const TaskTrace = v.object({
  keys: v.array(TraceKey),
  devlog: Devlog,
  group: GroupRow,
  owner: HumanMemberRow,
  ais: v.array(AIMemberRow),
  seq: int,
  num: int,
  steps: v.array(TraceStep),
});
export type TaskTrace = v.InferOutput<typeof TaskTrace>;

export const Handout = v.object({
  run: int,
  kind: str,
  task: optionalText,
  folder: str,
  context: str,
  text: str,
});

// MCP tools (mcp-tools.md section 5): what a node offers its AI sessions. Not a frame between
// nodes, so not read by the rules of contracts/conventions.md: the names follow MCP, and a member
// that the file does not have is refused, as internal/proto's test refuses it.

/** The JSON Schema of one argument, or of an element or member of one. */
export interface ToolArg {
  type: string;
  enum?: string[];
  default?: unknown;
  minimum?: number;
  maximum?: number;
  items?: ToolArg;
  properties?: Record<string, ToolArg>;
  required?: string[];
  additionalProperties?: boolean;
}

const ToolArg: v.GenericSchema<ToolArg> = v.strictObject({
  type: str,
  enum: v.optional(strs),
  default: v.optional(v.unknown()),
  minimum: optionalInt,
  maximum: optionalInt,
  items: v.optional(v.lazy(() => ToolArg)),
  properties: v.optional(
    v.record(
      str,
      v.lazy(() => ToolArg),
    ),
  ),
  required: v.optional(strs),
  additionalProperties: v.optional(bool),
});

const ToolList = v.strictObject({
  tools: v.array(
    v.strictObject({
      name: str,
      inputSchema: v.strictObject({
        type: v.literal('object'),
        properties: v.record(str, ToolArg),
        required: v.optional(strs),
        additionalProperties: v.literal(false),
      }),
    }),
  ),
});

/** internal/proto/testdata/tools.json: the `tools/list` result of each kind of session. */
export const ToolLists = v.strictObject({
  secretary: ToolList,
  group: ToolList,
  review: ToolList,
  task_level_1: ToolList,
  task_level_2_3: ToolList,
  external: ToolList,
});
export type ToolLists = v.InferOutput<typeof ToolLists>;

// The ctl channel (datachannel.md section 2)

/** What a client can receive on `ctl` without asking: every push, which has no `re`, and the
 *  error answer. A frame with `re` is the answer to a request: read it with `decodeAnswer`, by the
 *  type of the request whose `id` it names. */
export const Frame = v.union([ErrorResponse, Rows, Reset, Devlog, Groups, HostInfoPush, PTYResize]);
export type Frame = v.InferOutput<typeof Frame>;
export type Push = Rows | Reset | Devlog | Groups | HostInfoPush | PTYResize;

/** The schema of the answer to each request, by the request's `t`. An answer is read with the
 *  schema of the request it answers, not by what it holds: the answer to `cmd` has no
 *  `t`, and like any frame each ignores the members it does not read (contracts/conventions.md
 *  sections 2 and 3). */
export const AnswerOf = {
  hello: Welcome,
  sync: Synced,
  fetch: Fetched,
  cmd: CmdResult,
  'host.info': HostInfo,
  'watch.start': CmdResult,
} as const;
export type RequestType = keyof typeof AnswerOf;
export type AnswerTo<T extends RequestType> = v.InferOutput<(typeof AnswerOf)[T]>;

/** The one place where received JSON text becomes a typed value. Throws when the text is not
 *  JSON or does not match the schema; everything after it trusts the type. */
export function decode<S extends v.GenericSchema>(schema: S, text: string): v.InferOutput<S> {
  return v.parse(schema, JSON.parse(text));
}

/** Reads a frame that has `re` as the answer to a request of type `t`. A frame with `err` is the
 *  error answer, whatever else it holds (contracts/conventions.md section 3); any other is read
 *  with `AnswerOf[t]`. Throws as `decode` does. */
export function decodeAnswer<T extends RequestType>(
  t: T,
  text: string,
): AnswerTo<T> | ErrorResponse {
  const value: unknown = JSON.parse(text);
  const error = typeof value === 'object' && value !== null && 'err' in value;
  return v.parse(error ? ErrorResponse : AnswerOf[t], value) as AnswerTo<T> | ErrorResponse;
}

/** A request as its sender writes it; the connection numbers it. */
type Ask<R> = Omit<R, 'id'>;

/** A binary data channel opened beside `ctl`, such as `pty:<g>:<member>` (datachannel.md 1, 7). */
export interface PtyChannel {
  binaryType: BinaryType;
  readonly readyState: string;
  onmessage: ((this: any, event: any) => any) | null;
  onclose: ((this: any, event: any) => any) | null;
  send(data: Uint8Array<ArrayBuffer>): void;
  close(): void;
}

/** A `ctl` connection. It numbers each request with a new `id` and keeps the request's `t` under
 *  that id. A frame with `re` is the answer to the request of that id: it is read with
 *  `decodeAnswer(t, text)`, settles that request, and the id is forgotten. A frame without `re` is
 *  read with `Frame` and goes to the push handler given when the connection was made. Every request
 *  type has an overload, so its answer is typed. Implemented by `mock.ts`, and by the real
 *  connection later. */
export interface Ctl {
  request(req: Ask<Hello>): Promise<AnswerTo<'hello'> | ErrorResponse>;
  request(req: Ask<Sync>): Promise<AnswerTo<'sync'> | ErrorResponse>;
  request(
    req: Ask<FetchThread> | Ask<FetchKind> | Ask<FetchRow>,
  ): Promise<AnswerTo<'fetch'> | ErrorResponse>;
  request(req: Ask<Cmd>): Promise<AnswerTo<'cmd'> | ErrorResponse>;
  request(req: Ask<HostInfoRequest>): Promise<AnswerTo<'host.info'> | ErrorResponse>;
  request(req: Ask<WatchStart>): Promise<AnswerTo<'watch.start'> | ErrorResponse>;
  send?(frame: Devlog | PTYResize): void;
  channel?(label: string): PtyChannel | undefined;
}
