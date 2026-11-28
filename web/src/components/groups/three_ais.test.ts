import { flushSync } from 'svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { closePage, fill, one, show } from '../../../features/page';
import { Group, Home, homes } from '../../lib/groups.svelte';
import type { Net } from '../../lib/net.svelte';
import { nodes } from '../../lib/nodes.svelte';
import { goldens } from '../../lib/mock';
import { prefs, t } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import Chat from '../chat/Chat.svelte';
import CreateGroup from './CreateGroup.svelte';

beforeEach(() => {
  prefs.lang = 'en';
});

afterEach(async () => {
  await closePage();
  nodes.infos.clear();
});

function setupCreateGroupNodes() {
  const me = {
    user: 'owner',
    sign: async (payload: string) => ({ p: payload, k: 'key', s: 'sig' }),
  };
  const info1 = p.decode(p.HostInfoPush, goldens['host-info-push']);
  const info2 = p.decode(p.HostInfoPush, goldens['host-info-push']);
  const folders = p.decode(p.HostWorkspaces, goldens['host-workspaces-answer']);
  info1.device = 'dev-ws';
  info1.agents = [
    { ...info1.agents[0], agent: 'claude', account: 'personal', groups: [] },
    { ...info1.agents[0], agent: 'codex', account: 'api_key', groups: [] },
  ];
  info2.device = 'dev-lap';
  info2.agents = [{ ...info2.agents[0], agent: 'claude', account: 'personal', groups: [] }];

  const mockCtl = (info: p.HostInfoPush) => ({
    request: vi.fn(async (req: { t: string }) =>
      req.t === 'host.workspaces' ? folders : { ...info, re: 1, ok: true },
    ) as unknown as p.Ctl['request'],
  });
  const home1 = new Home(me);
  home1.owner = true;
  home1.device = 'dev-ws';
  home1.name = 'workstation';
  home1.ctl = mockCtl(info1);
  const home2 = new Home(me);
  home2.owner = true;
  home2.device = 'dev-lap';
  home2.name = 'laptop';
  home2.ctl = mockCtl(info2);

  homes.push(home1, home2);
  nodes.infos.set(home1, info1);
  nodes.infos.set(home2, info2);
}

it('shows the 3-AI hint in CreateGroup while workingCount < 2 without blocking creation', () => {
  setupCreateGroupNodes();
  show(CreateGroup, { open: true });

  const hint = () => document.querySelector('dialog .three-ais');
  expect(hint()?.textContent).toContain('3 AIs: 1 secretary + at least 2 working AIs');
  expect(hint()?.textContent).toContain('Add AI');

  prefs.lang = 'zh-CN';
  flushSync();
  expect(hint()?.textContent).toContain('3 个 AI：1 个秘书 + 至少 2 个干活 AI');
  expect(hint()?.textContent).toContain('添加 AI');
  prefs.lang = 'en';
  flushSync();

  const submit = () => one<HTMLButtonElement>('dialog footer .btn-primary');
  const [nameInput, memberInput] = [
    ...document.querySelectorAll<HTMLInputElement>('dialog .field input.input'),
  ];
  fill(nameInput, 'Solo Group');
  fill(memberInput, 'Alex');
  flushSync();
  // Group creation stays non-blocking even with 0 AIs ticked.
  expect(submit().disabled).toBe(false);

  const ticks = () => [
    ...document.querySelectorAll<HTMLInputElement>('dialog .ai input[type=checkbox]'),
  ];
  ticks()[0].click();
  // Ticking an AI leaves its folder empty and disables Create group until a folder is chosen.
  flushSync();
  expect(submit().disabled).toBe(true);
  ticks()[1].click();
  flushSync();
  // 1 secretary + 1 working AI => workingCount = 1 < 2, hint remains visible.
  expect(hint()).not.toBeNull();

  // Ticking a 3rd AI gives 1 secretary + 2 working AIs => hint hides.
  ticks()[2].click();
  flushSync();
  expect(hint()).toBeNull();
});

it('shows the 3-AI inline prompt in Chat only when a group with AIs has < 2 working AIs', () => {
  const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
  const home = new Home({
    user: person.user,
    sign: async (payload: string) => ({ p: payload, k: 'key', s: 'sig' }),
  });
  home.ctl = { request: vi.fn().mockResolvedValue({ re: 1, ok: true }) };
  const group = new Group(home, person.g);
  const info = p.decode(p.GroupRow, goldens['row-group']);
  delete info.secretary;
  group.apply([info, person]);
  homes.push(home);

  const net: Net = { source: 'live', homes, path: () => undefined };
  show(Chat, { group, net });

  const prompt = () => document.querySelector('.need-ais');
  // 0 AIs (human-only group): does not warn about missing working AIs.
  expect(prompt()).toBeNull();

  const ai = p.decode(p.AIMemberRow, goldens['row-member-ai']);
  // 1 secretary AI + 0 working AIs: shows inline hint.
  group.apply([
    { ...info, seq: 10, secretary: ai.id },
    { ...ai, id: ai.id, name: 'Secretary', secretary: true, seq: 11 },
  ]);
  flushSync();
  expect(prompt()?.textContent).toContain('add 2 working AIs besides the secretary');

  prefs.lang = 'zh-CN';
  flushSync();
  expect(prompt()?.textContent).toContain('除了秘书还要 2 个工作 AI');
  prefs.lang = 'en';
  flushSync();

  // Add 1 working AI => workingCount = 1 < 2, still shown.
  group.apply([{ ...ai, id: 'wrk-1', name: 'Claude Code', secretary: false, seq: 12 }]);
  flushSync();
  expect(prompt()).not.toBeNull();

  // Add 2nd working AI => workingCount = 2, hint disappears.
  group.apply([{ ...ai, id: 'wrk-2', name: 'Codex', secretary: false, seq: 13 }]);
  flushSync();
  expect(prompt()).toBeNull();
});

it('explains the 3-AI rule and shared-folder choice in guide step 4, CreateGroup, and Chat', () => {
  prefs.lang = 'en';
  expect(t('guide.step.group.body')).toContain('3 AIs');
  expect(t('guide.step.group.body')).toContain('1 secretary + at least 2 working AIs');
  expect(t('guide.step.group.body')).toContain('same CLI twice');
  expect(t('guide.step.group.body')).toContain('share one folder');
  expect(t('guide.step.group.body')).toContain('take turns');
  expect(t('guide.step.group.body')).toContain('same time');
  expect(t('groups.create.ais.need')).toContain('share one folder');
  expect(t('groups.create.ais.need')).toContain('same time');
  expect(t('chat.need_working_ais')).toContain('assignee and the reviewer must differ');
  expect(t('chat.need_working_ais')).toContain('same CLI twice');

  prefs.lang = 'zh-CN';
  expect(t('guide.step.group.body')).toContain('3 个 AI');
  expect(t('guide.step.group.body')).toContain('1 个秘书 + 至少 2 个工作 AI');
  expect(t('guide.step.group.body')).toContain('再加一次');
  expect(t('guide.step.group.body')).toContain('共用一个，任务会排队');
  expect(t('guide.step.group.body')).toContain('同时干活');
  expect(t('groups.create.ais.need')).toContain('共用同一个文件夹');
  expect(t('chat.need_working_ais')).toContain('执行和复核不能是同一个');
  expect(t('chat.need_working_ais')).toContain('同一个 CLI 可以再加一次');
});
