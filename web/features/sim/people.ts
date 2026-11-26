// The people of the demo world and what they do (contracts/step-2/people.md, confirm.md,
// watch-team.md): an invitation that Leo takes a moment later, roles, removing a person or an AI,
// changing a group or an AI, and the `confirm` cards by which an AI's owner lets it work for someone
// else or lets someone watch its terminal. Mina is the other person; she answers by herself.
import { newSecret, shown } from '../../src/lib/pairing';
import { words } from './content';
import { devices, ids, users } from './ids';
import { person, type Fields, type Row } from './rows';
import { Scene, type AI, type Msg } from './scene';

type Answer = { ok: true; [field: string]: unknown } | { err: { code: string; msg: string } };

const ok: Answer = { ok: true };
const refuse = (code: string): Answer => ({ err: { code, msg: `refused: ${code}` } });
const aiFields = [
  'name',
  'account',
  'allow',
  'approvers',
  'model',
  'effort',
  'tier',
  'skills',
  'tool',
];
const groupFields = ['name', 'checks', 'pass', 'pass_foundation', 'limits', 'templates'];

/** The address of the client in demo mode, with a fragment. */
export const here = (fragment: string) => `${location.origin}${location.pathname}?demo${fragment}`;

export class People extends Scene {
  #member(g: string, user: string) {
    return this.world.rows(g).find((row) => row.r === 'member' && row.user === user);
  }

  /** Whether an AI has work it cannot leave: it does or reviews a task that is not final. */
  #engaged(g: string, ai: string) {
    return this.tasksOf(g).some(
      (t) => !['merged', 'cancelled'].includes(t.state) && (t.assignee === ai || t.reviewer === ai),
    );
  }

  /** `invite` (people.md 2.1): a link; Leo opens it a moment later and joins. */
  invite(payload: Fields): Answer {
    const g = payload.g as string;
    const secret = newSecret(crypto);
    const expires = Date.now() + 600_000;
    this.world.memory.invites[secret] = { g, expires };
    const home = this.world.home(g)!.device;
    const link = here(`#join=${shown(secret)}&g=${g}&home=${home}`);
    this.world.after(8000, () => this.#leoJoins(g, secret));
    return { ok: true, link, expires };
  }

  #leoJoins(g: string, secret: string) {
    // The invitation stays in its room, so that the link still opens in this browser.
    if (this.#member(g, users.leo)) return;
    this.world.write(g, person(g, ids.leo, users.leo, 'Leo', 'member'));
    this.post(
      'system',
      { g, by: 'system' },
      { code: 'member_joined', member: ids.leo, by: ids.me },
    );
    this.world.after(2500, () => this.post('text', { g, by: ids.leo, text: words.leoHello }));
  }

  /** `join` from this browser: the person is a member of the group already. */
  join(payload: Fields): Answer {
    return this.#member(payload.g as string, this.world.me)
      ? refuse('conflict')
      : refuse('forbidden');
  }

  /** `member set` and `member remove`, of a person or of an AI. */
  member(payload: Fields): Answer {
    const g = payload.g as string;
    const target = this.world.find<Row>(g, 'member', payload.id as string);
    if (!target) return refuse('not_found');
    if (payload.op === 'remove')
      return target.kind === 'ai' ? this.removeAI(g, target as AI) : this.#removePerson(g, target);
    return target.kind === 'ai'
      ? this.#setAI(g, target as AI, payload)
      : this.#setRole(g, target, payload);
  }

  #setRole(g: string, target: Row, payload: Fields): Answer {
    if (target.role === 'owner') return refuse('forbidden');
    if (!['admin', 'member'].includes(payload.role as string)) return refuse('bad_request');
    this.world.write(g, { ...target, role: payload.role });
    return ok;
  }

  #setAI(g: string, target: AI, payload: Fields): Answer {
    const changes = Object.fromEntries(
      aiFields.filter((key) => key in payload).map((key) => [key, payload[key]]),
    );
    const taken = this.world
      .rows(g)
      .some((row) => row.r === 'member' && row.id !== target.id && row.name === changes.name);
    if (taken) return refuse('conflict');
    this.world.write(g, { ...target, ...changes } as Row);
    return ok;
  }

  /** `member remove` of an AI; the secretary's going clears `group.secretary`. */
  removeAI(g: string, ai: AI): Answer {
    if (this.#engaged(g, ai.id)) return refuse('conflict');
    this.world.home(g)!.remove(g, 'member', ai.id);
    if (ai.secretary) {
      this.world.change(g, 'group', g, { secretary: undefined });
      this.post('system', { g, by: 'system' }, { code: 'no_secretary' });
    }
    this.world.changed();
    return ok;
  }

  #removePerson(g: string, target: Row): Answer {
    if (target.role === 'owner') return refuse('forbidden');
    const theirs = this.ais(g).filter((ai) => ai.user === target.user);
    if (theirs.some((ai) => this.#engaged(g, ai.id))) return refuse('conflict');
    for (const ai of theirs) this.removeAI(g, ai);
    this.world.home(g)!.remove(g, 'member', target.id as string);
    const body = { code: 'member_removed', user: target.user, name: target.name, by: ids.me };
    this.post('system', { g, by: 'system' }, body);
    this.world.changed();
    return ok;
  }

  /** `group set`; a new name is listed again, a new secretary takes the flag. */
  group(payload: Fields): Answer {
    const g = payload.g as string;
    const changes = Object.fromEntries(
      groupFields.filter((key) => key in payload).map((key) => [key, payload[key]]),
    );
    if (payload.secretary) this.#secretary(g, payload.secretary as string);
    this.world.change(g, 'group', g, {
      ...changes,
      ...(payload.secretary ? { secretary: payload.secretary } : {}),
    });
    if (changes.name) this.#rename(g, changes.name as string);
    return ok;
  }

  #secretary(g: string, id: string) {
    const old = this.secretary(g);
    if (old) this.world.write(g, { ...old, secretary: false } as Row);
    const next = this.ai(g, id);
    if (next) this.world.write(g, { ...next, secretary: true } as Row);
  }

  #rename(g: string, name: string) {
    const node = this.world.home(g)!;
    node.groups.get(g)!.name = name;
    node.relist();
  }

  /** A person asks to watch an AI's terminal: the card waits for the AI's owner. */
  askToWatch(g: string, payload: Fields) {
    const body = payload.body as { ai: string };
    const ai = this.ai(g, body.ai);
    if (!ai || ai.user === this.world.me) return refuse('conflict');
    const say = { g, by: ids.me, main: true, id: payload.n as string };
    const card = this.post(
      'confirm',
      say,
      { what: 'watch', ai: ai.id, to: [ai.user] },
      { status: 'pending' },
    );
    this.world.after(3500, () => this.#minaLets(g, card));
    return ok;
  }

  /** Mina confirms that the person may watch her AI's terminal. */
  #minaLets(g: string, card: Msg) {
    const by = this.#member(g, users.mina)?.id;
    if (!by || (card.state as { status: string }).status !== 'pending') return;
    const ai = (card.body as { ai: string }).ai;
    this.decide(g, card.id, {
      status: 'confirmed',
      by,
      at: Date.now(),
      act: { p: '{}', k: users.mina, s: 'demo' },
    });
    this.world.memory.grants.push({ g, member: ai, user: this.world.me });
    this.world.node(devices.minaPc)?.broadcast(this.world.info(this.world.node(devices.minaPc)!));
  }

  /** Mina asks to watch the person's AI while they have taken over its terminal, once per takeover. */
  minaAsks(g: string, ai: string) {
    if (!this.#member(g, users.mina)) return;
    const body = { what: 'watch', ai, to: [this.world.me] };
    this.post('confirm', { g, by: this.#member(g, users.mina)!.id as string, main: true }, body, {
      status: 'pending',
    });
  }

  /** A `confirm` card was decided by the person (confirm.md section 5). */
  decided(g: string, card: Msg) {
    const body = card.body as { what: string; ai: string };
    const confirmed = (card.state as { status: string }).status === 'confirmed';
    if (!confirmed) return;
    if (body.what === 'direct' && card.re)
      this.world.after(2500, () => this.#direct(g, body.ai, card.re!));
    if (body.what === 'watch') this.#grant(g, body.ai, card.by);
  }

  #direct(g: string, ai: string, request: string) {
    const asked = this.message(g, request);
    if (!asked) return;
    this.touch(g, ai);
    this.post('text', { g, by: ai, text: words.directDone, mentions: [asked.by] });
    this.world.after(3000, () => this.post('text', { g, by: asked.by, text: words.minaThanks }));
  }

  #grant(g: string, ai: string, by: string) {
    const user = this.world.find<Row>(g, 'member', by)?.user as string | undefined;
    if (!user) return;
    this.world.memory.grants.push({ g, member: ai, user });
    const host = this.world.node(this.ai(g, ai)?.host ?? '');
    host?.broadcast(this.world.info(host));
  }
}
