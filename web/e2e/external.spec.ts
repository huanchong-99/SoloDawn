import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { join } from 'node:path';
import { createInterface } from 'node:readline';
import type { Page } from 'playwright';
import { expect, inject, test } from 'vitest';
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
  finishOnboarding,
} from './direct';
import { startNet, type Node } from './nodes';

async function pair(page: Page, surface: DirectSurface, node: Node) {
  const code = await readPairingCode(page, surface.signal);
  const result = await node.run(
    'solodawn',
    'pair',
    code,
    '--name',
    node.name,
    '--rendezvous',
    surface.signal,
  );
  expect(result.code, result.stderr).toBe(0);
}

async function joinExternal(page: Page, nodes: Node[], folder: string) {
  await page.getByRole('button', { name: 'Add AI' }).click();
  const form = page.getByRole('dialog', { name: 'Add an AI' });
  for (const node of nodes) {
    await poll(() =>
      form.locator('.choice', { hasText: new RegExp(`external.*${node.name}`, 'i') }).count(),
    ).toBe(1);
  }
  await form
    .locator('.choice', { hasText: new RegExp(`external.*${nodes[0].name}`, 'i') })
    .locator('input')
    .check();
  await form.getByRole('textbox', { name: 'Name in the group' }).fill('Scout');
  await form.getByRole('combobox', { name: /^Account/ }).selectOption('api_key');
  await form.getByRole('textbox', { name: 'Tool label' }).fill('ZCode');
  await chooseFolder(form, folder);
  await form.getByRole('button', { name: 'Add AI' }).click();
}

async function withExternalMCP(
  node: Node,
  executable: string,
  member: string,
  folder: string,
  answer: string,
  working: () => Promise<void>,
) {
  const child = spawn(executable, ['mcp', '--member', member], { cwd: folder, env: node.env });
  const closed = once(child, 'close');
  const lines = createInterface({ input: child.stdout })[Symbol.asyncIterator]();
  const write = (message: object) =>
    child.stdin.write(JSON.stringify({ jsonrpc: '2.0', ...message }) + '\n');
  const call = async (id: number, method: string, params: object) => {
    write({ id, method, params });
    const line = await lines.next();
    expect(line.done).toBe(false);
    const reply = JSON.parse(line.value as string);
    expect(reply.id).toBe(id);
    expect(reply.error).toBeUndefined();
    expect(reply.result?.isError).not.toBe(true);
  };
  try {
    await call(1, 'initialize', {
      protocolVersion: '2025-11-25',
      capabilities: {},
      clientInfo: { name: 'browser-acceptance', version: '1' },
    });
    write({ method: 'notifications/initialized' });
    await working();
    await call(2, 'tools/call', { name: 'report_usage', arguments: { window_pct: 40 } });
    await call(3, 'tools/call', { name: 'post', arguments: { text: answer } });
  } finally {
    child.stdin.end();
    child.kill();
    await closed;
  }
}

async function answerCheck(
  node: Node,
  exe: string,
  { member, folder }: { member: string; folder: string },
  answer: string,
  working: () => Promise<void>,
) {
  const got = await node.run('solodawn', 'wait', '--member', member, '--timeout', '30');
  expect(got.code, got.stderr).toBe(0);
  const handout = JSON.parse(got.stdout);
  expect(handout.text).toContain('Setup check: answer here in one line, then finish.');
  // Every hand-out names the folder the AI was added with (A10.18).
  expect(handout.folder).toBe(folder);
  await withExternalMCP(node, exe, member, handout.folder, answer, working);
  const done = await node.run('solodawn', 'done', '--member', member, '--run', String(handout.run));
  expect(done.code, done.stderr).toBe(0);
}

test('external setup, usage and removal (A10.17, A10.18, A10.20, A10.25, A10.27)', async () => {
  const net = await startNet({ nodes: 2 });
  const surface = await startDirectSurface(net.rendezvous, inject('browser'));
  const browser = await launch(inject('browser'));
  try {
    const [node, second] = net.nodes;
    const context = await openDirectContext(browser, [], surface);
    const page = await context.newPage();
    await page.goto(`${surface.url}#/onboarding`);
    await pair(page, surface, node);
    await page.locator('input[name="acct-claude"][value="personal"]').check();
    await page.locator('input[name="acct-codex"][value="api_key"]').check();
    await finishOnboarding(page);
    await page.goto(`${surface.url}#/onboarding`);
    await pair(page, surface, second);
    await page.goto(`${surface.url}#/groups`);
    await page.locator('button', { hasText: 'New group' }).first().click();
    await createGroup(page, { name: 'External project', member: 'Owner' });
    const folder = await makeFolder(net.dir, 'external-project');
    await joinExternal(page, net.nodes, folder);

    const card = page.locator('#chat-side .member', { hasText: 'Scout' });
    await poll(() => card.textContent()).toMatch(/external.*ZCode/is);
    expect(await card.getByRole('button', { name: /terminal/i }).count()).toBe(0);
    const panel = page.getByRole('dialog', { name: /external AI setup/i });
    const setup = await panel.getByRole('textbox', { name: 'Self-setup prompt' }).inputValue();
    const exe = join(inject('e2e').bin, 'solodawn');
    expect(setup).toContain(exe);
    const serverText = await panel.locator('pre').last().innerText();
    const server = JSON.parse(serverText);
    const { command, args } = server.solodawn;
    expect(command).toBe(exe);
    expect(args).toEqual(['mcp', '--member', expect.any(String)]);
    const member = args[2] as string;
    expect(setup).toContain(member);

    const working = async () => {
      await poll(() => card.textContent()).toMatch(/working/i);
    };
    await poll(() => card.textContent()).toMatch(/offline/i);
    await answerCheck(node, exe, { member, folder }, 'First scripted setup reply', working);
    await poll(() => page.getByText('First scripted setup reply', { exact: true }).count()).toBe(1);
    await poll(() => card.textContent()).toMatch(/waiting/i);
    await panel.getByRole('button', { name: /send.*check/i }).click();
    await answerCheck(node, exe, { member, folder }, 'Second scripted setup reply', working);
    await poll(() => page.getByText('Second scripted setup reply', { exact: true }).count()).toBe(
      1,
    );
    await panel.getByRole('button', { name: 'Close setup' }).click();
    await poll(() => card.textContent()).toContain('40%');
    await card.getByRole('button', { name: 'Remove AI' }).click();
    await card.getByRole('button', { name: 'Confirm removal' }).click();
    await poll(() => card.count()).toBe(0);
  } finally {
    await browser.close();
    await surface.close();
    await net.stop();
  }
});
