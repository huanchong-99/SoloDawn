// The step-2 cast in a real browser: P owns the group home P1 and a second computer P2, Q is a
// member whose AIs run on Q1. Loaded by the page of e2e/s2.spec.ts.
import { homes } from '../src/lib/groups.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import { browser, example, ownedNode, owner, TestHome, user, type OwnedNode } from './home';

type Fields = Record<string, unknown>;

export const ids = {
  g: 'c3RlcC10d28tZ3JvdXAx',
  team: 'c3RlcC10d28tdGVhbTAx',
  q: 'cS1wZXJzb24tMDAwMDAx',
  qUser: 'cS11c2VyLTAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAx',
  qTeam: 'cS10ZWFtLWFpLTAwMDAx',
  qPersonal: 'cS1wZXJzb25hbC1haTAx',
  q1: 'cTEtZGV2aWNlLTAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAx',
  p2: 'cDItZGV2aWNlLTAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAx',
  c1: 'Y29uZmlybS0wMDAwMDAwMDAx',
  c2: 'Y29uZmlybS0wMDAwMDAwMDAy',
  user,
};

const home = new TestHome('P1');
home.reply = (payload) =>
  payload.t === 'invite'
    ? { ok: true, link: 'https://p1.example/#join=one-time', expires: Date.now() + 600_000 }
    : { ok: true };
const ai = (id: string, name: string, account: string, state: string) =>
  example('row-member-ai', {
    id,
    g: ids.g,
    user: ids.qUser,
    name,
    cli: 'claude',
    host: ids.q1,
    host_name: 'Q1',
    account,
    allow: [],
    approvers: [],
    state,
  });
home.add(ids.g, 'Website');
home.write(ids.g, example('row-group-custom', { id: ids.g, checks: ['pnpm check', 'pnpm test'] }));
home.write(
  ids.g,
  example('row-member-human', { id: ids.q, g: ids.g, user: ids.qUser, name: 'Q', role: 'member' }),
);
home.write(ids.g, ai(ids.qTeam, 'Q-team', 'team', 'takeover'));
home.write(ids.g, ai(ids.qPersonal, 'Q-personal', 'personal', 'working'));
const ask = home.say(ids.g, owner, '@Q-personal fix the header', { mentions: [ids.qPersonal] });
for (const id of [ids.c1, ids.c2])
  home.write(
    ids.g,
    example('row-msg-confirm-pending', {
      id,
      g: ids.g,
      by: owner,
      re: ask.id,
      body: { what: 'direct', ai: ids.qPersonal, to: [ids.qUser] },
    }),
  );
/** Writes a secretary's notice to a team group. */
const bulletin = (g: string) => home.write(g, example('row-msg-bulletin', { g }));
home.add(ids.team, 'Team', 'row-group-team');
bulletin(ids.team);

let other: OwnedNode;

/** Connects this page as P or Q to P1, and to the other computer of that person. */
async function cast(as: 'P' | 'Q') {
  prefs.lang = 'en';
  const me = browser(home, as === 'P' ? user : ids.qUser);
  other = ownedNode(as === 'P' ? 'P2' : 'Q1', as === 'P' ? ids.p2 : ids.q1);
  other.info.agents = [
    {
      ...other.info.agents[0],
      groups: [
        { g: ids.g, member: ids.qTeam, level: 2, state: 'takeover', queued: 0, watchers: [user] },
      ],
    },
  ];
  other.home.reply = () => ({ ok: true });
  homes.push(me.store, other.store);
  await Promise.all([me.connect(), other.connect()]);
}

const commands = (of: TestHome) =>
  of.asked
    .filter((req) => req.t === 'cmd')
    .map((req) => {
      const { n, ts, ...rest } = JSON.parse((req.env as { p: string }).p) as Fields;
      return rest;
    });

/** The commands P1 received, without nonce and time. */
const sent = () => commands(home);

/** The commands the other computer of this page's person received, without nonce and time. */
const told = () => commands(other.home);

type Id = keyof typeof ids;

/** Writes changes into a row of the group. */
const update = (r: string, id: Id, changes: Fields) =>
  home.write(ids.g, {
    ...home.groups.get(ids.g)!.rows.get(`${r} ${ids[id]}`)!,
    ...changes,
  } as never);

const remove = (r: string, id: Id) => home.remove(ids.g, r, ids[id]);

const s2 = { ids, cast, sent, told, update, remove, bulletin };
declare global {
  interface Window {
    s2: typeof s2;
  }
}
window.s2 = s2;
