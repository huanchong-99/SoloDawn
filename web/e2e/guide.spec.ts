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

  // Navigate to #/onboarding via Connect, then reload so dock mounts fresh on #/onboarding.
  await page.locator('.guide-dock .step.cur button', { hasText: 'Connect' }).click();
  await poll(() => page.url()).toContain('#/onboarding');
  await page.locator('.guide-pill').waitFor();
  await page.reload();
  await page.locator('#prompt').waitFor();

  // Dock must auto-collapse to pill on #/onboarding instead of covering cards / status.
  await page.locator('.guide-pill').waitFor();
  expect(await page.locator('.guide-dock').count()).toBe(0);

  // Person can open pill, and pairing updates auto-collapse before covering Account type.
  await page.locator('.guide-pill').click();
  await page.locator('.guide-dock').waitFor();

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
  await page.locator('.guide-pill').waitFor();
  expect(await page.locator('.guide-dock').count()).toBe(0);

  await page.locator('input[name="acct-claude"][value="personal"]').check();
  await poll(() => page.locator('fieldset.acct').first().isDisabled()).toBe(false);
  await page.locator('input[name="acct-codex"][value="api_key"]').check();
  await poll(() => page.locator('a.done-btn').count()).toBe(1);
  expect(await page.locator('.guide-dock').count()).toBe(0);
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
    'collapses to pill over onboarding, nodes Set defaults/Save, and honors 768-1279 px chat pill rule at $width x $height',
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

      // Create a group so step 5 becomes current on chat, then verify 768-1279 px pill rule.
      await page.locator('.guide-pill').click();
      await page.locator('.guide-dock').waitFor();
      await page.locator('.guide-dock .step.cur button', { hasText: 'Create a group' }).click();
      await poll(() => page.url()).toContain('#/groups');
      const folder = await makeFolder(net.dir, 'work');
      await createGroup(page, {
        name: 'GuideCheck',
        member: 'Owner',
        ais: [{ cli: 'Claude Code', computer: 'studio', name: 'Claude', folder, secretary: true }],
      });
      await page.locator('.composer .send').waitFor();
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
