// End-to-end milestone 1 (E-M1): onboarding pairing prompt, fingerprint verification, CLI account
// selection, top-bar connection indicator, and Computers page with two enrolled nodes (A1.1, A1.2).
import type { Browser, BrowserContext } from 'playwright';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test } from 'vitest';
import { launch } from './config';
import {
  openDirectContext,
  poll,
  readPairingCode,
  startDirectSurface,
  type DirectSurface,
} from './direct';
import { startNet, type Net, type Node } from './nodes';

const fingerprintPattern = /^fingerprint ([0-9A-F]{4} [0-9A-F]{4} [0-9A-F]{4})$/m;

async function nodeFingerprint(node: Node): Promise<string> {
  const status = await node.run('solodawn', 'status');
  expect(status.code, status.stderr).toBe(0);
  expect(status.stdout).toContain('version dev\n');
  const match = fingerprintPattern.exec(status.stdout);
  expect(match, status.stdout).not.toBeNull();
  return match ? match[1] : '';
}

describe('E-M1: onboarding, pairing, account declaration, and two computers', () => {
  let browser: Browser;
  let net: Net | undefined;
  let surface: DirectSurface | undefined;
  const contexts: BrowserContext[] = [];

  beforeAll(async () => {
    browser = await launch(inject('browser'));
  });
  afterAll(() => browser?.close());
  afterEach(async () => {
    await Promise.allSettled(contexts.splice(0).map((ctx) => ctx.close()));
    await surface?.close();
    surface = undefined;
  });
  afterEach(() => net?.stop());

  test(
    'pairs a first node through onboarding and enrolls a second node on Computers',
    { timeout: 90_000 },
    async () => {
      const active = await startNet({ nodes: 2 });
      net = active;
      const direct = await startDirectSurface(active.rendezvous, inject('browser'));
      surface = direct;
      const [firstNode, secondNode] = active.nodes;

      const fpFirst = await nodeFingerprint(firstNode);
      const fpSecond = await nodeFingerprint(secondNode);
      expect(fpSecond).not.toBe(fpFirst);

      const context = await openDirectContext(browser, contexts, direct);
      const page = await context.newPage();

      await page.goto(`${direct.url}#/onboarding`);
      await poll(() => page.locator('.hero h1').textContent()).toContain(
        'Send one prompt to your AI.',
      );

      const firstCode = await readPairingCode(page, direct.signal);
      const promptText = (await page.locator('#prompt').textContent()) ?? '';
      expect(promptText).toContain(`${direct.url}install.md`);
      expect(promptText).toContain(`--rendezvous ${direct.signal}`);
      await poll(() => page.locator('.prompt footer').textContent()).toContain('expires in');

      const pairFirst = await firstNode.run(
        'solodawn',
        'pair',
        firstCode,
        '--name',
        'workstation',
        '--rendezvous',
        direct.signal,
      );
      expect(pairFirst.code, pairFirst.stderr).toBe(0);

      const steps = page.locator('.link-steps');
      await poll(() => steps.textContent()).toContain('Found: Claude Code, Codex');
      await poll(() => steps.textContent()).toContain('workstation');
      await poll(() => steps.locator('code').textContent()).toBe(fpFirst);
      await poll(() => steps.textContent()).toContain('Direct connection established');

      await poll(() =>
        page.locator('section[aria-labelledby="st-h"] .badge').textContent(),
      ).toContain('Action needed');
      expect(await page.locator('a.done-btn').count()).toBe(0);

      await page.locator('input[name="acct-claude"][value="personal"]').check();
      await poll(() => page.locator('fieldset.acct').first().isDisabled()).toBe(false);
      await page.locator('input[name="acct-codex"][value="api_key"]').check();

      await poll(() =>
        page.locator('section[aria-labelledby="st-h"] .badge').textContent(),
      ).toContain('Connected');
      await poll(() => page.locator('.prompt footer').textContent()).toContain(
        'Pairing code used and discarded.',
      );

      const doneLink = page.locator('a.done-btn');
      await poll(() => doneLink.textContent()).toContain('Create your first group');
      await doneLink.click();
      await poll(() => page.url()).toContain('#/groups');

      const indicator = page.locator('.bar a.conn');
      await poll(() => indicator.getAttribute('data-st')).toBe('ok');
      await poll(() => indicator.textContent()).toContain('Direct');
      await poll(() => indicator.getAttribute('aria-label')).toMatch(/^Connection status: direct/);

      await indicator.click();
      await poll(() => page.url()).toContain('#/nodes');

      const workstationCard = page.locator('section.card.node', { hasText: 'workstation' });
      await poll(() => workstationCard.count()).toBe(1);
      await poll(() => workstationCard.locator('header .st').textContent()).toContain('Online');
      await poll(() => workstationCard.locator('header .sub').textContent()).toMatch(
        /^(darwin|linux|windows)\/\S+\s*·\s*Node\s+dev\s*·\s*0 of \d+ AIs open$/,
      );
      await poll(() => workstationCard.locator('dl.kv').textContent()).toContain('Direct');
      await poll(() => workstationCard.textContent()).toContain('Claude Code');
      await poll(() => workstationCard.textContent()).toContain('Personal subscription');
      await poll(() => workstationCard.textContent()).toContain('Codex');
      await poll(() => workstationCard.textContent()).toContain('API key');

      await page.locator('.page-head a.btn', { hasText: 'Add a computer' }).click();
      await poll(() => page.url()).toContain('#/onboarding');

      const secondCode = await readPairingCode(page, direct.signal);
      expect(secondCode).not.toBe(firstCode);

      const pairSecond = await secondNode.run(
        'solodawn',
        'pair',
        secondCode,
        '--name',
        'laptop',
        '--rendezvous',
        direct.signal,
      );
      expect(pairSecond.code, pairSecond.stderr).toBe(0);

      await poll(() => steps.textContent()).toContain('Found: Claude Code, Codex');
      await poll(() => steps.textContent()).toContain('laptop');
      await poll(() => steps.locator('code').textContent()).toBe(fpSecond);

      await page.locator('input[name="acct-claude"][value="team"]').check();
      await poll(() =>
        page
          .locator('section[aria-labelledby="acct-h-claude"]')
          .evaluate((el) => el.classList.contains('need')),
      ).toBe(false);

      await page.locator('header.top a.brand').click();
      await poll(() => page.url()).toContain('#/groups');
      await page.locator('.rail a[href="#/nodes"]').click();
      await poll(() => page.url()).toContain('#/nodes');

      const laptopCard = page.locator('section.card.node', { hasText: 'laptop' });
      await poll(() => laptopCard.count()).toBe(1);
      await poll(() => laptopCard.locator('header .st').textContent()).toContain('Online');
      await poll(() => laptopCard.locator('header .sub').textContent()).toMatch(
        /^(darwin|linux|windows)\/\S+\s*·\s*Node\s+dev\s*·\s*0 of \d+ AIs open$/,
      );
      await poll(() => laptopCard.locator('dl.kv').textContent()).toContain('Direct');
      await poll(() => laptopCard.textContent()).toContain('Account type not set');

      await laptopCard.locator('fieldset.acct input[value="api_key"]').check();
      await poll(() => laptopCard.locator('fieldset.acct').count()).toBe(0);
      await poll(() => laptopCard.textContent()).toContain('Team plan');
      await poll(() => laptopCard.textContent()).toContain('API key');

      await poll(() => page.locator('section.card.node').count()).toBe(2);
      await poll(() => indicator.getAttribute('data-st')).toBe('ok');
    },
  );
});
