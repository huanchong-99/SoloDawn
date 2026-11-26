// The page of jump.spec.ts: a home whose group has a card 160 messages back, and whose pages of
// the main line wait until the test lets each one go.
import { browser, example, owner, TestHome } from '../features/home';
import { homes } from '../src/lib/groups.svelte';
import { jumpToMessage } from '../src/lib/jump';
import type * as p from '../src/lib/proto';

const g = 'jump-group';
const card = 'far-card';
/** The pages asked for and not yet answered: what lets each go, and the oldest row it brings. */
const pages: { go: () => void; oldest: Promise<string> }[] = [];

class LateHome extends TestHome {
  override connect(push: (frame: p.Push) => void) {
    const { ctl, close } = super.connect(push);
    // Ctl.request is overloaded per request type; this passes every type through.
    const base = ctl.request as (req: { t: string; r?: string }) => Promise<unknown>;
    const request = (req: { t: string; r?: string }) => {
      if (req.t !== 'fetch' || req.r) return base(req);
      let go!: () => void;
      const answer = new Promise<void>((resolve) => (go = resolve)).then(() => base(req));
      pages.push({ go, oldest: answer.then((page) => (page as p.Fetched).rows.at(-1)!.id) });
      return answer;
    };
    return { ctl: { request } as p.Ctl, close };
  }
}

const home = new LateHome();

async function cast() {
  home.add(g, 'Website');
  const say = (i: number) => home.say(g, owner, `Message ${i}: ${'text '.repeat(i % 7)}`);
  for (let i = 0; i < 39; i += 1) say(i);
  home.write(g, example('row-msg-approval-approved', { id: card, ts: Date.now() }));
  for (let i = 39; i < 199; i += 1) say(i);
  const me = browser(home);
  homes.push(me.store);
  await me.connect();
  return { g, card };
}

/** How many pages wait. */
const waiting = () => pages.length;

/** Lets the first waiting page go; the id of its oldest message. */
function release() {
  const page = pages.shift()!;
  page.go();
  return page.oldest;
}

/** Jumps to the message `back` before the newest, as a link to it does. */
function jumpBack(back: number) {
  const msgs = [...home.groups.get(g)!.rows.values()].filter((row) => row.r === 'msg');
  const id = msgs.sort((a, b) => (b.pos as number) - (a.pos as number))[back].id;
  jumpToMessage(new Event('click'), id);
}

/** A new message at the end of the line; its id. */
const arrive = () => home.say(g, owner, 'A new message').id;

declare global {
  interface Window {
    jumpCast: typeof jumpCast;
  }
}

const jumpCast = { cast, waiting, release, jumpBack, arrive };
window.jumpCast = jumpCast;
