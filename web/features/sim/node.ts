// One computer of the demo world, as a browser reaches it: the home of the groups it holds
// (TestHome and ActHome of features/home.ts answer `sync`, `fetch`, `post`, `act` and the group
// commands as planning/flows/sync.md says) and the host of the AIs that run on it, which answers
// `hello`, `host.info`, `host.workspaces`, `watch.start` and the host commands. What its AIs do is the director's.
import type * as p from '../../src/lib/proto';
import { ActHome } from '../home';
import { fetchKind } from './memory';
import type { Fields } from './rows';

/** A CLI found on the computer, as `host.info` lists it without its groups. */
export type Agent = Omit<p.HostInfo['agents'][number], 'groups'>;

/** What the world does for a node: the parts of a command that reach beyond the node. */
export interface NodeWorld {
  me: string;
  /** Answers a host command, or a home command that the scripts apply. */
  command(node: SimNode, payload: Fields): Promise<object | undefined> | object | undefined;
  /** A command the node applied as a test home does, after its answer. */
  applied(node: SimNode, payload: Fields): void;
  info(node: SimNode): p.HostInfoPush;
  /** The folders a CLI of the node was used in, newest first. */
  workspaces(node: SimNode, agent: string): object;
  watchStart(node: SimNode, g: string, member: string): object;
  channel(node: SimNode, label: string): p.PtyChannel | undefined;
  heard(node: SimNode, frame: p.Devlog | p.PTYResize): void;
  /** A row was written: the world is saved soon after. */
  changed(): void;
}

/** What a computer is, and whose. */
export interface NodeSpec {
  device: string;
  name: string;
  os: string;
  owner: string;
  agents: Agent[];
}

type Request = { t: string; env?: p.Envelope; g?: string; member?: string; agent?: string };
type Base = (req: Fields) => Promise<unknown>;

/** One connection of a browser to the node. */
export interface Line {
  request(req: Request): Promise<unknown>;
  send(frame: p.Devlog | p.PTYResize): void;
  channel(label: string): p.PtyChannel | undefined;
  close(): void;
}

const gOf = (req: Request) => (req.env ? (JSON.parse(req.env.p) as Fields).g : req.g) as string;

export class SimNode extends ActHome {
  readonly device: string;
  readonly os: string;
  readonly owner: string;
  agents: Agent[];
  /** The person stopped every AI of this computer (host.offline). */
  offline = false;
  /** The computer cannot be reached: turned off, or its network is down. */
  down = false;
  /** The browser has paired with it: it is in the person's device log. */
  paired = false;
  /** Groups that moved away, by the device id of their new home (contracts/step-2/export.md). */
  readonly moved = new Map<string, string>();
  #pushes = new Set<(frame: p.Push) => void>();
  #world: NodeWorld;

  constructor(spec: NodeSpec, world: NodeWorld) {
    super(spec.name);
    this.device = spec.device;
    this.os = spec.os;
    this.owner = spec.owner;
    this.agents = spec.agents;
    this.#world = world;
  }

  /** A browser connects: what it asks is answered as this computer would. */
  open(push: (frame: p.Push) => void): Line {
    const base = this.connect(push);
    this.#pushes.add(push);
    const request = base.ctl.request as unknown as Base;
    return {
      request: (req) => this.#route(request, req),
      send: (frame) => this.#world.heard(this, frame),
      channel: (label) => this.#world.channel(this, label),
      close: () => {
        this.#pushes.delete(push);
        base.close();
      },
    };
  }

  /** Pushes a frame to every browser connected to the node. */
  broadcast(frame: p.Push) {
    for (const push of this.#pushes) push(structuredClone(frame));
  }

  /** The groups this node homes, as `welcome` and `groups` list them. */
  listed() {
    return [...this.groups].map(([g, { name, seq }]) => ({ g, name, seq }));
  }

  /** The node's list of groups changed: every browser hears it. */
  relist() {
    this.broadcast({ t: 'groups', groups: this.listed() });
  }

  override write(g: string, row: Parameters<ActHome['write']>[1]) {
    const written = super.write(g, row);
    this.#world.changed();
    return written;
  }

  async #route(base: Base, req: Request): Promise<unknown> {
    if (req.t === 'hello') return this.#welcome(await base(req));
    if (req.t === 'host.info') return { ...this.#world.info(this), re: 0, ok: true };
    if (req.t === 'host.workspaces') return { re: 0, ...this.#world.workspaces(this, req.agent!) };
    if (req.t === 'watch.start')
      return { re: 0, ...this.#world.watchStart(this, req.g!, req.member!) };
    const moved = this.moved.get(gOf(req));
    if (moved) return { re: 0, err: { code: 'moved', msg: 'the group moved', home: moved } };
    if (req.t === 'fetch' && 'kind' in req) return fetchKind(this, req as Fields);
    return req.t === 'cmd' ? this.#command(base, req) : base(req);
  }

  #welcome(answer: unknown) {
    const owner = this.owner === this.#world.me;
    return { ...(answer as Fields), v: 3, device: this.device, owner, groups: this.listed() };
  }

  async #command(base: Base, req: Request) {
    const payload = JSON.parse(req.env!.p) as Fields;
    const handled = await this.#world.command(this, payload);
    if (handled) return { re: 0, ...handled };
    const answer = (await base(req)) as { err?: unknown };
    if (!answer.err) this.#world.applied(this, payload);
    return answer;
  }
}
