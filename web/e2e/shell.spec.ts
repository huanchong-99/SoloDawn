import type { Browser, BrowserContext } from 'playwright';
import { afterAll, afterEach, describe, expect, inject, test } from 'vitest';
import { launch } from './config';
import {
  chooseFolder,
  createGroup,
  makeFolder,
  openDirectContext,
  pairPrimaryBrowser,
  poll,
  startDirectSurface,
  type DirectSurface,
} from './direct';
import { startNet, type Net } from './nodes';

describe('App shell scroll containment E2E', () => {
  let browser: Browser;
  let net: Net | undefined;
  let surface: DirectSurface | undefined;
  const contexts: BrowserContext[] = [];

  afterEach(async () => {
    await Promise.allSettled(contexts.splice(0).map((ctx) => ctx.close()));
    await surface?.close();
    await net?.stop();
    surface = undefined;
    net = undefined;
  });

  afterAll(async () => {
    await browser?.close();
  });

  test('keeps app shell unclipped from focus scrolling when opening KB edit at bottom of #chat-side', async () => {
    browser ??= await launch(inject('browser'));
    net = await startNet({ nodes: 1 });
    surface = await startDirectSurface(net.rendezvous, inject('browser'));
    const context = await openDirectContext(browser, contexts, surface);
    const page = await context.newPage();
    await page.setViewportSize({ width: 1440, height: 900 });

    await pairPrimaryBrowser(page, surface, net.nodes[0], 'studio');

    const claudeFolder = await makeFolder(net.dir, 'claude-ws');
    const codexFolder = await makeFolder(net.dir, 'codex-ws');
    const workerFolder = await makeFolder(net.dir, 'worker-ws');
    await page.getByRole('button', { name: 'New group' }).first().click();
    await poll(() => page.locator('dialog.dlg[open]').count()).toBe(1);
    await createGroup(page, {
      name: 'ScrollCheck',
      member: 'Owner',
      ais: [
        {
          cli: 'Claude Code',
          computer: 'studio',
          name: 'Claude',
          folder: claudeFolder,
          secretary: true,
        },
        {
          cli: 'Codex',
          computer: 'studio',
          name: 'Codex',
          folder: codexFolder,
        },
      ],
    });

    const members = page.locator('#chat-side .members');
    await members.locator('button', { hasText: 'Add AI' }).click();
    const addDlg = page.locator('dialog.dlg[open]');
    await addDlg.locator('.choice', { hasText: 'Claude Code · studio' }).locator('input').check();
    await addDlg.getByRole('textbox', { name: 'Name in the group' }).fill('Worker');
    await chooseFolder(addDlg, workerFolder);
    await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
    await poll(() => addDlg.count()).toBe(0);
    await poll(() => members.locator('.member').count()).toBe(3);

    await page.locator('#chat-side .groupinfo details > summary').click();
    await page.locator('#chat-side .groupinfo button.add').click();
    await page
      .locator('#chat-side .groupinfo .edit-field .input')
      .fill('Keep side panel scrollable');
    await page.locator('#chat-side .groupinfo .edit-field button.btn-primary').click();
    await poll(() => page.locator('#chat-side .groupinfo .mem-list .mem-item').count()).toBe(1);

    await page.locator('#chat-side .preset details > summary').click();
    await page.locator('#chat-side .preset button.add').click();
    const presetFields = page.locator('#chat-side .preset .edit-field .input');
    await presetFields.nth(0).fill('Concise');
    await presetFields.nth(1).fill('Keep replies short and direct.');
    await page.locator('#chat-side .preset .edit-field button.btn-primary').click();
    await poll(() => page.locator('#chat-side .preset .mem-list .mem-item').count()).toBe(1);

    await page.locator('#chat-side .kb details > summary').click();

    const side = page.locator('#chat-side');
    await poll(() => side.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
    expect(await side.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);

    await page.locator('#chat-side .kb button.add').click();

    const input = page.locator('#chat-side .kb .edit-field .input').first();
    await input.waitFor();
    await poll(() => input.evaluate((el) => document.activeElement === el)).toBe(true);

    const layout = await page.evaluate(() => ({
      rootScroll: document.scrollingElement!.scrollTop,
      appScroll: document.querySelector('.app')!.scrollTop,
      barTop: document.querySelector('.bar')!.getBoundingClientRect().top,
    }));
    expect(layout).toEqual({ rootScroll: 0, appScroll: 0, barTop: 0 });
  });

  test('keeps onboarding, settings, and Notice scrollable at phone widths', async () => {
    browser ??= await launch(inject('browser'));
    net = await startNet({ nodes: 1 });
    surface = await startDirectSurface(net.rendezvous, inject('browser'));
    const context = await openDirectContext(browser, contexts, surface);
    const page = await context.newPage();
    await page.setViewportSize({ width: 375, height: 320 });

    await page.goto(`${surface.url}#/onboarding`);
    await page.locator('.app > main .onboarding').waitFor();
    const onboarding = await page.evaluate(() => {
      const main = document.querySelector<HTMLElement>('.app > main')!;
      main.scrollTop = 0;
      main.scrollTo(0, 120);
      return {
        overflows: main.scrollHeight > main.clientHeight,
        scrollTop: main.scrollTop,
      };
    });
    expect(onboarding.overflows).toBe(true);
    expect(onboarding.scrollTop).toBeGreaterThan(0);

    await page.goto(`${surface.url}#/settings`);
    await page.locator('.app > main .guide-card').waitFor();
    const settings = await page.evaluate(() => {
      const main = document.querySelector<HTMLElement>('.app > main')!;
      main.scrollTop = 0;
      main.scrollTo(0, 120);
      return {
        overflows: main.scrollHeight > main.clientHeight,
        scrollTop: main.scrollTop,
      };
    });
    expect(settings.overflows).toBe(true);
    expect(settings.scrollTop).toBeGreaterThan(0);

    const noticePage = await context.newPage();
    await noticePage.setViewportSize({ width: 375, height: 200 });
    await noticePage.addInitScript(() => {
      CSS.supports = () => false;
    });
    await noticePage.goto(surface.url);
    await noticePage.locator('main .card').waitFor();
    const notice = await noticePage.evaluate(() => {
      const root = document.scrollingElement!;
      const overflows = root.scrollHeight > root.clientHeight;
      window.scrollTo(0, 120);
      return {
        hasApp: document.querySelector('.app') !== null,
        overflows,
        scrollTop: root.scrollTop,
      };
    });
    expect(notice).toMatchObject({ hasApp: false, overflows: true });
    expect(notice.scrollTop).toBeGreaterThan(0);
  });
});
