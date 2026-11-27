// What happens in the demo world outside the browser, which the demo panel lets the person set
// off: a computer loses its network and comes back, the person moves a group to their other
// computer on the command line (contracts/step-2/export.md section 3), Mina takes her terminal or
// gives it back, Mina asks one of the person's AIs for something, and a reminder arrives as it
// would while the page is closed.
import type * as p from '../../src/lib/proto';
import { words } from './content';
import type { Director } from './director';
import { devices, ids } from './ids';
import { remind } from './reminders';
import { msg, type Row } from './rows';
import { refresh } from './session';
import type { World } from './world';

export class Events {
  constructor(
    readonly world: World,
    readonly director: Director,
  ) {}

  /** A computer loses its network, or gets it back: its AIs are offline meanwhile in every
   *  group (a closed host connection, contracts/commands.md 1.9). */
  setDown(device: string, down: boolean) {
    const node = this.world.node(device)!;
    node.down = down;
    const { host } = this.director;
    for (const g of this.world.groups())
      for (const each of host.ais(g).filter((ai) => ai.host === device))
        host.setAI(g, each.id, {
          state: down || node.offline ? 'offline' : this.#idle(g, each.task),
        });
    refresh();
    this.world.changed();
    if (!down) this.director.resume();
  }

  #idle(g: string, task: string | undefined) {
    return this.director.host.task(g, task)?.state === 'active' ? 'working' : 'waiting';
  }

  /** `solodawn group export <g> --to <other>` on its home, then `group import` on the other: the
   *  old home answers `moved` from now on, the new one lists the group a moment later. The group
   *  stays in the world meanwhile, so its AIs are still counted on their computers. */
  move(g: string) {
    const from = this.world.home(g)!;
    const to = [...this.world.nodes.values()].find(
      (node) => node !== from && node.owner === this.world.me && node.paired,
    )!;
    from.moved.set(g, to.device);
    to.moved.delete(g);
    from.relist();
    this.world.changed();
    this.world.after(2500, () => this.world.arrive(g, to));
    return to.name;
  }

  /** Where a group could move: the person's other paired computer. */
  movable(g: string) {
    const from = this.world.home(g);
    return (
      Boolean(from) &&
      [...this.world.nodes.values()].some(
        (node) => node !== from && node.owner === this.world.me && node.paired,
      )
    );
  }

  /** Mina takes over her AI's terminal, or gives it back. */
  minaTerminal(taken: boolean) {
    this.director.host.setAI(ids.site, ids.minaAi, { state: taken ? 'takeover' : 'waiting' });
    if (!taken)
      this.world.memory.grants = this.world.memory.grants.filter(
        (each) => each.member !== ids.minaAi,
      );
    this.world.node(devices.minaPc)?.broadcast(this.world.info(this.world.node(devices.minaPc)!));
  }

  /** Mina mentions the person's AI: its owner is asked first (contracts/step-2/confirm.md). */
  minaAsks() {
    const g = ids.site;
    const { host } = this.director;
    const ask = host.post('text', { g, by: ids.mina, text: words.minaAsk, mentions: [ids.claude] });
    const body = { what: 'direct', ai: ids.claude, to: [this.world.me] };
    this.world.write(
      g,
      msg('confirm', { g, by: 'system', main: true, re: ask.id }, body, { status: 'pending' }),
    );
  }

  /** A reminder of the newest card that waits for the person, or of the newest message that
   *  mentions them, as the push service would bring it to a closed page. */
  remind(lang: 'en' | 'zh-CN') {
    const found = this.#newest();
    if (!found) return false;
    const { g, row } = found;
    const group = this.world.home(g)!.groups.get(g)!.name;
    const who = (this.world.find<Row>(g, 'member', row.by as string)?.name as string) ?? '秘书';
    const kind = row.kind === 'text' ? 'mention' : (row.kind as string);
    const reminder: p.Reminder = { g, msg: row.id, group, kind, who, title: this.#title(row) };
    return remind(reminder, lang);
  }

  #newest() {
    const me = this.world.me;
    const waits = (row: Row) =>
      (row.state as { status?: string } | null)?.status === 'pending' &&
      JSON.stringify(row.body).includes(me);
    const all = this.world.groups().flatMap((g) =>
      this.world
        .rows(g)
        .filter((row) => row.r === 'msg')
        .map((row) => ({ g, row })),
    );
    const cards = all.filter(({ row }) => waits(row));
    const mentions = all.filter(({ row }) => (row.mentions as string[]).includes(ids.me));
    return [...cards, ...mentions].sort((a, b) => (b.row.ts as number) - (a.row.ts as number))[0];
  }

  #title(row: Row): string | undefined {
    const body = row.body as { title?: string; question?: string; why?: string };
    return body.title ?? body.question ?? body.why;
  }

  /** The person's own computers and whether the browser reaches each. */
  computers() {
    return [...this.world.nodes.values()].filter((node) => node.owner === this.world.me);
  }

  /** The first of the person's computers that the browser has not paired with yet. */
  unpaired() {
    return this.computers().find((node) => !node.paired);
  }
}
