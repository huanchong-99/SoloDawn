// A jump to a card far back, which the client fetches alone before it jumps (`Group.seek`, as a
// reminder's address does): the card stays where the jump put it while the pages of the gap and
// the messages before it arrive, one by one, after the jump.
import type { Browser, Page } from 'playwright';
import { createServer, type ViteDevServer } from 'vite';
import { afterAll, afterEach, beforeAll, expect, inject, test } from 'vitest';
import { launch } from './config';
import type {} from './jump-fixture';

let engine: Browser;
let server: ViteDevServer;

beforeAll(async () => {
  server = await createServer({ logLevel: 'warn', server: { host: '127.0.0.1', port: 0 } });
  await server.listen();
  engine = await launch(inject('browser'));
});
afterEach(async () => {
  for (const context of engine.contexts()) await context.close();
});
afterAll(async () => {
  await engine?.close();
  await server?.close();
});

/** The chat of the fixture's group, with more of the address's query, cast. */
async function open(
  motion: 'reduce' | 'no-preference',
  query: string,
  viewport = { width: 1280, height: 720 },
) {
  const page = await engine.newPage({ viewport, reducedMotion: motion });
  page.setDefaultTimeout(8_000);
  await page.goto(`${server.resolvedUrls!.local[0]}?nomock${query}#/groups/jump-group`);
  await page.addScriptTag({ type: 'module', url: '/e2e/jump-fixture.ts' });
  await page.waitForFunction(() => window.jumpCast);
  await page.evaluate(() => window.jumpCast.cast());
  return page;
}

/** Whether the line is scrolled to its top. */
const atTop = (page: Page) =>
  page.evaluate(() => {
    const scroller = document.querySelector('.scroller')!;
    return Math.abs(scroller.scrollTop + scroller.scrollHeight - scroller.clientHeight) <= 1;
  });

const cases = [
  { width: 1280, height: 720, motion: 'no-preference' },
  { width: 1280, height: 720, motion: 'reduce' },
  { width: 390, height: 844, motion: 'no-preference' },
  { width: 390, height: 844, motion: 'reduce' },
] as const;

test.each(cases)(
  'keeps a card 160 messages back in place at $width px with $motion motion',
  async ({ width, height, motion }) => {
    const card = 'far-card';
    // The address a reminder opens: the chat, and the card to jump to.
    const page = await open(motion, `&msg=${card}&kind=approval`, { width, height });

    const top = () =>
      page.evaluate((id) => {
        const scroller = document.querySelector('.scroller')!;
        const el = document.getElementById(`m-${id}`)!;
        return el.getBoundingClientRect().top - scroller.getBoundingClientRect().top;
      }, card);
    // The card is the oldest message held, so the jump ends with the line at its top.
    await page.locator(`#m-${card}:focus-within`).waitFor();
    await expect.poll(() => atTop(page), { timeout: 8_000 }).toBe(true);
    // The card rises as it is drawn; where it is is where it ends.
    const motions = (el: Element) => Promise.all(el.getAnimations().map((each) => each.finished));
    await page.locator(`#m-${card}`).evaluate(motions);
    const before = await top();

    // The gap of 110 messages below the card in two pages, then 10 of them, the card and the 39
    // before it.
    for (let i = 0; i < 3; i += 1) {
      await expect.poll(() => page.evaluate(() => window.jumpCast.waiting())).toBe(1);
      const oldest = await page.evaluate(() => window.jumpCast.release());
      await page.locator(`#m-${oldest}`).waitFor({ state: 'attached' });
      await expect.poll(async () => Math.abs((await top()) - before)).toBeLessThanOrEqual(1);
    }
    await page.getByText('This is where the group starts.').waitFor();
    expect(await page.locator(`#m-${card}:focus-within`).count()).toBe(1);
  },
);

// The scroller's offset: 0 at the newest message, negative above it.
const offset = (page: Page) => page.evaluate(() => document.querySelector('.scroller')!.scrollTop);

test.each(['no-preference', 'reduce'] as const)(
  'lets the card go when the person scrolls, with %s motion',
  async (motion) => {
    const page = await open(motion, '&msg=far-card&kind=approval');
    await page.locator('#m-far-card:focus-within').waitFor();
    // The jump has landed: the line is at its top, and its scroll has been seen.
    await expect.poll(() => atTop(page), { timeout: 8_000 }).toBe(true);
    await page.evaluate(() => new Promise(requestAnimationFrame));
    // A scroll that is not the jump's: Space, a scrollbar, focus, a screen reader or find.
    await page.evaluate(() =>
      document.querySelector('.scroller')!.scrollBy({ top: 400, behavior: 'instant' }),
    );
    await expect
      .poll(() =>
        page.evaluate(() => document.querySelector<HTMLElement>('.scroller')!.style.overflowAnchor),
      )
      .toBe('');
    const card = () =>
      page.evaluate(() => document.getElementById('m-far-card')!.getBoundingClientRect().top);
    const scrolledTo = await card();
    await expect.poll(() => page.evaluate(() => window.jumpCast.waiting())).toBe(1);
    const oldest = await page.evaluate(() => window.jumpCast.release());
    await page.locator(`#m-${oldest}`).waitFor({ state: 'attached' });
    // The page below the card does not bring the card back to where the jump put it, 400 px down.
    expect(Math.abs((await card()) - (scrolledTo + 400))).toBeGreaterThan(100);
  },
);

test('follows new messages after a jump to one at the end of the line', async () => {
  const page = await open('reduce', '');
  await page.locator('.scroller .msg').last().waitFor();
  await page.evaluate(() => window.jumpCast.jumpBack(1));
  for (let i = 0; i < 3; i += 1) {
    const id = await page.evaluate(() => window.jumpCast.arrive());
    await page.locator(`#m-${id}`).waitFor({ state: 'attached' });
    expect(Math.abs(await offset(page))).toBeLessThanOrEqual(1);
  }
});
