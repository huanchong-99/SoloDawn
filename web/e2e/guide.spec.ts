import type { Browser, BrowserContext, Locator, Page } from 'playwright';
import { afterAll, afterEach, describe, expect, inject, test } from 'vitest';
import { launch } from './config';
import {
  chooseFolder,
  createGroup,
  makeFolder,
  openDirectContext,
  poll,
  readPairingCode,
  startDirectSurface,
  type DirectSurface,
} from './direct';
import { startNet, type Net } from './nodes';

async function boxOf(locator: Locator) {
  return (await locator.boundingBox())!;
}

function boxesOverlap(
  a: { x: number; y: number; width: number; height: number },
  b: { x: number; y: number; width: number; height: number },
): boolean {
  return a.x < b.x + b.width && a.x + a.width > b.x && a.y < b.y + b.height && a.y + a.height > b.y;
}

async function expectAllDisjoint(dock: Locator, targets: Locator): Promise<void> {
  const dockBox = await boxOf(dock);
  const count = await targets.count();
  for (let i = 0; i < count; i++) {
    const tBox = await targets.nth(i).boundingBox();
    if (tBox && tBox.width > 0 && tBox.height > 0) {
      expect(boxesOverlap(dockBox, tBox)).toBe(false);
    }
  }
}

async function pairAndVerifyOnboarding(
  page: Page,
  surface: DirectSurface,
  net: Net,
  viewport: { width: number; height: number },
) {
  await page.setViewportSize(viewport);
  await page.goto(`${surface.url}#/groups`);
  const welcome = page.getByRole('dialog', { name: 'Welcome to SoloDawn' });
  await welcome.getByRole('button', { name: 'Start', exact: true }).click();
  await page.locator('.guide-dock').waitFor();

  // Navigate to #/onboarding via Connect, then reload; neither dock nor pill shows on onboarding.
  await page.locator('.guide-dock .step.cur button', { hasText: 'Connect' }).click();
  await poll(() => page.url()).toContain('#/onboarding');
  await page.locator('#prompt').waitFor();
  expect(await page.locator('.guide-dock').count()).toBe(0);
  expect(await page.locator('.guide-pill').count()).toBe(0);
  await page.reload();
  await page.locator('#prompt').waitFor();

  // Neither dock nor pill is mounted on #/onboarding (before or after pairing updates).
  expect(await page.locator('.guide-dock').count()).toBe(0);
  expect(await page.locator('.guide-pill').count()).toBe(0);

  const code = await readPairingCode(page, surface.signal);
  const paired = await net.nodes[0].run(
    'solodawn',
    'pair',
    code,
    '--name',
    'studio',
    '--rendezvous',
    surface.signal,
  );
  expect(paired.code, paired.stderr).toBe(0);

  await poll(() => page.locator('fieldset.acct').count()).toBeGreaterThanOrEqual(2);
  expect(await page.locator('.guide-dock').count()).toBe(0);
  expect(await page.locator('.guide-pill').count()).toBe(0);

  await page.locator('input[name="acct-claude"][value="personal"]').check();
  await poll(() => page.locator('fieldset.acct').first().isDisabled()).toBe(false);
  await page.locator('input[name="acct-codex"][value="api_key"]').check();
  await poll(() => page.locator('a.done-btn').count()).toBe(1);
  expect(await page.locator('.guide-dock').count()).toBe(0);
  expect(await page.locator('.guide-pill').count()).toBe(0);
}

describe('First-run guide overlap auto-collapse E2E', () => {
  let browser: Browser;
  let net: Net | undefined;
  let surface: DirectSurface | undefined;
  const contexts: BrowserContext[] = [];

  afterEach(async () => {
    await Promise.all(contexts.splice(0).map((c) => c.close()));
    await surface?.close();
    await net?.stop();
    surface = undefined;
    net = undefined;
  });

  afterAll(async () => {
    await browser?.close();
  });

  test.each([
    { width: 1440, height: 900 },
    { width: 1280, height: 800 },
  ])(
    'hides guide on onboarding, collapses to pill over nodes Set defaults/Save, and honors 768-1279 px chat pill rule at $width x $height',
    async ({ width, height }) => {
      browser ??= await launch(inject('browser'));
      net = await startNet({ nodes: 1 });
      surface = await startDirectSurface(net.rendezvous, inject('browser'));
      const context = await openDirectContext(browser, contexts, surface);
      const page = await context.newPage();

      await pairAndVerifyOnboarding(page, surface, net, { width, height });

      // Proceed to #/nodes and verify reload/navigation auto-collapses over Set defaults.
      await page.locator('a.done-btn').click();
      await poll(() => page.url()).toContain('#/nodes');
      await poll(() => page.locator('[data-guide="defaults"][data-unset]').count()).toBe(2);
      await page.reload();
      await poll(() => page.locator('[data-guide="defaults"][data-unset]').count()).toBe(2);
      await page.locator('.guide-pill').waitFor();
      expect(await page.locator('.guide-dock').count()).toBe(0);

      // Click Set defaults for Claude Code -> Save editor opens uncovered; guide stays a pill.
      await page.locator('[data-guide="defaults"][data-unset]').first().click();
      const saveBtn = page.locator('.defaults .editor .btn-primary');
      await saveBtn.waitFor();
      await expectAllDisjoint(page.locator('.guide-pill'), saveBtn);
      expect(await page.locator('.guide-dock').count()).toBe(0);
      await saveBtn.click();
      await page.locator('.defaults .editor').waitFor({ state: 'hidden' });

      // Save Codex defaults and confirm guide never auto-expands over #/nodes.
      await page.locator('[data-guide="defaults"][data-unset]').first().click();
      await page.locator('.defaults .editor .btn-primary').click();
      await page.locator('.defaults .editor').waitFor({ state: 'hidden' });
      await poll(() => page.locator('[data-guide="defaults"][data-unset]').count()).toBe(0);
      expect(await page.locator('.guide-dock').count()).toBe(0);
      await page.locator('.guide-pill').waitFor();

      // Create a group with 1 secretary + 1 working AI, verify step 4 points to Add AI, then add
      // a 2nd working AI so step 5 becomes current on chat and test the 768-1279 px pill rule.
      await page.locator('.guide-pill').click();
      await page.locator('.guide-dock').waitFor();
      await page.locator('.guide-dock .step.cur button', { hasText: 'Create a group' }).click();
      await poll(() => page.url()).toContain('#/groups');
      const secFolder = await makeFolder(net.dir, 'work-sec');
      const codexFolder = await makeFolder(net.dir, 'work-codex');
      const workerFolder = await makeFolder(net.dir, 'work-worker');
      await createGroup(page, {
        name: 'GuideCheck',
        member: 'Owner',
        ais: [
          {
            cli: 'Claude Code',
            computer: 'studio',
            name: 'Claude',
            folder: secFolder,
            secretary: true,
          },
          { cli: 'Codex', computer: 'studio', name: 'Codex', folder: codexFolder },
        ],
      });
      await page.locator('#chat-side .members button', { hasText: 'Add AI' }).waitFor();
      await page.locator('.guide-pill.over-chat').click();
      await page.locator('.guide-dock .step.cur button', { hasText: 'Add AI' }).waitFor();
      expect(await page.locator('.guide-dock .step.cur .body p').textContent()).toBe(
        'This group needs 1 more working AI. Click Add AI; you can add the same CLI again with another name.',
      );
      expect(await page.locator('.guide-dock .step.cur .tip').count()).toBe(0);
      await page.locator('.guide-dock .step.cur button', { hasText: 'Add AI' }).click();
      await page.locator('.guide-beacon').waitFor();
      expect(
        boxesOverlap(
          await boxOf(page.locator('.guide-beacon')),
          await boxOf(page.locator('[data-guide="add-ai"]')),
        ),
      ).toBe(true);

      await page.setViewportSize({ width: 768, height: 900 });
      await page.locator('.guide-pill.over-chat').waitFor();
      await expectAllDisjoint(page.locator('.guide-pill.over-chat'), page.locator('.need-ais'));
      await page.locator('.guide-pill.over-chat').click();
      await page.locator('.guide-dock .step.cur button', { hasText: 'Add AI' }).click();
      await page.locator('.guide-beacon').waitFor();
      expect(
        boxesOverlap(
          await boxOf(page.locator('.guide-beacon')),
          await boxOf(page.locator('.side-open')),
        ),
      ).toBe(true);
      await page.setViewportSize({ width, height });
      await page.locator('#chat-side .members button', { hasText: 'Add AI' }).click();
      const addDlg = page.locator('dialog.dlg[open]');
      await addDlg.locator('.choice', { hasText: 'Claude Code · studio' }).locator('input').check();
      await addDlg.getByRole('textbox', { name: 'Name in the group' }).fill('Worker');
      await chooseFolder(addDlg, workerFolder);
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.count()).toBe(0);
      await page.locator('.composer .send').waitFor();
      if (width === 1440) {
        const pill = page.locator('.guide-pill.over-chat');
        await pill.waitFor();
        await poll(() => page.locator('#chat-side .member').count()).toBeGreaterThanOrEqual(3);
        const padBottom = await page.locator('#chat-side').evaluate((el) => {
          const bottom = parseFloat(getComputedStyle(el).paddingBottom);
          el.scrollTo({ top: el.scrollHeight, behavior: 'instant' });
          return bottom;
        });
        expect(padBottom).toBeGreaterThanOrEqual(76);
        await expectAllDisjoint(pill, page.locator('#chat-side .member').last());
        await expectAllDisjoint(pill, page.locator('#chat-side > section').last());
      }
      for (const [chatWidth, chatHeight] of [
        [768, 900],
        [1024, 768],
        [1279, 900],
      ]) {
        await page.setViewportSize({ width: chatWidth, height: chatHeight });
        await page.locator('.guide-pill.over-chat').waitFor();
        await expectAllDisjoint(
          page.locator('.guide-pill.over-chat'),
          page.locator('.composer .send'),
        );
      }
    },
  );
});
