// These steps need layout, CSS animations and the browser's own view-transition snapshots.
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { chromium, type Browser, type Page } from 'playwright';
import { build, preview, type PreviewServer } from 'vite';
import { afterAll, beforeAll, expect } from 'vitest';

let browser: Browser;
let server: PreviewServer;
let outDir: string;
let page: Page;

// Vitest gives a hook 10 s. On a machine whose cores are all busy, browser.close() alone took
// 10.0 s (20 to 60 ms when idle), and the file failed although every test passed.
const browserHookTimeout = 60_000;

beforeAll(async () => {
  outDir = await mkdtemp(join(tmpdir(), 'solodawn-web-'));
  const config = { build: { outDir }, logLevel: 'silent' } as const;
  await build(config);
  server = await preview({ ...config, preview: { host: '127.0.0.1', port: 0 } });
  browser = await chromium.launch();
}, browserHookTimeout);

afterAll(async () => {
  await browser?.close();
  await server?.close();
  await rm(outDir, { recursive: true, force: true });
}, browserHookTimeout);

export async function newPage() {
  await page?.close();
  page = await browser.newPage({ locale: 'zh-CN', colorScheme: 'light' });
}

export async function open(hash = '') {
  await page.goto(server.resolvedUrls!.local[0] + hash);
  await page.getByRole('navigation').waitFor();
  if (!hash || hash === '#/groups') {
    const welcome = page.getByRole('dialog', { name: '欢迎使用 SoloDawn' });
    await welcome.waitFor({ state: 'visible' });
    await welcome.getByRole('button', { name: '先随便看看' }).click();
    await welcome.waitFor({ state: 'hidden' });
  }
}

// Read the first frame of the destination heading, even on a slow test machine.
export function arrival(hash: string) {
  return page.evaluate(
    (hash) =>
      new Promise<number[]>((resolve) => {
        const previous = document.querySelector('main .page-head');
        document.addEventListener('animationstart', function started(event) {
          const target = event.target as HTMLElement;
          if (target === previous || !target.matches('main .page-head')) return;
          document.removeEventListener('animationstart', started);
          const animation = target.getAnimations()[0];
          animation.currentTime = 0;
          const { e, f } = new DOMMatrix(getComputedStyle(target).transform);
          resolve([Math.sign(e), Math.sign(f)]);
        });
        location.hash = hash;
      }),
    hash,
  );
}

// Read the parts of the destination page one frame (16 ms) after they arrive, while the later
// parts still wait their turn to move, and again when no animation runs.
export function waitingParts(hash: string) {
  return page.evaluate(
    (hash) =>
      new Promise<{ opacity: string; travel: string; still: string }[]>((resolve) => {
        const previous = document.querySelector('main .page-head');
        document.addEventListener('animationstart', function started(event) {
          const target = event.target as HTMLElement;
          if (target === previous || !target.matches('main .page-head')) return;
          document.removeEventListener('animationstart', started);
          const parts = [...target.parentElement!.children];
          const animations = parts.flatMap((part) => part.getAnimations());
          for (const animation of animations) {
            animation.pause();
            animation.currentTime = 16;
          }
          const waiting = parts.map((part) => {
            const style = getComputedStyle(part);
            const { e, f } = new DOMMatrix(style.transform);
            return {
              opacity: (+style.opacity).toFixed(2),
              travel: (Math.abs(e) + Math.abs(f)).toFixed(2),
            };
          });
          for (const animation of animations) animation.cancel();
          resolve(
            parts.map((part, i) => ({ ...waiting[i], still: getComputedStyle(part).opacity })),
          );
        });
        location.hash = hash;
      }),
    hash,
  );
}

// Press Tab so many times and read, at the moment focus lands on each part, where the part is
// and how much of its focus ring is seen. The ring of a radio is on its label.
export async function focusCues(tabs: number) {
  await page.evaluate(() => {
    const cues: object[] = [];
    Object.assign(window, { cues });
    document.addEventListener('focusin', (event) => {
      const part = event.target as HTMLElement;
      const ringed = part.closest('.seg label') ?? part;
      const ring = getComputedStyle(ringed, '::after');
      cues.push({
        part: part.getAttribute('aria-label') ?? ringed.textContent!.trim(),
        top: Math.round(part.getBoundingClientRect().top),
        ring: ring.content === 'none' ? getComputedStyle(part).outlineStyle : ring.opacity,
      });
    });
  });
  for (let i = 0; i < tabs; i++) await page.keyboard.press('Tab');
  return page.evaluate(
    () => (window as unknown as { cues: { part: string; top: number; ring: string }[] }).cues,
  );
}

// Hover the logo, then hold a button down, and read each once it has come to rest.
export async function underPointer() {
  const atRest = (selector: string) =>
    page
      .locator(selector)
      .first()
      .evaluate(async (part) => {
        await Promise.all(part.getAnimations({ subtree: true }).map((a) => a.finished));
        return [getComputedStyle(part).transform, getComputedStyle(part, '::before').opacity];
      });
  await page.locator('.logo').hover();
  const [logo] = await atRest('.logo-mark');
  await page.locator('.bar .icon-btn').first().hover();
  await page.mouse.down();
  const [button, layer] = await atRest('.bar .icon-btn');
  await page.mouse.move(0, 0);
  await page.mouse.up();
  return { logo, button, layer };
}

// The selectors of the rules that apply when the system asks for reduced motion.
export function reducedMotionRules() {
  return page.evaluate(() =>
    [...document.styleSheets]
      .flatMap((sheet) => [...sheet.cssRules])
      .filter((rule) => (rule as CSSMediaRule).conditionText?.includes('reduced-motion: reduce'))
      .flatMap((media) => [...(media as CSSMediaRule).cssRules])
      .map((rule) => (rule as CSSStyleRule).selectorText),
  );
}

export async function expectMark(selector: string, items: string, index: number) {
  const offset = await page.locator(selector).evaluate(
    async (group, { items, index }) => {
      await document.fonts.ready;
      await Promise.all(group.getAnimations({ subtree: true }).map((a) => a.finished));
      const choices = group.querySelectorAll(items);
      const first = choices[0].getBoundingClientRect();
      const selected = choices[index].getBoundingClientRect();
      const mark = getComputedStyle(group, '::before');
      const { e, f } = new DOMMatrix(mark.transform);
      // CSS matrices serialize fractional pixels with less precision than DOMRect.
      return [
        mark.content,
        Math.round(Math.abs(first.x + e - selected.x)),
        Math.round(Math.abs(first.y + f - selected.y)),
      ];
    },
    { items, index },
  );
  expect(offset).toEqual(['""', 0, 0]);
}

export async function choose(label: string) {
  await page.getByRole('radio', { name: label, exact: true }).check();
}

export async function viewTransitions() {
  expect(await page.evaluate(() => 'startViewTransition' in document)).toBe(true);
}

export function reduce() {
  return page.emulateMedia({ reducedMotion: 'reduce' });
}

export async function crossFade() {
  return page.getByRole('button', { name: '切换深色 / 浅色' }).evaluate((button) => {
    const fading = new Promise<string | undefined>((resolve) => {
      document.addEventListener('animationstart', function started(event) {
        if (event.pseudoElement !== '::view-transition-old(root)') return;
        document.removeEventListener('animationstart', started);
        resolve(document.documentElement.dataset.theme);
      });
    });
    (button as HTMLButtonElement).click();
    return Promise.all([document.documentElement.dataset.theme, fading]);
  });
}

// The pictures of a page cross-fade that are animating.
function pictures() {
  return page.evaluate(() =>
    document
      .getAnimations()
      .map((animation) => (animation.effect as KeyframeEffect).pseudoElement)
      .filter((pseudo) => pseudo?.startsWith('::view-transition')),
  );
}

// Start a cross-fade and hold it at its first frame.
async function heldCrossFade() {
  await crossFade();
  await page.evaluate(() => document.getAnimations().forEach((animation) => animation.pause()));
  return pictures();
}

const theme = () => page.evaluate(() => document.documentElement.dataset.theme);

// Press as a person does: Playwright presses only when the button is what the pointer hits.
export async function pressDuringCrossFade() {
  const held = await heldCrossFade();
  await page.getByRole('button', { name: '切换深色 / 浅色' }).click({ timeout: 2000 });
  await page.waitForFunction(() => document.documentElement.dataset.theme !== 'dark');
  return { pictures: held, theme: await theme() };
}

export async function reduceDuringCrossFade() {
  const held = await heldCrossFade();
  await reduce();
  await page.evaluate(() => new Promise((frame) => requestAnimationFrame(frame)));
  return { before: held, after: await pictures(), theme: await theme() };
}

export async function reducedThemeChange() {
  return page.getByRole('button', { name: '切换深色 / 浅色' }).evaluate(async (button) => {
    const start = document.startViewTransition.bind(document);
    let transitions = 0;
    document.startViewTransition = (update) => {
      transitions++;
      return start(update);
    };
    (button as HTMLButtonElement).click();
    await Promise.resolve();
    return {
      theme: document.documentElement.dataset.theme,
      transitions,
      animations: document.getAnimations().length,
    };
  });
}

// Moves the theme colour from the radio named `from` with the left arrow key, counting the
// cross-fades the page starts and the animations under way once the colour is chosen.
export async function accentByArrow(from: string) {
  await page.evaluate(() => {
    const start = document.startViewTransition.bind(document);
    Object.assign(window, { crossFades: 0 });
    document.startViewTransition = (update) => {
      Object.assign(window, { crossFades: (window as unknown as Faded).crossFades + 1 });
      return start(update);
    };
  });
  await page.getByRole('radio', { name: from, exact: true }).focus();
  await page.keyboard.press('ArrowLeft');
  return page.evaluate(() => ({
    chosen: document
      .querySelector<HTMLInputElement>('input[name=accent]:checked')!
      .labels![0].textContent!.trim(),
    crossFades: (window as unknown as Faded).crossFades,
    animations: document.getAnimations().length,
  }));
}
type Faded = { crossFades: number };

// The computed colour of each token, as a part painted in it has it.
export function colours(...tokens: string[]) {
  return page.evaluate(
    (tokens) =>
      tokens.map((token) => {
        const probe = document.body.appendChild(document.createElement('i'));
        probe.style.color = `var(--${token})`;
        const colour = getComputedStyle(probe).color;
        probe.remove();
        return colour;
      }),
    tokens,
  );
}

export function forceColors() {
  return page.emulateMedia({ forcedColors: 'active' });
}

// The accessible name of the language button, as Chromium computes it, and the language of each
// part of its content.
export async function languageButton() {
  const button = page.getByRole('button', { name: /Switch language/ });
  return {
    snapshot: await button.ariaSnapshot(),
    parts: await button.evaluate((b) =>
      [...b.querySelectorAll<HTMLElement>('[lang]')].map((part) => [part.textContent, part.lang]),
    ),
  };
}

// For each group of options, which ones are marked in the system highlight colours and keep them:
// the system puts no backplate of its page colour behind the text, which would hide it.
export function highlighted() {
  return page.evaluate(() => {
    const probe = document.body.appendChild(document.createElement('i'));
    probe.style.background = 'Highlight';
    probe.style.color = 'HighlightText';
    const system = getComputedStyle(probe);
    const [highlight, text] = [system.backgroundColor, system.color];
    probe.remove();
    return ['fieldset:has([name=theme]) label', 'fieldset:has([name=lang]) label', '.tabs a'].map(
      (selector) =>
        [...document.querySelectorAll(selector)].map((part) => {
          const style = getComputedStyle(part);
          return (
            style.backgroundColor === highlight &&
            style.color === text &&
            style.forcedColorAdjust === 'none'
          );
        }),
    );
  });
}

// Whether the hover layer of the current page's entry covers it while the pointer is on it.
export async function coveredUnderPointer() {
  const current = page.locator('.tabs a[aria-current]');
  await current.hover();
  return current.evaluate(async (entry) => {
    await Promise.all(entry.getAnimations({ subtree: true }).map((a) => a.finished));
    const layer = getComputedStyle(entry, '::before');
    return layer.display !== 'none' && layer.opacity !== '0';
  });
}
