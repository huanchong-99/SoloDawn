// Screenshots of the group list and the chat for a pull request: the design's four widths in
// both themes, one sheet per screen. Run in web/ with
//   node features/shots.ts <directory> [screen ...]
// With no screen named it takes them all. `list`, `chat` and `create` are taken at every width;
// `mention`, `long` and `readonly` are states of the chat, taken at one width. A screen named
// with a width and a theme, as `chat:820:light`, is one picture.
// It starts a development server, loads features/demo.ts into the page and uses the Chromium
// that `pnpm exec playwright install --only-shell chromium` installs.
import { mkdir, readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { chromium, type Browser, type Page } from 'playwright';
import { createServer } from 'vite';

const out = process.argv[2] ?? 'shots';
const widths = [375, 820, 1100, 1440];
const schemes = ['light', 'dark'] as const;
// The time of the mock-ups, so that a sheet reads the same whenever it is made.
const now = new Date('2026-11-20T11:44:00+07:00');

type Demo = { ids: Record<string, string> };

/** A page of the given width that shows the demo's groups, with nothing moving any more. */
async function open(browser: Browser, url: string, width: number, scheme: 'light' | 'dark') {
  const page = await browser.newPage({
    viewport: { width, height: width < 768 ? 812 : 900 },
    colorScheme: scheme,
    locale: 'zh-CN',
    timezoneId: 'Asia/Bangkok',
  });
  await page.clock.setFixedTime(now);
  await page.goto(`${url}?nomock#/groups`);
  const demo = await page.evaluate(async () => {
    const { demo } = (await import('/features/demo.ts' as string)) as {
      demo: () => Promise<Demo>;
    };
    return (await demo()).ids;
  });
  return { page, ids: demo };
}

/** Waits until what arrives has arrived; what breathes for ever is left alone. */
async function still(page: Page) {
  await page.evaluate(async () => {
    await document.fonts.ready;
    await new Promise((frame) => requestAnimationFrame(frame));
    const ending = document
      .getAnimations()
      .filter((animation) => animation.effect?.getComputedTiming().endTime !== Infinity);
    await Promise.all(ending.map((animation) => animation.finished.catch(() => undefined)));
  });
}

const screens: Record<string, (page: Page, ids: Record<string, string>) => Promise<void>> = {
  list: async () => {},
  chat: async (page, ids) => {
    await page.evaluate((g) => (location.hash = `#/groups/${g}`), ids.rebuild);
    await page.getByRole('region', { name: '主线' }).waitFor();
  },
  create: async (page) => {
    await page.getByRole('button', { name: '新建项目群' }).first().click();
    await page.getByLabel('群名称').fill('文档站');
    await page.getByLabel('Git 仓库地址').fill('https://github.com/alex/docs.git');
    await page.getByLabel('你在群里的名字').fill('Alex');
    await page.getByLabel('检查命令').fill('pnpm check\npnpm test');
  },
  mention: async (page, ids) => {
    await screens.chat(page, ids);
    await page.getByRole('combobox', { name: '发消息' }).pressSequentially('请 @c');
    await page.getByRole('listbox').waitFor();
  },
  long: async (page, ids) => {
    await screens.chat(page, ids);
    await page.getByRole('combobox', { name: '发消息' }).fill('日志很长。'.repeat(450));
  },
  readonly: async (page, ids) => {
    await page.evaluate((g) => (location.hash = `#/groups/${g}`), ids.phone);
    await page.getByRole('region', { name: '主线' }).waitFor();
  },
};

/** One sheet: each picture under its name, the two themes as rows. */
async function sheet(browser: Browser, name: string, rows: string[][]) {
  const page = await browser.newPage({ viewport: { width: 800, height: 600 } });
  const cell = async (file: string) => {
    const data = (await readFile(join(out, `${file}.png`))).toString('base64');
    return `<figure><img src="data:image/png;base64,${data}"><figcaption>${file}</figcaption></figure>`;
  };
  const html = await Promise.all(
    rows.map(async (row) => `<div>${(await Promise.all(row.map(cell))).join('')}</div>`),
  );
  await page.setContent(
    `<style>body{margin:24px;width:max-content;background:#888;font:14px system-ui;color:#fff}
     div{display:flex;gap:24px;align-items:flex-start;margin-bottom:24px}figure{margin:0}
     img{display:block;margin-bottom:6px}</style>${html.join('')}`,
  );
  await page.screenshot({ path: join(out, `${name}.png`), fullPage: true });
  await page.close();
}

await mkdir(out, { recursive: true });
const server = await createServer({ server: { host: '127.0.0.1', port: 0 }, logLevel: 'silent' });
await server.listen();
const url = server.resolvedUrls!.local[0];
const browser = await chromium.launch();

async function shoot(screen: string, width: number, scheme: 'light' | 'dark') {
  const { page, ids } = await open(browser, url, width, scheme);
  await screens[screen](page, ids);
  await still(page);
  const file = `${screen}-${width}-${scheme}`;
  await page.screenshot({ path: join(out, `${file}.png`) });
  await page.close();
  return file;
}

const named = process.argv.slice(3).filter((name) => !name.includes(':'));
const single = process.argv.slice(3).filter((name) => name.includes(':'));
const wanted = (screen: string) =>
  named.length === 0 ? single.length === 0 : named.includes(screen);

try {
  for (const screen of ['list', 'chat', 'create'].filter(wanted)) {
    const rows: string[][] = [];
    for (const scheme of schemes) {
      const row: string[] = [];
      for (const width of widths) row.push(await shoot(screen, width, scheme));
      rows.push(row);
    }
    await sheet(browser, `sheet-${screen}`, rows);
  }
  const states = [
    ['mention', 375],
    ['mention', 1100],
    ['long', 820],
    ['readonly', 820],
  ] as const;
  const taken: string[][] = [];
  for (const scheme of schemes) {
    const row: string[] = [];
    for (const [screen, width] of states)
      if (wanted(screen)) row.push(await shoot(screen, width, scheme));
    taken.push(row);
  }
  if (taken[0].length) await sheet(browser, 'sheet-states', taken);
  for (const name of single) {
    const [screen, width, scheme] = name.split(':');
    await shoot(screen, Number(width), scheme === 'dark' ? 'dark' : 'light');
  }
} finally {
  await browser.close();
  await server.close();
}
