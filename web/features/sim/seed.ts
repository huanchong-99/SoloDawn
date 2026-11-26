// The computers of the demo world and, unless it starts empty for onboarding, its two groups with
// a short history: a group with a secretary and three working AIs, each in its folder on its
// computer, where one task was done yesterday, and a group of people only that the person shares
// with Mina.
import { agents, words } from './content';
import { devices, ids, users } from './ids';
import { ai, group, msg, person, system, task, type AISpec, type Row } from './rows';
import type { World } from './world';

const hour = 3_600_000;
const minute = 60_000;

/** The AIs of the website group, by member id. The secretary and Claude Code share one folder. */
export function siteAIs(me: string): AISpec[] {
  const mac = {
    host: devices.macbook,
    hostName: 'MacBook Pro',
    user: me,
    workspace: '/Users/alex/dev/solodawn-site',
  };
  return [
    {
      ...mac,
      id: ids.secretary,
      name: '秘书',
      cli: 'claude',
      secretary: true,
      model: 'opus',
      effort: 'high',
      tier: 'strong',
      skills: '拆需求、派活、盯进度',
    },
    {
      ...mac,
      id: ids.claude,
      name: 'Claude Code',
      cli: 'claude',
      model: 'sonnet',
      effort: 'medium',
      skills: '前端、Svelte',
    },
    {
      id: ids.codex,
      name: 'Codex',
      cli: 'codex',
      user: me,
      host: devices.office,
      hostName: '办公室台式机',
      workspace: '/home/alex/solodawn-site',
      account: 'team',
      model: 'gpt-5.4',
      effort: 'high',
      tier: 'strong',
      skills: '测试、文档',
    },
    {
      id: ids.minaAi,
      name: 'Mina 的 Claude',
      cli: 'claude',
      user: users.mina,
      host: devices.minaPc,
      hostName: 'Mina 的笔记本',
      workspace: '/Users/mina/work/solodawn-site',
      model: 'sonnet',
    },
  ];
}

/** Adds the three computers: two of the person's own, and Mina's. Before onboarding no account
 *  type is declared on the person's computers yet. */
export function computers(world: World) {
  const declared = (accounts: Record<string, string>) =>
    agents(world.mode === 'full' ? accounts : {});
  const mac = world.add({
    device: devices.macbook,
    name: 'MacBook Pro',
    os: 'macOS 15.3',
    owner: world.me,
    agents: declared({ claude: 'personal', codex: 'personal', qwen: 'api_key' }),
  });
  const office = world.add({
    device: devices.office,
    name: '办公室台式机',
    os: 'Ubuntu 24.04',
    owner: world.me,
    agents: declared({ claude: 'team', codex: 'team' }),
  });
  world.add({
    device: devices.minaPc,
    name: 'Mina 的笔记本',
    os: 'macOS 15.1',
    owner: users.mina,
    agents: agents({ claude: 'personal' }),
  });
  return { mac, office };
}

/** The world of `?demo`: both computers paired, both groups with their history. */
export function seed(world: World, now = Date.now()) {
  const { mac, office } = computers(world);
  mac.paired = office.paired = true;
  site(world, now);
  talk(world, now);
}

const at = (now: number, ago: number) => ({ ts: now - ago });

function site(world: World, now: number) {
  const g = ids.site;
  const home = world.node(devices.macbook)!;
  home.groups.set(g, { name: '官网改版', seq: 0, rows: new Map() });
  const spec = {
    g,
    name: '官网改版',
    home: devices.macbook,
    secretary: ids.secretary,
    created: now - 50 * hour,
  };
  world.write(g, group(spec));
  world.write(g, person(g, ids.me, world.me, 'Alex', 'owner'));
  world.write(g, person(g, ids.mina, users.mina, 'Mina', 'member'));
  for (const each of siteAIs(world.me))
    world.write(g, ai(g, each, each.id === ids.minaAi ? 'takeover' : 'waiting'));
  yesterday(world, now - 24 * hour);
  today(world, now);
}

/** The first request and its one task, done. */
function yesterday(world: World, day: number) {
  const g = ids.site;
  const ask = msg('text', {
    g,
    by: ids.me,
    text: '@秘书 首页的价格表在手机上会横向溢出，看看怎么修。',
    mentions: [ids.secretary],
    ...at(day, 0),
  });
  world.write(g, ask);
  const brief = briefRow(world, day);
  world.write(g, brief);
  done(world, day, brief.id as string);
}

function briefRow(world: World, day: number): Row {
  const g = ids.site;
  const body = {
    title: '修复手机上价格表溢出',
    goal: '价格表在 375 像素宽的手机上不出现横向滚动条，三档价格都能看全。',
    criteria: ['375 像素宽时没有横向滚动条', '三档价格都能完整看到', '电脑上的样子不变'],
    scope: { in: ['首页价格表', '对应的测试'], out: ['价格本身', '其他页面'] },
    tasks: [
      {
        title: '价格表在小屏上改成纵向卡片',
        assignee: ids.claude,
        reviewer: ids.codex,
        foundation: false,
        depends: [],
      },
    ],
    pass: 90,
    requester: ids.me,
    replaces: null,
  };
  const act = { p: '{}', k: world.me, s: 'demo' };
  const state = { status: 'confirmed', by: ids.me, at: day + 3 * minute, act };
  return msg(
    'brief',
    { g, by: ids.secretary, text: words.briefLead, ...at(day, -minute) },
    body,
    state,
  );
}

/** Yesterday's task: worked on, reported, passed its review, done, summed up. */
function done(world: World, day: number, brief: string) {
  const g = ids.site;
  const made = task({
    g,
    num: 1,
    title: '价格表在小屏上改成纵向卡片',
    assignee: ids.claude,
    reviewer: ids.codex,
    brief,
  });
  const id = made.id as string;
  world.write(g, msg('task', { g, by: 'system', id, ...at(day, -4 * minute) }, { task: id }));
  const thread = (by: string, ago: number) => ({ g, by, thread: id, ...at(day, -ago * minute) });
  world.write(
    g,
    msg('progress', { ...thread(ids.claude, 6), text: '开始做，先看一下价格表的样式。' }),
  );
  const report = msg('report', { ...thread(ids.claude, 21), text: words.report(1) });
  world.write(g, report);
  const review = {
    task: id,
    num: 1,
    title: made.title,
    report: report.id,
    round: 1,
    score: 94,
    pass: 90,
    ok: true,
    items: [],
  };
  world.write(
    g,
    msg('review', { ...thread(ids.codex, 30), text: '改得干净，小屏和大屏都看过了。' }, review),
  );
  world.write(g, { ...system(g, { code: 'task_done', task: id }, id), ts: day + 30 * minute });
  world.write(g, finished(made, report.id, day));
  const summary = { task: id, score: 94, rounds: 1, minutes: 26 };
  world.write(g, msg('summary', { ...thread(ids.secretary, 31), main: true }, summary));
}

function finished(made: Row, report: string, day: number): Row {
  return {
    ...made,
    state: 'done',
    step: 'review',
    report,
    review: {
      round: 1,
      failed: 0,
      score: 94,
      ok: true,
      by: ids.codex,
      report,
      at: day + 30 * minute,
    },
    thread: { count: 5, rounds: 0 },
    stats: { started: day + 4 * minute, ended: day + 30 * minute },
    created: day + 4 * minute,
  };
}

function today(world: World, now: number) {
  const g = ids.site;
  world.write(
    g,
    msg('text', {
      g,
      by: ids.mina,
      text: '首页的新插画我放在 design/hero-v2.png 了，有空看看。',
      ...at(now, 20 * hour),
    }),
  );
  const lighthouse =
    '@Alex 我跑了一下 Lighthouse：性能 92 分，主要慢在字体加载。要不要换成系统字体？';
  world.write(
    g,
    msg('text', {
      g,
      by: ids.codex,
      text: lighthouse,
      mentions: [ids.me],
      ...at(now, 95 * minute),
    }),
  );
  world.write(
    g,
    msg('text', { g, by: ids.me, text: '先不换，等下周设计定稿再说。', ...at(now, 80 * minute) }),
  );
  const ask = msg('text', {
    g,
    by: ids.mina,
    text: words.minaAsk,
    mentions: [ids.claude],
    ...at(now, 12 * minute),
  });
  world.write(g, ask);
  const body = { what: 'direct', ai: ids.claude, to: [world.me] };
  const card = { g, by: 'system', main: true, re: ask.id as string, ...at(now, 12 * minute) };
  world.write(g, msg('confirm', card, body, { status: 'pending' }));
}

/** A group of people only, homed on the office computer: Mina is in it, so she can be added to a
 *  new group made there without an invitation. */
function talk(world: World, now: number) {
  const g = ids.talk;
  const home = world.node(devices.office)!;
  home.groups.set(g, { name: '产品讨论', seq: 0, rows: new Map() });
  world.write(g, group({ g, name: '产品讨论', home: devices.office, created: now - 72 * hour }));
  world.write(g, person(g, ids.me, world.me, 'Alex', 'owner'));
  world.write(g, person(g, ids.minaTalk, users.mina, 'Mina', 'admin'));
  const demoDay = '下周三要给客户演示新官网，在那之前大家别合并大改动。';
  world.write(g, msg('text', { g, by: ids.minaTalk, text: demoDay, ...at(now, 26 * hour) }));
  const news = '「官网改版」T-1 已完成：价格表在手机上不再溢出。';
  world.write(g, msg('text', { g, by: ids.me, text: news, ...at(now, 23 * hour) }));
  world.write(g, msg('text', { g, by: ids.me, text: '收到，我会盯着。', ...at(now, 22 * hour) }));
}
