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

beforeAll(async () => {
  outDir = await mkdtemp(join(tmpdir(), 'solodawn-web-'));
  const config = { build: { outDir }, logLevel: 'silent' } as const;
  await build(config);
  server = await preview({ ...config, preview: { host: '127.0.0.1', port: 0 } });
  browser = await chromium.launch();
});

afterAll(async () => {
  await browser?.close();
  await server?.close();
  await rm(outDir, { recursive: true, force: true });
});

export async function newPage() {
  await page?.close();
  page = await browser.newPage({ locale: 'zh-CN', colorScheme: 'light' });
}

export async function open(hash = '') {
  await page.goto(server.resolvedUrls!.local[0] + hash);
  await page.getByRole('navigation').waitFor();
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
        if (event.pseudoElement !== '::view-transition-new(root)') return;
        document.removeEventListener('animationstart', started);
        resolve(document.documentElement.dataset.theme);
      });
    });
    (button as HTMLButtonElement).click();
    return Promise.all([document.documentElement.dataset.theme, fading]);
  });
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
