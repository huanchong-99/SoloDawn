import type { Browser, Locator } from 'playwright';
import { createServer, type ViteDevServer } from 'vite';
import { afterAll, afterEach, beforeAll, expect, inject, test } from 'vitest';
import { launch } from './config';
import type {} from './external-fixture';

const fixture = '/e2e/external-fixture.ts';
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

const layouts = [
  { width: 1280, lang: 'en', theme: 'light' },
  { width: 390, lang: 'zh-CN', theme: 'dark' },
];

const expected = {
  add: ['Add AI', '添加 AI'],
  submit: ['Add AI', '添加'],
  changeFolder: ['Change the folder of Scout', '更改 Scout 的文件夹'],
  useFolder: ['Use this folder', '用这个文件夹'],
  external: ['External AI', '外部 AI'],
  name: ['Name in the group', '在群里的名字'],
  form: ['Add an AI', '添加 AI 到本群'],
  model: ['Model', '模型'],
  setup: ['External AI setup', '设置外部 AI'],
  open: ['Setup', '设置'],
  close: ['Close setup', '关闭设置'],
  resend: ['Send setup check again', '再次发送连接检查'],
  remove: ['Remove AI', '移除 AI'],
  confirm: ['Confirm removal', '确认移除'],
  unset: ['Not set', '未设置'],
  terminal: ['Open terminal', '打开终端'],
};

async function fits(dialog: Locator, width: number) {
  const box = (await dialog.boundingBox())!;
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(width);
  const content = await dialog.evaluate((el) => [el.scrollWidth, el.clientWidth]);
  expect(content[0]).toBeLessThanOrEqual(content[1]);
}

test.each(layouts)(
  'adds, checks, and removes an external AI at $width px in $lang $theme',
  async ({ width, lang, theme }) => {
    const label = (key: keyof typeof expected) => expected[key][lang === 'en' ? 0 : 1];
    const page = await engine.newPage({ viewport: { width, height: 900 } });
    page.setDefaultTimeout(8_000);
    await page.addInitScript(
      ({ lang, theme }) => {
        localStorage.setItem('sd-lang', lang);
        localStorage.setItem('sd-theme', theme);
      },
      { lang, theme },
    );
    await page.goto(`${server.resolvedUrls!.local[0]}?nomock`);
    await page
      .getByRole('button', { name: lang === 'en' ? 'Look around first' : '先随便看看' })
      .click();
    await page.addScriptTag({ type: 'module', url: fixture });
    await page.waitForFunction(() => window.externalCast);
    const { g, id, exe, host } = await page.evaluate(() => window.externalCast.cast());
    await page.evaluate((g) => (location.hash = `#/groups/${g}`), g);
    if (width === 390) {
      await page
        .getByRole('button', { name: lang === 'en' ? 'People and details' : '成员和群资料' })
        .click();
    }

    await page.getByRole('button', { name: label('add'), exact: true }).click();
    const add = page.getByRole('dialog', { name: label('form') });
    await add.getByRole('radio', { name: new RegExp(`${label('external')} · Laptop`) }).waitFor();
    await add
      .getByRole('radio', { name: new RegExp(`${label('external')} · MCP workstation`) })
      .check();
    await add.getByRole('textbox', { name: label('name') }).fill('Scout');
    await add.getByRole('textbox', { name: /Tool label|工具标签/ }).fill('ZCode');
    await add.getByRole('textbox', { name: label('model'), exact: true }).fill('Model X');
    expect(await add.getByRole('checkbox').count()).toBe(0);
    const account = add.getByRole('combobox', { name: /^(Account|账号)$/ });
    expect(await account.evaluate((el: HTMLSelectElement) => el.validity.valueMissing)).toBe(true);
    await account.selectOption('api_key');
    await expect.poll(() => add.locator('.path-row code').textContent()).toBe('/home/dev/website');
    await add.getByRole('button', { name: label('changeFolder'), exact: true }).click();
    await page.getByRole('button', { name: label('useFolder'), exact: true }).click();
    await fits(add, width);
    await add.getByRole('button', { name: label('submit'), exact: true }).click();
    const joined = () => page.evaluate(() => window.externalCast.join());
    await expect.poll(joined).toMatchObject({
      t: 'host.join',
      agent: 'external',
      to: host,
      name: 'Scout',
      // The newest folder its computer lists, pre-highlighted and confirmed in the folder dialog.
      workspace: '/home/dev/website',
      account: 'api_key',
      tool: 'ZCode',
      secretary: false,
      model: 'Model X',
      tier: 'standard',
    });
    const publish = (state: string, usage = {}) =>
      page.evaluate(({ state, usage }) => window.externalCast.publish(state, usage), {
        state,
        usage,
      });
    await publish('offline');

    const setup = page.getByRole('dialog', { name: `${label('setup')} · Scout` });
    await setup.waitFor();
    await fits(setup, width);
    expect(JSON.parse((await setup.locator('pre').textContent())!)).toEqual({
      solodawn: { command: exe, args: ['mcp', '--member', id] },
    });
    const textbox = setup.getByRole('textbox');
    expect(await textbox.getAttribute('rows')).toBe('10');
    expect(
      await setup
        .locator('.marks')
        .evaluate((el) => getComputedStyle(el).gridTemplateColumns.split(' ')),
    ).toHaveLength(2);
    const prompt = await textbox.inputValue();
    expect(prompt).toContain(
      'You are joining the SoloDawn group "Website" as the AI member "Scout".',
    );
    expect(prompt).toContain(`Its program is: ${exe}\nYour member id is: ${id}`);
    const commands = () => page.evaluate(() => window.externalCast.groupCommands());
    const check = {
      t: 'post',
      g,
      kind: 'text',
      thread: null,
      main: false,
      text: 'Setup check: answer here in one line, then finish.',
      mentions: [id],
      re: null,
      body: {},
    };
    await expect.poll(commands).toEqual([check]);
    const marks = setup.locator('li.done');
    expect(await marks.count()).toBe(0);
    await publish('waiting');
    await expect.poll(() => marks.count()).toBe(1);
    await publish('working');
    await expect.poll(() => marks.count()).toBe(2);
    await page.evaluate(() => window.externalCast.answer());
    await expect.poll(() => marks.count()).toBe(3);
    await publish('waiting', { window_pct: 40, window_resets: 1763611200000 });
    await expect.poll(() => marks.count()).toBe(4);

    await setup.getByRole('button', { name: label('close') }).click();
    const card = page.locator('.members .member', { hasText: 'Scout' });
    expect(await card.getByText(`${label('external')} · ZCode`).count()).toBe(1);
    expect(await card.getByText(label('unset'), { exact: true }).count()).toBe(2);
    expect(await card.getByRole('meter').getAttribute('aria-valuenow')).toBe('40');
    await publish('offline', { window_pct: 40 });
    await expect.poll(() => card.locator('.st').getAttribute('data-st')).toBe('offline');
    expect(await card.getByText(label('terminal')).count()).toBe(0);
    const open = card.getByRole('button', { name: label('open'), exact: true });
    await open.click();
    await setup.getByRole('button', { name: label('resend') }).click();
    await expect.poll(commands).toEqual([check, check]);
    await expect.poll(() => marks.count()).toBe(0);
    await setup.getByRole('button', { name: label('close') }).click();
    const remove = card.getByRole('button', { name: label('remove'), exact: true });
    const confirm = card.getByRole('button', { name: label('confirm'), exact: true });
    await remove.click();
    await confirm.press('Escape');
    await remove.waitFor();
    await remove.click();
    await confirm.waitFor();
    expect(await commands()).toEqual([check, check]);
    await confirm.click();
    await expect.poll(commands).toEqual([check, check, { t: 'member', g, op: 'remove', id }]);
    await card.waitFor({ state: 'detached' });
  },
);
