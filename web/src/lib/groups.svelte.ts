// The row cache of every group this browser can read, and the lists the screens draw from it
// (planning/flows/sync.md, planning/contracts/datachannel.md section 4). A cache is never an
// authoritative copy: the home writes, the client applies rows and sends commands.
import { SvelteMap } from 'svelte/reactivity';
import { encode } from './identity';
import type * as p from './proto';

/** The person at this browser: their user id, and this device, which signs their commands. */
export interface Me {
  user: string;
  sign(p: string): Promise<p.Envelope>;
}

export type Member = p.HumanMemberRow | p.AIMemberRow;

/** The answer to a request whose connection was lost before the node answered. `lost` is no code
 *  of the contract: the client words it as a home that cannot be reached. */
export const lost = (): p.ErrorResponse => ({
  re: 0,
  err: { code: 'lost', msg: 'connection lost' },
});

/** A new random id: a command's nonce, which for a `post` is also the message's id and for a
 *  group's `create` the group's (contracts/envelope.md section 1). */
export const newId = () => encode(crypto.getRandomValues(new Uint8Array(16)));

const key = (r: string, id: string) => `${r} ${id}`;
const shown = (row: p.Row) => !('gone' in row);
const isMsg = (row: p.Row): row is p.MsgRow => row.r === 'msg' && shown(row);
const isMember = (row: p.Row): row is Member => row.r === 'member' && shown(row);

/** How many messages a page of `fetch` asks for. */
const pageSize = 50;

/** Where the reading of one thread, or of the main line, has got to. */
export class Paging {
  /** The `before` of the next page: 0 asks for the newest, then the lowest `pos` received. */
  before = 0;
  /** Whether the home has older messages. Unknown, so true, until a page says. */
  more = $state(true);
  /** A page is on its way. */
  busy = $state(false);
}

/** Whether a card waits for this person: a pending brief they asked for, a pending approval they
 *  may give, a pending decision put to them. `member` is their member id in the card's group. */
function waitsFor(msg: p.MsgRow, user: string, member: string | undefined): boolean {
  if (msg.state?.status !== 'pending') return false;
  if (msg.kind === 'brief') return msg.body.requester === member;
  if (msg.kind === 'approval') return msg.body.approvers.includes(user);
  return msg.kind === 'decision' && msg.body.to.includes(user);
}

/** What a card is about, in one line: the title of a brief, the reason or else the request of
 *  an approval, the question of a decision. For any other message, its text. */
export function subject(msg: p.MsgRow): string {
  if (msg.kind === 'brief') return msg.body.title;
  if (msg.kind === 'approval') return msg.body.why ?? msg.body.command;
  if (msg.kind === 'decision') return msg.body.question;
  return msg.text;
}

/** One group as this browser holds it. */
export class Group {
  readonly home: Home;
  readonly g: string;
  /** Every row and deletion marker held, by type and id. A marker stays, unseen, so that an
   *  older copy of its row arriving later cannot bring the row back. */
  readonly rows = new SvelteMap<string, p.Row>();
  /** The `since` of the next `sync`: 0 at the start and from every `reset`, S at `synced`, then
   *  the `seq` of each live row applied. Rows before `synced` and pages never move it. */
  cursor = 0;
  name = $state('');
  #live = false;
  // Replaced, not cleared, when the group is read again: a page still on its way from before
  // then changes a paging that nobody holds any more.
  #paging = $state.raw(new Map<string | null, Paging>());

  constructor(home: Home, g: string) {
    this.home = home;
    this.g = g;
  }

  /** Every message held, in the order of creation. */
  #msgs = $derived([...this.rows.values()].filter(isMsg).sort((a, b) => a.pos - b.pos));
  /** The main line: messages outside a thread, and cards raised from one. */
  main = $derived(this.#msgs.filter((msg) => msg.thread === null || msg.main));
  /** The members that have not left, by name. */
  members = $derived(
    [...this.rows.values()].filter(isMember).sort((a, b) => a.name.localeCompare(b.name)),
  );
  /** The person at this browser as a member of the group. */
  me = $derived(
    this.members.find((member) => member.kind === 'human' && member.user === this.home.me.user),
  );
  /** The cards that cannot move until this person acts, oldest first. */
  waiting = $derived(this.#msgs.filter((msg) => waitsFor(msg, this.home.me.user, this.me?.id)));
  /** The group's own row, once it has arrived. */
  info = $derived.by(() => this.rows.get(key('group', this.g)) as p.GroupRow | undefined);
  /** When the main line last got a message; before the first one, when the group was made. */
  latest = $derived(this.main.at(-1)?.ts ?? this.info?.created ?? 0);

  /** The messages of one task's thread, in the order of creation. */
  thread(task: string) {
    return this.#msgs.filter((msg) => msg.thread === task);
  }

  /** A member by id; undefined for one that left, and for `system`. */
  member(id: string) {
    return this.members.find((member) => member.id === id);
  }

  /** The paging of a thread, or of the main line (null). */
  paging(thread: string | null = null) {
    const known = this.#paging.get(thread);
    if (known) return known;
    const fresh = new Paging();
    this.#paging.set(thread, fresh);
    return fresh;
  }

  /** Takes rows in. A row replaces the copy held only with a higher `seq`; an equal or lower one
   *  changes nothing and does not move the cursor. */
  apply(rows: p.Row[], live = this.#live) {
    for (const row of rows) {
      const held = this.rows.get(key(row.r, row.id));
      if (held && held.seq >= row.seq) continue;
      this.rows.set(key(row.r, row.id), row);
      if (live) this.cursor = row.seq;
    }
  }

  /** The home said to drop the cache: the cursor returns to 0 with it. */
  reset() {
    this.rows.clear();
    this.cursor = 0;
  }

  /** Reads the group from the cursor on and subscribes to its changes. Until `synced` the rows
   *  that arrive are not live, so a read that breaks off leaves the cursor where it was: at 0
   *  after a `reset`, unchanged in a delta. Paging starts again with each read. */
  async sync(ctl: p.Ctl) {
    this.#live = false;
    this.#paging = new Map();
    const answer = await ctl
      .request({ t: 'sync', g: this.g, since: this.cursor, state_only: false })
      .catch(lost);
    if ('err' in answer) return;
    this.cursor = answer.seq;
    this.#live = true;
  }

  /** Fetches the page of messages before the ones read so far, of a thread or of the main line.
   *  Does nothing while a page is on its way, when the home has no more, or cannot be reached.
   *  A page that fails ends the paging until the group is read again. */
  async older(thread: string | null = null) {
    const paging = this.paging(thread);
    const ctl = this.home.ctl;
    if (!ctl || paging.busy || !paging.more) return;
    paging.busy = true;
    const answer = await ctl
      .request({ t: 'fetch', g: this.g, thread, before: paging.before, limit: pageSize })
      .catch(lost);
    paging.busy = false;
    const page = 'err' in answer ? { rows: [], more: false } : answer;
    this.apply(page.rows, false);
    // An empty page is the end, whatever it says: nothing would move the next request on.
    paging.more = page.rows.length > 0 && page.more;
    // A page is newest first, so its last row has the lowest pos; an empty page changes nothing.
    paging.before = (page.rows.at(-1) as p.MsgRow | undefined)?.pos ?? paging.before;
  }

  /** Posts a text message to the main line. The row comes back from the home like any other. */
  post(text: string, mentions: string[]) {
    return this.home.command({
      t: 'post',
      g: this.g,
      kind: 'text',
      thread: null,
      main: false,
      text,
      mentions,
      re: null,
      body: {},
    });
  }
}

/** What the person fills in to create a project group (contracts/commands.md 1.1). */
export interface NewGroup {
  name: string;
  repo: string;
  base: string;
  forge: string;
  checks: string[];
  member_name: string;
}

/** One node this browser connects to, with the groups homed on it. The code that owns the
 *  connection makes one `Home` per node, gives `push` to each connection as its push handler,
 *  calls `online` after every `welcome` and `offline` when the connection is gone. */
export class Home {
  readonly me: Me;
  /** The connection, or undefined while the node cannot be reached: its groups are then shown
   *  from the rows held, read-only. */
  ctl = $state.raw<p.Ctl>();
  /** The groups homed here that this person may read, as the node last listed them. */
  groups = $state.raw<Group[]>([]);
  /** The node's name, and whether this person owns it: only then can a group be created here. */
  name = $state('');
  owner = $state(false);

  constructor(me: Me) {
    this.me = me;
  }

  /** The node answered `hello` on a new connection: every group is read again from its cursor. */
  online(ctl: p.Ctl, welcome: p.Welcome) {
    this.ctl = ctl;
    this.name = welcome.name;
    this.owner = welcome.owner;
    this.#list(welcome.groups);
    this.#read(this.groups, ctl);
  }

  /** The connection is gone. The rows stay. */
  offline() {
    this.ctl = undefined;
  }

  /** A push of this node's connection, which is online. A group new in a `groups` push is read
   *  at once; rows for a group that is not listed are dropped. */
  push = (frame: p.Push) => {
    if (frame.t === 'groups') this.#read(this.#list(frame.groups), this.ctl!);
    if (frame.t === 'rows') this.#find(frame.g)?.apply(frame.rows);
    if (frame.t === 'reset') this.#find(frame.g)?.reset();
  };

  /** Sends a signed command and answers what the node answered (contracts/commands.md). `n` is
   *  its nonce: a command sent again with the same one has no second effect. */
  async command(fields: { t: string; g: string; [field: string]: unknown }, n = newId()) {
    const { t, g, ...rest } = fields;
    const env = await this.me.sign(JSON.stringify({ t, n, ts: Date.now(), g, ...rest }));
    const ctl = this.ctl;
    return ctl ? ctl.request({ t: 'cmd', env }).catch(lost) : lost();
  }

  /** Makes this node the home of a new project group, whose id is the command's nonce. Answers
   *  with that id and what the node said; the group itself arrives in a `groups` push. */
  async create(group: NewGroup) {
    const g = newId();
    const fields = { t: 'group', g, op: 'create', ...group, kind: 'project' };
    return { g, answer: await this.command(fields, g) };
  }

  #find(g: string) {
    return this.groups.find((group) => group.g === g);
  }

  #read(groups: Group[], ctl: p.Ctl) {
    for (const group of groups) void group.sync(ctl);
  }

  /** Replaces the list. A group listed before keeps its rows; the new ones are returned. */
  #list(listed: p.Welcome['groups']) {
    const fresh: Group[] = [];
    this.groups = listed.map(({ g, name }) => {
      let group = this.#find(g);
      if (!group) fresh.push((group = new Group(this, g)));
      group.name = name;
      return group;
    });
    return fresh;
  }
}

/** Every node this browser connects to. */
export const homes: Home[] = $state([]);

/** Every group this person may read, the one with the newest main-line message first. */
export const recent = () =>
  homes.flatMap((home) => home.groups).sort((a, b) => b.latest - a.latest);

/** A group by id. */
export const find = (g: string | undefined) =>
  homes.flatMap((home) => home.groups).find((group) => group.g === g);
