// The row cache of every group this browser can read, and the lists the screens draw from it
// (planning/flows/sync.md, planning/contracts/datachannel.md section 4). A cache is never an
// authoritative copy: the home writes, the client applies rows and sends commands.
import { SvelteMap } from 'svelte/reactivity';
import { confirmSentence } from './confirm';
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

const key = (r: string, id: string | undefined) => `${r} ${id}`;
const shown = (row: p.Row) => !('gone' in row);
const isMsg = (row: p.Row): row is p.MsgRow => row.r === 'msg' && shown(row);
const isMember = (row: p.Row): row is Member => row.r === 'member' && shown(row);

/** How many messages a page of `fetch` asks for. */
const pageSize = 50;

/** Where the reading of one thread, or of the main line, has got to. */
export class Paging {
  /** The `before` of the next page, once a page was read: the lowest `pos` received. */
  before: number | undefined;
  /** Whether the home has older messages. Unknown, so true, until a page says. */
  more = $state(true);
  /** A page is on its way. */
  busy = $state(false);
  /** The home refused a page, or the read of the group: no other is asked for until the group is
   *  read again. */
  failed = $state(false);

  /** Whether a page can be asked for now. */
  get ready() {
    return this.more && !this.busy && !this.failed;
  }
}

/** Whether a card waits for this person: a pending brief they asked for, a pending approval they
 *  may give, a pending decision or confirmation put to them. `member` is their member id in the
 *  card's group. */
function waitsFor(msg: p.MsgRow, user: string, member: string | undefined): boolean {
  if (msg.state?.status !== 'pending') return false;
  if (msg.kind === 'brief') return msg.body.requester === member;
  if (msg.kind === 'approval') return msg.body.approvers.includes(user);
  if (msg.kind === 'decision' || msg.kind === 'confirm') return msg.body.to.includes(user);
  return false;
}

const approvalSubject = ({ why, command }: { why?: string; command: string }) => why ?? command;

/** What a card is about, in one line: the title of a brief, the reason or else the request of
 *  an approval, the question of a decision, the request of a confirmation. For any other message,
 *  its text. */
export function subject(group: Group, msg: p.MsgRow): string {
  if (msg.kind === 'brief') return msg.body.title;
  if (msg.kind === 'approval') return approvalSubject(msg.body);
  if (msg.kind === 'decision') return msg.body.question;
  if (msg.kind === 'confirm') return confirmSentence(group, msg);
  return msg.text;
}

/** One group as this browser holds it. */
export class Group {
  home = $state.raw<Home>()!;
  readonly g: string;
  /** Every row and deletion marker held, by type and id. A marker stays, unseen, so that an
   *  older copy of its row arriving later cannot bring the row back. */
  readonly rows = new SvelteMap<string, p.Row>();
  /** The `since` of the next `sync`: 0 at the start and from every `reset`, S at `synced`, then
   *  the `seq` of each live row applied. Rows before `synced` and pages never move it. */
  cursor = 0;
  name = $state('');
  #live = false;
  // What the answer to a read or a page must still find when it arrives: each read of the group
  // makes a new one, so an answer that was overtaken changes nothing.
  #read = {};
  // Replaced when a read of the group ends: whoever draws from a paging then holds a new one, and
  // asks for its first page from the rows that the read brought.
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
    this.members.find(
      (member): member is p.HumanMemberRow =>
        member.kind === 'human' && member.user === this.home.me.user,
    ),
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

  /** A person's display name in this group by user id; undefined when absent or left. */
  personName(user: string) {
    return this.members.find((member) => member.kind === 'human' && member.user === user)?.name;
  }

  /** A task by id; undefined when absent or deleted. */
  task(id: string | undefined) {
    const row = this.rows.get(key('task', id));
    return row && shown(row) ? (row as p.TaskRow) : undefined;
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
   *  after a `reset`, unchanged in a delta. An answer that comes too late moves nothing: the read
   *  that overtook it will. Paging starts again at `synced`, from what the read brought. */
  async sync(ctl: p.Ctl) {
    const read = (this.#read = {});
    this.#live = false;
    const answer = await ctl
      .request({ t: 'sync', g: this.g, since: this.cursor, state_only: false })
      .catch(lost);
    if (this.#late(read, ctl)) return;
    if (answer.err) {
      this.home.follow(this.g, answer.err);
      // Nothing can page the group until a read succeeds: the top says so, not that it loads.
      this.paging().failed = true;
      return;
    }
    this.cursor = answer.seq;
    this.#live = true;
    this.#paging = new Map();
    for (const row of this.#requests()) void this.#fetchRow(read, ctl, row);
  }

  /** The requests of the waiting confirm cards that are not held: a read brings every pending card,
   *  but no message of a thread. Of a task without a brief the request is its `task` message, and
   *  the task row that says the same is always held. */
  #requests() {
    const ids = this.waiting.flatMap((msg) => (msg.kind === 'confirm' && msg.re) || []);
    return [...new Set(ids)].filter((id) => !this.rows.has(key('msg', id)) && !this.task(id));
  }

  /** Fetches one message. */
  async #fetchRow(read: object, ctl: p.Ctl, row: string) {
    const answer = await ctl
      .request({ t: 'fetch', g: this.g, r: 'msg', row, before: 0, limit: 1 })
      .catch(lost);
    if (!answer.err && !this.#late(read, ctl)) this.apply(answer.rows, false);
  }

  /** Whether an answer comes too late: the group was read again since it was asked for, or the
   *  connection it was asked on is no longer the home's. */
  #late(read: object, ctl: p.Ctl) {
    return read !== this.#read || ctl !== this.home.ctl;
  }

  /** Where the first page of a thread, or of the main line, starts. A cold read brings the
   *  newest 50 of the main line and, beside them, every card still pending, however old
   *  (planning/flows/sync.md A): the oldest of the newest 50 held is the oldest message with
   *  nothing missing after it, so the page before it is the first worth asking for. What is held
   *  of a thread can have a gap anywhere: a thread is read from its newest page, `before: 0`. */
  #start(thread: string | null) {
    if (thread !== null) return 0;
    return this.main.at(-pageSize)?.pos ?? this.main.at(0)?.pos ?? 0;
  }

  /** The connection a page can be asked on now. None while the group is being read: the rows a
   *  first page would start from may be about to be dropped by a `reset`, and the page would
   *  then leave a gap that nothing fills. None while the home cannot be reached. */
  #pageable(paging: Paging) {
    return this.#live && paging.ready ? this.home.ctl : undefined;
  }

  /** Fetches the page of messages before the ones read so far, of a thread or of the main line.
   *  Does nothing while the group is being read, while a page is on its way, when the home has
   *  no more or refused one, or cannot be reached. A page that comes too late is dropped: after
   *  a reset it could bring back a row whose deletion the client can no longer be told about. */
  async older(thread: string | null = null) {
    const paging = this.paging(thread);
    const ctl = this.#pageable(paging);
    if (!ctl) return;
    const read = this.#read;
    const before = paging.before ?? this.#start(thread);
    paging.busy = true;
    const answer = await ctl
      .request({ t: 'fetch', g: this.g, thread, before, limit: pageSize })
      .catch(lost);
    paging.busy = false;
    if (this.#late(read, ctl)) return;
    if (answer.err) {
      this.home.follow(this.g, answer.err);
      paging.failed = true;
      return;
    }
    this.#page(paging, answer, before);
  }

  #page(paging: Paging, answer: p.Fetched, before: number) {
    this.apply(answer.rows, false);
    // An empty page is the end, whatever it says: nothing would move the next request on.
    paging.more = answer.rows.length > 0 && answer.more;
    // A page is newest first, so its last row has the lowest pos.
    paging.before = (answer.rows.at(-1) as p.MsgRow | undefined)?.pos ?? before;
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

/** What the person fills in to create a group (contracts/commands.md 1.1). A team group leaves
 *  `repo` out (contracts/step-2/watch-team.md section 7). */
export interface NewGroup {
  name: string;
  kind: 'project' | 'team';
  repo?: string;
  base: string;
  forge: string;
  checks: string[];
  member_name: string;
  templates?: p.GroupCreate['templates'];
}

/** One connection to a node, from before it exists until it is gone. */
export interface Link {
  /** The connection's push handler. */
  push(frame: p.Push): void;
  /** The node answered `hello` on this connection, which is the home's from now on: every group
   *  is read again from its cursor. The last connection to call this wins, so the code that owns
   *  the connections calls it only for the one that is to become the home's: for a probe of a
   *  better path, when it takes over, not while the connection it may replace still serves. */
  online(ctl: p.Ctl, welcome: p.Welcome): void;
  /** This connection is gone. The rows stay. */
  offline(): void;
}

/** One node this browser connects to, with the groups homed on it. The code that owns the
 *  connections makes one `Home` per node and one `connect()` per connection. */
export class Home {
  hostInfo = $state.raw<p.HostInfoPush>();
  readonly terminalSizes = new SvelteMap<string, p.PTYResize>();
  readonly me: Me;
  /** The connection, or undefined while the node cannot be reached: its groups are then shown
   *  from the rows held, read-only. */
  ctl = $state.raw<p.Ctl>();
  /** The groups homed here that this person may read, as the node last listed them. */
  groups = $state.raw<Group[]>([]);
  /** The node's device id and name, and whether this person owns it: only then can a group be
   *  created here. */
  device = $state('');
  name = $state('');
  owner = $state(false);
  // The connection that is the home's now: the last one to come online, until it is gone.
  #link: Link | undefined;
  #arrivals = new Set<string>();
  onmove?: (g: string, device: string) => void;

  constructor(me: Me) {
    this.me = me;
  }

  /** A connection to the node that is about to be made. Only the last one to call `online`
   *  counts: what an earlier one still pushes is dropped, and its going leaves the home online. */
  connect(): Link {
    const link: Link = {
      push: (frame) => {
        if (this.#link === link) this.#push(frame);
      },
      online: (ctl, welcome) => {
        this.#link = link;
        this.ctl = ctl;
        this.device = welcome.device;
        this.name = welcome.name;
        this.owner = welcome.owner;
        this.#list(welcome.groups);
        this.#readAll(this.groups);
      },
      offline: () => {
        if (this.#link !== link) return;
        this.#link = undefined;
        this.ctl = undefined;
      },
    };
    return link;
  }

  /** A push of the home's connection. A group new in a `groups` push is read at once; rows for a
   *  group that is not listed are dropped. */
  #push(frame: p.Push) {
    this.#hostPush(frame);
    if (frame.t === 'groups') this.#readAll(this.#list(frame.groups));
    if (frame.t === 'rows') this.#find(frame.g)?.apply(frame.rows);
    if (frame.t === 'reset') this.#find(frame.g)?.reset();
  }

  #hostPush(frame: p.Push) {
    if (frame.t === 'host.info') this.hostInfo = frame;
    if (frame.t === 'pty.resize') this.terminalSizes.set(`${frame.g}:${frame.ai}`, frame);
  }

  /** Sends a signed command and answers what the node answered (contracts/commands.md). `n` is
   *  its nonce: a command sent again with the same one has no second effect. */
  async command(fields: { t: string; g: string; [field: string]: unknown }, n = newId()) {
    const { t, g, ...rest } = fields;
    const env = await this.me.sign(JSON.stringify({ t, n, ts: Date.now(), g, ...rest }));
    const ctl = this.ctl;
    const answer = await (ctl ? ctl.request({ t: 'cmd', env }).catch(lost) : lost());
    if (ctl === this.ctl && answer.err) this.follow(g, answer.err);
    return answer;
  }

  follow(g: string, error: p.ErrorResponse['err']) {
    if (error.code === 'moved' && error.home && error.home !== this.device) {
      this.onmove?.(g, error.home);
    }
  }

  move(g: string, destination: Home) {
    const group = this.#find(g);
    if (!group) return;
    this.groups = this.groups.filter((item) => item.g !== g);
    if (!destination.#find(g)) destination.#arrivals.add(g);
    destination.groups = [...destination.groups.filter((item) => item.g !== g), group];
    group.home = destination;
    if (destination.ctl) void group.sync(destination.ctl);
  }

  /** Makes this node the home of a new group, whose id is the command's nonce. Answers with that
   *  id and what the node said; the group itself arrives in a `groups` push. */
  async create(group: NewGroup) {
    const g = newId();
    return { g, answer: await this.command({ t: 'group', g, op: 'create', ...group }, g) };
  }

  #find(g: string) {
    return this.groups.find((group) => group.g === g);
  }

  /** Reads groups on the home's connection, which a frame of it or its `online` vouches for. */
  #readAll(groups: Group[]) {
    const ctl = this.ctl!;
    for (const group of groups) void group.sync(ctl);
  }

  /** Replaces the list. A group listed before keeps its rows; the new ones are returned. */
  #list(listed: p.Welcome['groups']) {
    const fresh: Group[] = [];
    const pending = this.groups;
    const next = listed.map(({ g, name }) => {
      let group = this.#find(g);
      if (!group) fresh.push((group = new Group(this, g)));
      else if (this.#arrivals.has(g)) fresh.push(group);
      this.#arrivals.delete(g);
      group.name = name;
      return group;
    });
    this.groups = [...next, ...pending.filter((group) => this.#arrivals.has(group.g))];
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
