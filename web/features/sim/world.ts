// The demo world: the computers, the groups they home, the people and AIs in them, and the clock
// on which the AIs act. It is kept in this browser's localStorage, so that a reload finds the
// world as it was; the work an AI was doing at that moment starts again from its task's row.
import type * as p from '../../src/lib/proto';
import { SimNode, type Agent, type NodeSpec, type NodeWorld } from './node';
import type { Fields, Row } from './rows';

export type Mode = 'full' | 'fresh' | 'pairfail';

/** The address of a mode of the demo. */
export const address = (mode: Mode) =>
  `${location.pathname}?demo${mode === 'full' ? '' : `=${mode}`}`;

/** What the scripts remember beside the rows. */
export interface Memory {
  /** Per task: the steps of its script already taken. */
  flags: Record<string, string[]>;
  /** Per AI: the messages it was sent while busy, which it answers at its next pause. */
  queue: Record<string, { g: string; msg: string }[]>;
  /** One-off events that happened. */
  done: string[];
  /** Open invitations, by secret. */
  invites: Record<string, { g: string; expires: number }>;
  /** Each group's invitation code, as a home stores it in `invites` (contracts/storage.md 2):
   *  the secret, the admin who made it and when it ends. One past its end counts as none. */
  codes: Record<string, { code: string; by: string; expires: number }>;
  /** Watch grants: who may watch which AI. */
  grants: { g: string; member: string; user: string }[];
  /** Open takeovers, by `g:member`: the task the terminal is on, if any. */
  takeovers: Record<string, { task?: string }>;
}

const emptyMemory = (): Memory => ({
  flags: {},
  queue: {},
  done: [],
  invites: {},
  codes: {},
  grants: [],
  takeovers: {},
});

interface SavedNode {
  device: string;
  groups: [string, { name: string; seq: number; rows: [string, Row][] }][];
  moved: [string, string][];
  agents: Agent[];
  offline: boolean;
  paired: boolean;
}

/** The world as localStorage holds it. A world of another version is not read: its rows may
 *  have another shape. */
export interface Saved {
  v: 2;
  mode: Mode;
  me: string;
  nodes: SavedNode[];
  memory: Memory;
}

export const storageKey = 'sd-demo-world';

/** The parts of the world that answer for its nodes; the director and the host scripts. */
export interface Hands {
  command(node: SimNode, payload: Fields): Promise<object | undefined> | object | undefined;
  applied(node: SimNode, payload: Fields): void;
  info(node: SimNode): p.HostInfoPush;
  workspaces(node: SimNode, agent: string): object;
  watchStart(node: SimNode, g: string, member: string): object;
  channel(node: SimNode, label: string): p.PtyChannel | undefined;
  heard(node: SimNode, frame: p.Devlog | p.PTYResize): void;
}

export class World implements NodeWorld {
  readonly nodes = new Map<string, SimNode>();
  readonly me: string;
  readonly mode: Mode;
  memory: Memory = emptyMemory();
  /** How long a step of the scripts takes, as a factor of the times they name. */
  pace = 1;
  hands!: Hands;
  #timers = new Set<ReturnType<typeof setTimeout>>();
  #saving: ReturnType<typeof setTimeout> | undefined;
  #storage: Storage | undefined;

  constructor(me: string, mode: Mode, storage?: Storage) {
    this.me = me;
    this.mode = mode;
    this.#storage = storage;
  }

  add(spec: NodeSpec) {
    const node = new SimNode(spec, this);
    this.nodes.set(spec.device, node);
    return node;
  }

  node(device: string) {
    return this.nodes.get(device);
  }

  /** The computer that homes a group now. */
  home(g: string) {
    return [...this.nodes.values()].find((node) => node.groups.has(g));
  }

  /** `group import` on `to`: the group that was exported from its home is its home's now. Its
   *  invitation code ended with the move (contracts/step-2/export.md section 3). */
  arrive(g: string, to: SimNode) {
    delete this.memory.codes[g];
    const from = this.home(g)!;
    const held = from.groups.get(g)!;
    from.groups.delete(g);
    to.groups.set(g, held);
    this.change(g, 'group', g, { home: to.device });
    to.relist();
  }

  /** Every row of a group. */
  rows(g: string): Row[] {
    const held = [...(this.home(g)?.groups.get(g)?.rows.values() ?? [])] as unknown as Row[];
    return held.filter((row) => !row.gone);
  }

  find<T = Row>(g: string, r: string, id: string | undefined): T | undefined {
    const found = this.home(g)?.groups.get(g)?.rows.get(`${r} ${id}`);
    return found && !found.gone ? (found as T) : undefined;
  }

  write(g: string, row: Row) {
    return this.home(g)!.write(g, row as never) as unknown as Row;
  }

  /** Changes some fields of a row that exists. */
  change(g: string, r: string, id: string, fields: Fields) {
    const held = this.find(g, r, id);
    return held && this.write(g, { ...held, ...fields } as Row);
  }

  /** The group's invitation code while it is open. */
  code(g: string) {
    const held = this.memory.codes[g];
    return held && held.expires > Date.now() ? held : undefined;
  }

  /** Every group of every computer, with its home. */
  groups() {
    return [...this.nodes.values()].flatMap((node) => [...node.groups.keys()]);
  }

  /** Runs `fn` after `ms` milliseconds of the scripts' clock. */
  after(ms: number, fn: () => void) {
    const timer = setTimeout(() => {
      this.#timers.delete(timer);
      fn();
    }, ms * this.pace);
    this.#timers.add(timer);
  }

  /** Stops every script: nothing more happens in this world. */
  stop() {
    for (const timer of this.#timers) clearTimeout(timer);
    this.#timers.clear();
    clearTimeout(this.#saving);
  }

  // NodeWorld: what the nodes ask of the world goes to its hands.
  command(node: SimNode, payload: Fields) {
    return this.hands.command(node, payload);
  }
  applied(node: SimNode, payload: Fields) {
    this.hands.applied(node, payload);
  }
  info(node: SimNode) {
    return this.hands.info(node);
  }
  workspaces(node: SimNode, agent: string) {
    return this.hands.workspaces(node, agent);
  }
  watchStart(node: SimNode, g: string, member: string) {
    return this.hands.watchStart(node, g, member);
  }
  channel(node: SimNode, label: string) {
    return this.hands.channel(node, label);
  }
  heard(node: SimNode, frame: p.Devlog | p.PTYResize) {
    this.hands.heard(node, frame);
  }

  changed() {
    if (!this.#storage || this.#saving) return;
    this.#saving = setTimeout(() => {
      this.#saving = undefined;
      this.save();
    }, 300);
  }

  save() {
    try {
      this.#storage?.setItem(storageKey, JSON.stringify(this.saved()));
    } catch {
      // A full or blocked storage keeps the world in memory only.
    }
  }

  saved(): Saved {
    const nodes = [...this.nodes.values()].map((node) => ({
      device: node.device,
      groups: [...node.groups].map(([g, held]) => [g, { ...held, rows: [...held.rows] }]),
      moved: [...node.moved],
      agents: node.agents,
      offline: node.offline,
      paired: node.paired,
    })) as SavedNode[];
    return { v: 2, mode: this.mode, me: this.me, nodes, memory: this.memory };
  }

  /** Puts back what a saved world held into this one, whose computers are already added. */
  restore(saved: Saved) {
    for (const held of saved.nodes) {
      const node = this.nodes.get(held.device)!;
      node.groups.clear();
      for (const [g, group] of held.groups)
        node.groups.set(g, { ...group, rows: new Map(group.rows) as never });
      for (const [g, to] of held.moved) node.moved.set(g, to);
      Object.assign(node, { agents: held.agents, offline: held.offline, paired: held.paired });
    }
    // A group exported when the world was saved and not yet imported is imported now.
    for (const node of this.nodes.values())
      for (const [g, device] of node.moved)
        if (node.groups.has(g)) this.arrive(g, this.nodes.get(device)!);
    this.memory = { ...emptyMemory(), ...saved.memory };
  }
}
