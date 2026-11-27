// What the people and AIs of the demo world say, in Chinese, the language of its owner. The
// interface keeps its own language switch; only the content of the world is fixed.
import { devices } from './ids';
import type { Agent } from './node';

const claudeModels = [
  {
    model: 'opus',
    name: 'Opus',
    efforts: ['low', 'medium', 'high', 'xhigh', 'max'],
    tier: 'strong',
  },
  { model: 'sonnet', name: 'Sonnet', efforts: ['low', 'medium', 'high'], tier: 'standard' },
  { model: 'haiku', name: 'Haiku', efforts: [], tier: 'light' },
];
const codexModels = [
  { model: 'gpt-5.4', name: 'GPT-5.4', efforts: ['low', 'medium', 'high'], tier: 'strong' },
  { model: 'gpt-5.4-mini', name: 'GPT-5.4 mini', efforts: ['low', 'medium'], tier: 'light' },
];
const openModels = [
  { model: 'deepseek/deepseek-chat', name: 'DeepSeek Chat', efforts: [], tier: 'standard' },
  { model: 'qwen/qwen3-coder', name: 'Qwen3 Coder', efforts: [], tier: 'standard' },
];

const anyAccount = { accounts: ['personal', 'team', 'api_key'] };
const keyOnly = { accounts: ['api_key'], takeover: 'none' };

/** The four CLIs found on a computer, with the account types declared there so far. */
export function agents(accounts: Record<string, string | null>): Agent[] {
  const found = (agent: string, cli_version: string, driver: Agent['driver'], models = []) => ({
    agent,
    cli_version,
    account: accounts[agent] ?? null,
    login_ok: true,
    caps: {},
    driver,
    models: models as Agent['models'],
  });
  return [
    found('claude', '2.1.40', { ...anyAccount, takeover: 'new' }, claudeModels as never),
    found('codex', '0.51.0', { ...anyAccount, takeover: 'resume' }, codexModels as never),
    found('opencode', '1.19.6', keyOnly, openModels as never),
    found('qwen', '0.28.0', keyOnly),
  ];
}

/** The folder that each person's files are under, by computer: a folder elsewhere does not exist
 *  there. */
export const homeDirs: Record<string, string> = {
  [devices.macbook]: '/Users/alex/',
  [devices.office]: '/home/alex/',
  [devices.minaPc]: '/Users/mina/',
};

/** The folders each CLI of a computer was used in, newest first (host.workspaces). */
export const folders: Record<string, Record<string, string[]>> = {
  [devices.macbook]: {
    claude: [
      '/Users/alex/dev/solodawn-site',
      '/Users/alex/dev/blog',
      '/Users/alex/dev/design-system',
      '/Users/alex/notes',
    ],
    codex: ['/Users/alex/dev/solodawn-site', '/Users/alex/dev/api'],
  },
  [devices.office]: {
    claude: ['/home/alex/infra'],
    codex: ['/home/alex/solodawn-site', '/home/alex/reports'],
  },
  [devices.minaPc]: { claude: ['/Users/mina/work/solodawn-site'] },
};

/** The first sentence of a request, as the title of a brief. */
export function titleOf(plain: string): string {
  const first = plain.split(/[。！？!?\n，,；;]/)[0].trim() || plain;
  return first.length > 26 ? `${first.slice(0, 25)}…` : first;
}

export const words = {
  briefLead: '需求我整理成了任务说明，你看一下，确认后我就派活：',
  briefAgain: '按你的意见改好了，再看一下：',
  confirmed: '收到，已经派下去了。进度会在各自的任务里更新。',
  allDone: '这批任务都完成了。',
  closeAsk: '这个任务完成了。请按项目的做法把改动收尾，做完在这里告诉我。',
  closed: '收尾做完了，改动都在我的文件夹里。',
  oneAI: '这个群现在只有一个干活的 AI。任务要有人复核，再加一个 AI 我就能派活。',
  answer:
    '我查了一下：官网用 Vite 构建、Svelte 写页面，部署在 GitHub Pages。要我把细节整理成文档吗？',
  criteria: ['手机和电脑上显示都正常', '相关测试全部通过', '不影响现有页面'],
  scopeIn: ['前端页面', '对应的测试'],
  scopeOut: ['后端接口', '设计稿以外的样式调整'],
  testsTitle: '补测试',
  docsTitle: '写使用说明',
  start: (title: string) => `开始做「${title}」，先看一下相关代码。`,
  coded: '改好了，正在跑项目自己的测试。',
  testFailed: '有一个测试没过：手机宽度下按钮被挤到了第二行。我改一下布局再跑。',
  fixed: '修好了，测试全部通过。',
  revised: '按评审意见改好了，重新跑了测试。',
  approvalWhy: '要用真实浏览器截图来验证手机布局，需要装 Playwright。',
  approvalCommand: 'pnpm add -D @playwright/test\npnpm exec playwright install chromium',
  approved: '已经装好，用浏览器截图验证了三种屏幕宽度。',
  denied: '好的，不装新依赖。改用现有的 vitest 和 happy-dom 检查布局。',
  question: '导出的表格用什么分隔符？',
  context: '客户有的用中文版 Excel，有的用 Numbers。',
  options: [
    { id: 'A', label: '逗号（CSV）', effect: '大多数软件直接打开；中文版 Excel 可能乱码。' },
    { id: 'B', label: '制表符（TSV）', effect: '中文版 Excel 不会乱码；部分软件要手动选格式。' },
  ],
  decided: (choice: string) => `好，按「${choice}」来做。`,
  decidedText: (text: string) => `收到：「${text}」。按这个来。`,
  report:
    '做完了：改了 3 个文件，加了对应的测试，pnpm test 全部通过。改动都在我的文件夹里，主要在 src/routes/Home.svelte。',
  reviewItems: [
    {
      text: '小屏幕下的间距可以再统一一点，不影响通过。',
      veto: false,
      file: 'src/routes/Home.svelte',
      line: 42,
    },
  ],
  sentBackItems: [
    {
      text: '导出按钮在手机宽度下被挤到了第二行。',
      veto: false,
      file: 'src/routes/Home.svelte',
      line: 57,
    },
  ],
  passed: '看过了，通过。',
  sentBack: '还差一点，改完再交。',
  handback: '主人刚才在终端里直接改了几处，我已经把改动整理好，接着做剩下的部分。',
  mention: [
    '收到，我先看看相关代码，有结论了在这里回你。',
    '好的，这个我来处理，大概十分钟。',
    '看过了：问题出在缓存没有按语言区分，我会在下一个任务里一起修掉。',
  ],
  busyAnswer: '刚才在忙任务，现在回你：',
  preemptedAnswer: '先停下手上的任务回你：',
  minaAsk: '@Claude Code 帮我把登录页的报错文案改成中文吧。',
  directDone: '@Mina 登录页的报错文案已经改成中文了，一共 6 处，截图放在话题里。',
  minaThanks: '谢谢！我晚点看。',
  leoHello: '大家好，我是 Leo，负责官网的文案。',
  addedHello: '我进来了，有需要我的地方 @ 我。',
  minaWatching: 'Mina 想看你接管的终端。',
};

/** What the takeover terminal of an AI shows when it opens, by CLI. */
export const banners: Record<string, string> = {
  claude: [
    '\x1b[38;5;209m✻\x1b[0m \x1b[1mClaude Code\x1b[0m v2.1.40（演示终端）',
    '\x1b[2m  cwd: ~/dev/solodawn-site\x1b[0m',
    '',
    '\x1b[2m  会话已从任务里接过来，历史都在。输入要做的事，回车发送。\x1b[0m',
    '',
  ].join('\r\n'),
  codex: [
    '\x1b[1m>_ OpenAI Codex\x1b[0m (v0.51.0)  （演示终端）',
    '\x1b[2m  model: gpt-5.4   directory: ~/solodawn-site\x1b[0m',
    '',
  ].join('\r\n'),
};

/** What the AI answers in the terminal to a line the person typed. */
export const terminalReply = (line: string) =>
  [
    '',
    `\x1b[38;5;209m●\x1b[0m 好的，我来处理：「${line}」`,
    '  \x1b[2m⎿  Read src/routes/Home.svelte (128 lines)\x1b[0m',
    '  \x1b[2m⎿  Update src/routes/Home.svelte: +6 -2\x1b[0m',
    '\x1b[38;5;209m●\x1b[0m 改好了。还有别的吗？',
    '',
  ].join('\r\n');

/** The lines Mina's terminal shows, one after another, to someone who watches. */
export const minaScreen = [
  '\x1b[1m✻ Claude Code\x1b[0m v2.1.40 — Mina 的笔记本',
  '',
  '\x1b[36m> \x1b[0m把登录页的报错文案统一成中文，语气友好一点',
  '',
  '\x1b[38;5;209m●\x1b[0m 我先找出所有报错文案。',
  '  \x1b[2m⎿  Search "error" in src/routes/Login.svelte (6 matches)\x1b[0m',
  '\x1b[38;5;209m●\x1b[0m 共 6 处。先改「密码错误」这一条：',
  '  \x1b[2m⎿  Update src/routes/Login.svelte: +1 -1\x1b[0m',
  '\x1b[32m  - Wrong password.\x1b[0m',
  '\x1b[32m  + 密码不对，再试一次？\x1b[0m',
  '\x1b[38;5;209m●\x1b[0m 接下来改网络错误和账号锁定的提示。',
  '  \x1b[2m⎿  Update src/routes/Login.svelte: +5 -5\x1b[0m',
  '\x1b[38;5;209m●\x1b[0m 全部改完，跑一下测试。',
  '  \x1b[2m⎿  pnpm test login — 12 passed\x1b[0m',
  '\x1b[36m> \x1b[0m',
];
