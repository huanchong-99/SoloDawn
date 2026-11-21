// End-to-end milestone 1 (E-M1): onboarding pairing prompt, fingerprint verification, CLI account
// selection, top-bar connection indicator, and Computers page with two enrolled nodes (A1.1, A1.2).
import type { Browser, BrowserContext, Page } from 'playwright';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test, vi } from 'vitest';
import { launch } from './config';
import { startNet, type Net, type Node } from './nodes';

const codePattern =
  /\b([23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{4}(?:-[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{4}){3})\b/;
const fingerprintPattern = /^fingerprint ([0-9A-F]{4} [0-9A-F]{4} [0-9A-F]{4})$/m;

async function nodeFingerprint(node: Node): Promise<string> {
  const status = await node.run('solodawn', 'status');
  expect(status.code, status.stderr).toBe(0);
  expect(status.stdout).toContain('version dev\n');
  const match = fingerprintPattern.exec(status.stdout);
  expect(match, status.stdout).not.toBeNull();
  return match ? match[1] : '';
}

async function readPairingCode(page: Page): Promise<string> {
  const prompt = page.locator('#prompt');
  let found = '';
  await vi.waitFor(async () => {
    const muted = await prompt.evaluate((el) => el.classList.contains('muted'));
    expect(muted).toBe(false);
    const text = (await prompt.textContent()) ?? '';
    const match = codePattern.exec(text);
    expect(match, text).not.toBeNull();
    found = match ? match[1] : '';
  });
  return found;
}

describe('E-M1: onboarding, pairing, account declaration, and two computers', () => {
  let browser: Browser;
  let net: Net | undefined;
  const contexts: BrowserContext[] = [];

  beforeAll(async () => {
    browser = await launch(inject('browser'));
  });
  afterAll(() => browser?.close());
  afterEach(() => Promise.allSettled(contexts.splice(0).map((ctx) => ctx.close())));
  afterEach(() => net?.stop());

  test('pairs a first node through onboarding and enrolls a second node on Computers', async () => {
    const active = await startNet({ nodes: 2 });
    net = active;
    const [firstNode, secondNode] = active.nodes;

    const fpFirst = await nodeFingerprint(firstNode);
    const fpSecond = await nodeFingerprint(secondNode);
    expect(fpSecond).not.toBe(fpFirst);

    const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
    contexts.push(context);
    const page = await context.newPage();

    await page.goto(`${active.rendezvous.url}#/onboarding`);
    await expect
      .poll(() => page.locator('.hero h1').textContent())
      .toContain('Send one prompt to your AI.');

    const firstCode = await readPairingCode(page);
    const promptText = (await page.locator('#prompt').textContent()) ?? '';
    expect(promptText).toContain(`${active.rendezvous.url}install.md`);
    expect(promptText).toContain(`--rendezvous ${active.rendezvous.signal}`);
    await expect.poll(() => page.locator('.prompt footer').textContent()).toContain('expires in');

    const pairFirst = await firstNode.run(
      'solodawn',
      'pair',
      firstCode,
      '--name',
      'workstation',
      '--rendezvous',
      active.rendezvous.signal,
    );
    expect(pairFirst.code, pairFirst.stderr).toBe(0);

    const steps = page.locator('.link-steps');
    await expect.poll(() => steps.textContent()).toContain('workstation');
    await expect.poll(() => steps.locator('code').textContent()).toBe(fpFirst);
    await expect.poll(() => steps.textContent()).toContain('Direct connection established');
    await expect.poll(() => steps.textContent()).toContain('Found: Claude Code, Codex');

    await expect
      .poll(() => page.locator('section[aria-labelledby="st-h"] .badge').textContent())
      .toContain('Action needed');
    expect(await page.locator('a.done-btn').count()).toBe(0);

    await page.locator('input[name="acct-claude"][value="personal"]').check();
    await page.locator('input[name="acct-codex"][value="api_key"]').check();

    await expect
      .poll(() => page.locator('section[aria-labelledby="st-h"] .badge').textContent())
      .toContain('Connected');
    await expect
      .poll(() => page.locator('.prompt footer').textContent())
      .toContain('Pairing code used and discarded.');

    const doneLink = page.locator('a.done-btn');
    await expect.poll(() => doneLink.textContent()).toContain('Create your first group');
    await doneLink.click();
    await expect.poll(() => page.url()).toContain('#/groups');

    const indicator = page.locator('.bar a.conn');
    await expect.poll(() => indicator.getAttribute('data-st')).toBe('ok');
    await expect.poll(() => indicator.textContent()).toContain('Direct');
    await expect
      .poll(() => indicator.getAttribute('aria-label'))
      .toMatch(/^Connection status: direct/);

    await indicator.click();
    await expect.poll(() => page.url()).toContain('#/nodes');

    const workstationCard = page.locator('section.card.node', { hasText: 'workstation' });
    await expect.poll(() => workstationCard.count()).toBe(1);
    await expect
      .poll(() => workstationCard.locator('header .st').textContent())
      .toContain('Online');
    await expect
      .poll(() => workstationCard.locator('header .sub').textContent())
      .toMatch(/^(darwin|linux|windows) · Node dev · 0 of \d+ AIs open$/);
    await expect.poll(() => workstationCard.locator('dl.kv').textContent()).toContain('Direct');
    await expect.poll(() => workstationCard.textContent()).toContain('Claude Code');
    await expect.poll(() => workstationCard.textContent()).toContain('Personal subscription');
    await expect.poll(() => workstationCard.textContent()).toContain('Codex');
    await expect.poll(() => workstationCard.textContent()).toContain('API key');

    await page.locator('.page-head a.btn', { hasText: 'Add a computer' }).click();
    await expect.poll(() => page.url()).toContain('#/onboarding');

    const secondCode = await readPairingCode(page);
    expect(secondCode).not.toBe(firstCode);

    const pairSecond = await secondNode.run(
      'solodawn',
      'pair',
      secondCode,
      '--name',
      'laptop',
      '--rendezvous',
      active.rendezvous.signal,
    );
    expect(pairSecond.code, pairSecond.stderr).toBe(0);

    await expect.poll(() => steps.textContent()).toContain('laptop');
    await expect.poll(() => steps.locator('code').textContent()).toBe(fpSecond);

    await page.locator('input[name="acct-claude"][value="team"]').check();
    await expect
      .poll(() =>
        page
          .locator('section[aria-labelledby="acct-h-claude"]')
          .evaluate((el) => el.classList.contains('need')),
      )
      .toBe(false);

    await page.goto(`${active.rendezvous.url}#/nodes`);
    const laptopCard = page.locator('section.card.node', { hasText: 'laptop' });
    await expect.poll(() => laptopCard.count()).toBe(1);
    await expect.poll(() => laptopCard.locator('header .st').textContent()).toContain('Online');
    await expect
      .poll(() => laptopCard.locator('header .sub').textContent())
      .toMatch(/^(darwin|linux|windows) · Node dev · 0 of \d+ AIs open$/);
    await expect.poll(() => laptopCard.locator('dl.kv').textContent()).toContain('Direct');
    await expect.poll(() => laptopCard.textContent()).toContain('Account type not set');

    await laptopCard.locator('fieldset.acct input[value="api_key"]').check();
    await expect.poll(() => laptopCard.locator('fieldset.acct').count()).toBe(0);
    await expect.poll(() => laptopCard.textContent()).toContain('Team plan');
    await expect.poll(() => laptopCard.textContent()).toContain('API key');

    await expect.poll(() => page.locator('section.card.node').count()).toBe(2);
    await expect.poll(() => indicator.getAttribute('data-st')).toBe('ok');
  });
});
