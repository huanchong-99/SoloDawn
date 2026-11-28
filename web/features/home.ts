// A group home in memory, as far as a browser sees one, for the scenarios of the group list and
// the chat: it lists its groups, answers `sync`, `fetch` and `cmd` as planning/flows/sync.md says,
// and pushes every change to the connections that read the group. A test double: the real home is
// internal/home.
import { Home } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import type { AccountKind } from '../src/lib/onboard.svelte';
import * as p from '../src/lib/proto';

/** The person of the wire examples, and their member id there. */
export const user: string = JSON.parse(goldens['row-member-human']).user;
export const owner: string = JSON.parse(goldens['row-member-human']).id;

type Fields = Record<string, unknown>;

/** A wire example as a row, with some fields changed. */
export const example = (name: string, changes: Fields = {}) =>
  ({ ...p.decode(p.Row, goldens[name]), ...changes }) as p.Row & Fields;

interface Stored {
  name: string;
  seq: number;
  rows: Map<string, p.Row & Fields>;
}

interface Connection {
  push: (frame: p.Push) => void;
  reads: Set<string>;
  open: boolean;
}

const isState = (row: Fields) => ['group', 'member', 'device', 'task'].includes(row.r as string);
const pending = (row: Fields) => (row.state as Fields | null)?.status === 'pending';
const inMain = (row: Fields) => row.r === 'msg' && (row.thread === null || row.main === true);
const bySeq = (a: p.Row, b: p.Row) => a.seq - b.seq;

/** What a reset sends: the state rows, the cards still pending and the newest 50 of the main
 *  line (planning/flows/sync.md A, step 5). */
function coldRead(rows: (p.Row & Fields)[]) {
  const newest = rows
    .filter(inMain)
    .sort((a, b) => (a.pos as number) - (b.pos as number))
    .slice(-50);
  return rows.filter((row) => isState(row) || pending(row) || newest.includes(row));
}

export class TestHome {
  readonly groups = new Map<string, Stored>();
  /** Every request received, as sent. */
  readonly asked: Fields[] = [];
  /** How many reads this home has answered with `synced`. */
  synced = 0;
  /** How many rows a `rows` frame holds at most. */
  batch = 100;
  /** When set, a `sync` breaks its connection after this many `rows` frames, before `synced`. */
  breakAfter: number | undefined;
  /** When set, every `post` is refused with this code. */
  refuse: string | undefined;
  /** People, by user, whose log this home no longer holds: `add_person` is `not_found` for them. */
  readonly forgotten = new Set<string>();
  /** When set, answers every command this home does not apply itself. */
  reply: ((payload: Fields) => object) | undefined;
  readonly name: string;
  #connections = new Set<Connection>();
  #receipts = new Map<string, object>();

  constructor(name = 'workstation') {
    this.name = name;
  }

  /** Adds a group with its row and the person as its owner. */
  add(g: string, name: string, changes: Fields = {}) {
    this.groups.set(g, { name, seq: 0, rows: new Map() });
    this.write(g, example('row-group', { id: g, name, created: Date.now(), ...changes }));
    this.write(g, example('row-member-human', { g, name: 'Alex' }));
    return g;
  }

  /** Writes a row as the home does: it gets the next sequence number, a message that is new also
   *  its `pos`, and every connection that reads the group is pushed the row. */
  write(g: string, row: p.Row & Fields) {
    const group = this.groups.get(g)!;
    const key = `${row.r} ${row.id}`;
    const seq = (group.seq += 1);
    const pos = (group.rows.get(key)?.pos as number | undefined) ?? seq;
    const written = 'pos' in row ? { ...row, g, seq, pos } : { ...row, seq };
    group.rows.set(key, written);
    for (const connection of this.#connections)
      if (connection.reads.has(g)) connection.push({ t: 'rows', g, rows: [written as p.Row] });
    return written;
  }

  /** Writes a text message of a member to the main line. */
  say(g: string, by: string, text: string, changes: Fields = {}) {
    const id = `msg-${this.groups.get(g)!.seq + 1}`;
    return this.write(g, example('row-msg-text', { id, by, text, ts: Date.now(), ...changes }));
  }

  /** Removes a row on purpose: its marker takes its place. */
  remove(g: string, r: string, id: string) {
    const group = this.groups.get(g)!;
    const marker = { r, id, seq: (group.seq += 1), gone: true as const };
    group.rows.set(`${r} ${id}`, marker);
    for (const connection of this.#connections)
      if (connection.reads.has(g)) connection.push({ t: 'rows', g, rows: [marker] });
  }

  /** A new connection from a browser. Its requests are answered in a later task, as a node's. */
  connect(push: (frame: p.Push) => void) {
    const connection: Connection = { push, reads: new Set(), open: true };
    this.#connections.add(connection);
    const ctl = {
      request: async (req: Fields) => {
        await new Promise((later) => setTimeout(later));
        this.asked.push(req);
        if (!connection.open) throw new Error('connection closed');
        return this.#answer(connection, req);
      },
    } as p.Ctl;
    return { ctl, close: () => this.#close(connection) };
  }

  /** Stops the home: every connection is gone. */
  stop() {
    for (const connection of this.#connections) this.#close(connection);
  }

  #close(connection: Connection) {
    connection.open = false;
    this.#connections.delete(connection);
  }

  #listed() {
    return [...this.groups].map(([g, { name, seq }]) => ({ g, name, seq }));
  }

  #answer(connection: Connection, req: Fields): object {
    const re = this.asked.length;
    if (req.t === 'hello')
      return {
        ...p.decode(p.Welcome, goldens.welcome),
        re,
        name: this.name,
        time: Date.now(),
        groups: this.#listed(),
      };
    if (req.t === 'cmd') return { re, ...this.#command(req.env as p.Envelope) };
    const group = this.groups.get(req.g as string);
    if (!group) return { re, err: { code: 'not_found', msg: 'no such group' } };
    if (req.t === 'sync') return this.#sync(connection, req, group, re);
    return this.#fetch(req, group, re);
  }

  #sync(connection: Connection, req: Fields, group: Stored, re: number) {
    const g = req.g as string;
    const since = req.since as number;
    const all = [...group.rows.values()].sort(bySeq);
    const missed = all.filter((row) => row.seq > since);
    const delta = since !== 0 && missed.length <= 1000;
    if (!delta) connection.push({ t: 'reset', g });
    const rows = delta ? missed : coldRead(all);
    for (let i = 0; i < rows.length; i += this.batch) {
      if (this.breakAfter === i / this.batch) {
        this.breakAfter = undefined;
        this.#close(connection);
        throw new Error('connection closed');
      }
      connection.push({ t: 'rows', g, rows: rows.slice(i, i + this.batch) as p.Row[] });
    }
    connection.reads.add(g);
    this.synced += 1;
    return { t: 'synced', re, ok: true, g, seq: group.seq };
  }

  #fetch(req: Fields, group: Stored, re: number) {
    if (req.r) {
      const row = group.rows.get(`${req.r} ${req.row}`);
      return { t: 'fetched', re, ok: true, rows: row ? [row] : [], more: false };
    }
    const before = req.before as number;
    const scope = [...group.rows.values()]
      .filter((row) => (req.thread === null ? inMain(row) : row.thread === req.thread))
      .filter((row) => before === 0 || (row.pos as number) < before)
      .sort((a, b) => (b.pos as number) - (a.pos as number));
    const rows = scope.slice(0, req.limit as number);
    return { t: 'fetched', re, ok: true, rows, more: scope.length > rows.length };
  }

  /** Applies a command once: a nonce seen before gets the answer it got then. */
  #command(env: p.Envelope): object {
    const payload = JSON.parse(env.p) as Fields & { n: string; g: string; ts: number };
    const known = this.#receipts.get(payload.n);
    if (known) return known;
    const outcome = this.#apply(payload);
    this.#receipts.set(payload.n, outcome);
    return outcome;
  }

  #apply(payload: Fields & { n: string; g: string; ts: number }): object {
    if (payload.t === 'group' && payload.op === 'create') {
      // A new group has no secretary until an AI is made one (contracts/commands.md 1.1).
      this.add(payload.g, payload.name as string, { secretary: undefined });
      this.#relist();
      return { ok: true };
    }
    if (this.refuse) return { err: { code: this.refuse, msg: 'refused by the test' } };
    if (payload.t === 'member' && payload.op === 'add_person') return this.#addPerson(payload);
    if (payload.t === 'post' && this.groups.has(payload.g)) {
      const { n, ts, g, kind, thread, main, text, mentions, re, body } = payload;
      this.write(g, {
        ...example('row-msg-text'),
        ...{ id: n, by: owner, ts, kind, thread, main, text, mentions, re, body },
      } as p.Row & Fields);
      return { ok: true };
    }
    if (payload.t === 'group' && (payload.op === 'pause' || payload.op === 'resume')) {
      const row = this.groups.get(payload.g)!.rows.get(`group ${payload.g}`)!;
      const paused = payload.op === 'pause' ? { by: owner, at: payload.ts } : null;
      this.write(payload.g, { ...row, paused });
      return { ok: true };
    }
    if (this.reply) return this.reply(payload);
    return { err: { code: 'bad_request', msg: 'not a command this home knows' } };
  }

  #relist() {
    for (const connection of this.#connections)
      connection.push({ t: 'groups', groups: this.#listed() });
  }

  /** `member add_person` (contracts/step-2/people.md section 2.5): a person of another group here
   *  becomes a member, with what an accepted join writes but the device rows, which this home does
   *  not keep. */
  #addPerson(payload: Fields & { n: string; g: string }): object {
    const { n, g, user, name } = payload;
    const people = (rows: Map<string, Fields>) =>
      [...rows.values()].filter((row) => row.r === 'member' && row.kind === 'human');
    const group = this.groups.get(g)!;
    const known =
      !this.forgotten.has(user as string) &&
      [...this.groups.entries()].some(
        ([other, { rows }]) => other !== g && people(rows).some((row) => row.user === user),
      );
    if (!known) return { err: { code: 'not_found', msg: 'no log of this person here' } };
    const taken = people(group.rows).some((row) => row.user === user || row.name === name);
    if (taken) return { err: { code: 'conflict', msg: 'a member already' } };
    this.write(g, example('row-member-human', { id: n, g, user, name, role: 'member' }));
    const body = { code: 'member_joined', member: n, by: owner };
    const id = `msg-${group.seq + 1}`;
    this.write(g, example('row-msg-system', { id, ts: Date.now(), thread: null, body }));
    this.#relist();
    return { ok: true };
  }
}

/** A TestHome that also applies `act` commands to card rows, checking every received command
 *  against `proto.Act` and every resulting row against `proto.MsgRow`. While `hold` is set, an act
 *  waits for `release`. */
export class ActHome extends TestHome {
  hold = false;
  release: (() => void) | undefined;

  override connect(push: (frame: p.Push) => void) {
    const conn = super.connect(push);
    const base = conn.ctl.request as (req: Fields) => Promise<unknown>;
    const ctl = { request: (req: Fields) => this.#request(base, req) } as p.Ctl;
    return { ctl, close: conn.close };
  }

  async #request(base: (req: Fields) => Promise<unknown>, req: Fields) {
    if (req.t !== 'cmd') return base(req);
    const env = req.env as p.Envelope;
    if ((JSON.parse(env.p) as Fields).t !== 'act') return base(req);
    if (this.hold) await new Promise<void>((resolve) => (this.release = resolve));
    else await new Promise((later) => setTimeout(later));
    this.asked.push(req);
    const re = this.asked.length;
    if (this.refuse) return { re, err: { code: this.refuse, msg: 'refused by the test' } };
    return { re, ...this.#act(env, p.decode(p.Act, env.p)) };
  }

  #act(env: p.Envelope, act: p.Act) {
    const current = this.groups.get(act.g)?.rows.get(`msg ${act.msg}`);
    if (!current) return { err: { code: 'not_found', msg: 'no such card' } };
    const base = { by: owner, at: act.ts, act: env };
    const states: Record<p.Act['action'], Fields> = {
      confirm: { status: 'confirmed', ...base },
      revise: { status: 'revise', ...base, ...(act.text ? { note: act.text } : {}) },
      approve: { status: 'approved', ...base },
      deny: { status: 'denied', ...base },
      choose: {
        status: 'decided',
        ...base,
        ...(act.arg ? { choice: act.arg } : {}),
        ...(act.text ? { text: act.text } : {}),
      },
    };
    const state = states[act.action];
    const updated = p.decode(p.MsgRow, JSON.stringify({ ...current, state }));
    this.write(act.g, updated as p.Row & Fields);
    return { ok: true };
  }
}

const sign = async (payload: string) => ({ p: payload, k: 'this-browser', s: 'signature' });

/** A browser of the person, as the client holds it: a `Home` of the store, and its connection to
 *  the test home. `connect` says hello and reads; `drop` is the connection lost. */
export function browser(home: TestHome, me = user, Store = Home) {
  const store = new Store({ user: me, sign });
  /** The type of every push received, in order. */
  const seen: string[] = [];
  let drop = () => {};
  return {
    store,
    seen,
    async connect() {
      const link = store.connect();
      const connection = home.connect((frame) => {
        seen.push(frame.t);
        link.push(frame);
      });
      drop = () => {
        connection.close();
        link.offline();
      };
      const welcome = await connection.ctl.request({
        t: 'hello',
        v: 3,
        kind: 'browser',
        name: 'test',
        addrs: [],
      });
      link.online(connection.ctl, welcome as p.Welcome);
    },
    drop: () => drop(),
  };
}

export interface OwnedNode {
  home: TestHome;
  store: Home;
  device: string;
  info: p.HostInfo;
  commands: p.Envelope[];
  connect(): Promise<void>;
  drop(): void;
}

/** A computer of the person's own, as the client holds it: besides what `browser` does, it
 *  answers `host.info`, `host.workspaces` (with the wire example), `agent.set`, `pair.browser`,
 *  `host.offline` and `host.online`, and keeps those commands. */
export function ownedNode(
  name: string,
  device: string,
  overrides: Partial<p.HostInfo> = {},
): OwnedNode {
  const home = new TestHome(name);
  const store = new Home({ user, sign });
  const commands: p.Envelope[] = [];
  let drop = () => {};

  const { permissions: _permissions, ...base } = p.decode(p.HostInfo, goldens['host-info-answer']);
  const info: p.HostInfo = {
    ...base,
    device,
    name,
    ...overrides,
  };

  return {
    home,
    store,
    device,
    info,
    commands,
    async connect() {
      const link = store.connect();
      const base = home.connect(link.push);
      function handleCmd(env: p.Envelope): p.CmdResult | undefined {
        const parsed = JSON.parse(env.p) as { t: string; agent?: string; account?: AccountKind };
        if (parsed.t === 'pair.browser') {
          commands.push(env);
          return p.decode(p.CmdResult, goldens['cmd-pair-link']);
        }
        if (parsed.t === 'host.offline' || parsed.t === 'host.online') {
          commands.push(env);
          info.offline = parsed.t === 'host.offline';
          for (const entry of info.agents.flatMap((agent) => agent.groups))
            entry.state = info.offline ? 'offline' : 'dormant';
          return { re: 1, ok: true };
        }
        if (parsed.t !== 'agent.set') return undefined;
        commands.push(env);
        const entry = info.agents.find((item) => item.agent === parsed.agent);
        if (entry && parsed.account) entry.account = parsed.account;
        return { re: 1, ok: true };
      }
      // What the node answers itself; anything else is the test home's.
      const own: Record<string, (req: { env?: p.Envelope }) => unknown> = {
        'host.info': () => structuredClone(info),
        'host.workspaces': () =>
          p.decodeAnswer('host.workspaces', goldens['host-workspaces-answer']),
        cmd: (req) => req.env && handleCmd(req.env),
      };
      const ctl = {
        request: async (req: { t: string; env?: p.Envelope }) =>
          (await own[req.t]?.(req)) ?? base.ctl.request(req as never),
      } as p.Ctl;
      drop = () => {
        base.close();
        link.offline();
      };
      const welcome = (await base.ctl.request({
        t: 'hello',
        v: 3,
        kind: 'browser',
        name: 'test',
        addrs: [],
      })) as p.Welcome;
      link.online(ctl, { ...welcome, device });
    },
    drop: () => drop(),
  };
}
