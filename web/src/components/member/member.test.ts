import { flushSync, tick } from 'svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { all, closePage, one, show, texts } from '../../../features/page';
import { Group, Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import MemberPanel from './MemberPanel.svelte';

const human = p.decode(p.HumanMemberRow, goldens['row-member-human']);
const golden = p.decode(p.AIMemberRow, goldens['row-member-ai']);

/** A group's panel, read by its owner or, with `role`, by another person of that role, who owns
 *  none of the AIs. */
function setup(changes: Partial<p.AIMemberRow> = {}, role?: string) {
  const home = new Home({
    user: role ? 'reader' : human.user,
    sign: async (payload) => ({ p: payload, k: 'key', s: 'signature' }),
  });
  const group = new Group(home, human.g);
  const ai: p.AIMemberRow = { ...golden, user: human.user, secretary: false, ...changes };
  const reader = { ...human, id: 'reader', user: 'reader', name: 'Reader', role: role ?? 'member' };
  group.apply([p.decode(p.GroupRow, goldens['row-group']), human, reader, ai]);
  home.groups = [group];
  const request = vi.fn().mockResolvedValue({ re: 1, ok: true });
  home.ctl = { request: request as p.Ctl['request'] };
  show(MemberPanel, { group, net: { source: 'mock', homes: [], path: () => undefined } });
  return { group, ai, home, request };
}

const removal = () =>
  all<HTMLButtonElement>('.member .card-foot button').find((each) =>
    ['Remove AI', 'Confirm removal'].includes(each.textContent!.trim()),
  )!;

async function press() {
  removal().click();
  await tick();
}

beforeEach(() => {
  prefs.lang = 'en';
});
afterEach(closePage);

it.each(['claude', 'codex', 'opencode', 'qwen', 'external'])(
  'removes a %s AI after a second press by its owner',
  async (cli) => {
    const { group, ai, request } = setup({ cli });
    await press();
    expect(removal().textContent!.trim()).toBe('Confirm removal');
    expect(removal().classList.contains('btn-danger')).toBe(true);
    expect(texts('.card-foot .why')).toEqual([]);
    await press();
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    const { n: _n, ts: _ts, ...command } = JSON.parse(request.mock.calls[0][0].env.p);
    expect(command).toEqual({ t: 'member', g: group.g, op: 'remove', id: ai.id });
    expect(removal().textContent!.trim()).toBe('Remove AI');
  },
);

it('lets an admin remove the AI of another person, and no other reader', async () => {
  const { group, request } = setup({ state: 'working' }, 'admin');
  expect(texts('.card-foot button')).toEqual(['Remove AI']);
  await press();
  await press();
  await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));
  group.apply([{ ...human, id: 'reader', user: 'reader', role: 'member', seq: 10000 }]);
  flushSync();
  expect(all('.card-foot')).toEqual([]);
});

it('takes the first press back on Escape, and is off while the home cannot be reached', async () => {
  const { home, request } = setup();
  const escape = () =>
    new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
  const idle = escape();
  removal().dispatchEvent(idle);
  expect(idle.defaultPrevented).toBe(false);
  await press();
  removal().dispatchEvent(new KeyboardEvent('keydown', { key: 'a', bubbles: true }));
  await tick();
  expect(removal().textContent!.trim()).toBe('Confirm removal');
  const back = escape();
  removal().dispatchEvent(back);
  await tick();
  expect(back.defaultPrevented).toBe(true);
  expect(removal().textContent!.trim()).toBe('Remove AI');
  expect(request).not.toHaveBeenCalled();
  home.ctl = undefined;
  flushSync();
  expect(removal().disabled).toBe(true);
});

it('says on the first press that an AI on unfinished tasks cannot be removed, and which', async () => {
  const { group, ai, request } = setup({ name: '$& Codex' });
  const task = p.decode(p.TaskRow, goldens['row-task']);
  const on = (id: string, num: number) => ({ ...task, id, num, assignee: ai.id, seq: 20000 + num });
  group.apply([on('t-a', 12), on('t-b', 14)]);
  await press();
  await press();
  expect(removal().textContent!.trim()).toBe('Remove AI');
  expect(texts('.card-foot [role=alert]')).toEqual([
    '$& Codex is on T-12 and T-14. Reassign or cancel that work first.',
  ]);
  expect(one('.card-foot .why').id).toBe(removal().getAttribute('aria-describedby'));
  expect(request).not.toHaveBeenCalled();
  // Once the tasks are over the same place asks for the second press.
  group.apply([
    { ...on('t-a', 12), state: 'merged', seq: 30000 },
    { ...on('t-b', 14), state: 'cancelled', seq: 30001 },
  ]);
  flushSync();
  expect(removal().textContent!.trim()).toBe('Confirm removal');
});

it('says what removing the secretary means, and shows a refusal of the home until the next try', async () => {
  const { request } = setup({ secretary: true });
  await press();
  expect(texts('.card-foot .why')).toEqual([
    'The group will have no secretary until you choose another one.',
  ]);
  request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden', msg: '' } });
  await press();
  await vi.waitFor(() =>
    expect(texts('.card-foot [role=alert]')).toEqual(['Not sent: this device may not do that.']),
  );
  await press();
  expect(texts('.card-foot [role=alert]')).toEqual([]);
});

it('shows the turns of today as plain text and a share of the limit as a meter', () => {
  const { group, ai } = setup({ usage: { turns_today: 14 }, last_seen: Date.now() });
  expect(texts('.member .meter')).toEqual(['Turns today14']);
  expect(all('.member [role=meter]')).toEqual([]);
  group.apply([{ ...ai, usage: { turns_today: 15, window_pct: 40 }, seq: ai.seq + 1 }]);
  flushSync();
  expect(texts('.member [role=meter]')).toEqual(['Window used40%']);
});
