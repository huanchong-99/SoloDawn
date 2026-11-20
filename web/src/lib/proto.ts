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

/** A random id: 16 bytes in their canonical base64url spelling (contracts/envelope.md section 1).
 *  That is 21 characters and one that carries two bits, its other four being zero. A field that
 *  holds an id takes no other text. */
export const Id = v.pipe(str, v.regex(/^[A-Za-z0-9_-]{21}[AQgw]$/));

const PayloadHeader = { n: Id, ts: num, g: v.optional(Id), to: v.optional(str) };
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

export const MsgRow = v.object({
  r: v.literal('msg'),
  ...RowHeader,
  pos: num,
  g: str,
  thread: v.nullable(str),
  main: bool,
  by: str,
  kind: str,
  ts: num,
  text: str,
  mentions: strs,
  re: v.nullable(str),
  body: v.unknown(),
  state: v.nullable(
    v.object({
      status: str,
      by: v.optional(str),
      at: v.optional(num),
      note: v.optional(str),
      choice: v.optional(str),
      text: v.optional(str),
      reply: v.optional(str),
      ms: v.optional(num),
      act: v.optional(Envelope),
    }),
  ),
  env: v.optional(Envelope),
});
export type MsgRow = v.InferOutput<typeof MsgRow>;

export const NoteRow = v.object({
  r: v.literal('note'),
  ...RowHeader,
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
