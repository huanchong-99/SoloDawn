import { flushSync, tick } from 'svelte';
import { prefersReducedMotion } from 'svelte/motion';
import { afterEach, expect, it, vi } from 'vitest';
import { closePage, field, one, openPage, press, show, text, type } from '../../../features/page';
import { Group, Home, homes } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { cards } from '../../lib/mounts.svelte';
import * as p from '../../lib/proto';
import Message from '../chat/Message.svelte';
import TerminalInfo from '../terminal/TerminalInfo.svelte';

afterEach(closePage);

function setup() {
  const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
  const home = new Home({ user: person.user, sign: async () => ({ p: '', k: '', s: '' }) });
  const group = new Group(home, person.g);
  group.apply([person, { ...p.decode(p.GroupRow, goldens['row-group-team']), id: group.g }]);
  home.groups = [group];
  home.ctl = { request: vi.fn().mockResolvedValue({ re: 1, ok: true }) as p.Ctl['request'] };
  homes.push(home);
  return { group, home };
}

it('shows the empty team main line when no older messages remain', () => {
  const { group } = setup();
  group.paging().more = false;
  openPage(`#/groups/${group.g}`);
  expect(text('.empty h3')).toBe('Nothing here yet');
  expect(text('.empty p')).toBe(
    'Write to the whole team. Notices that secretaries post show up here too.',
  );
  expect(text('.edge')).toBe('');
});

it('marks new messages while reading back, and returns to the latest message', () => {
  const { group } = setup();
  group.apply([p.decode(p.MsgRow, goldens['row-msg-bulletin'])]);
  openPage(`#/groups/${group.g}`);
  const scroller = one('.scroller');
  const scroll = vi.spyOn(scroller, 'scrollTo').mockImplementation(() => {});
  scroller.scrollTop = -300;
  scroller.dispatchEvent(new Event('scroll'));
  flushSync();
  expect(text('.latest')).toBe('Back to the latest');
  const msg = p.decode(p.MsgRow, goldens['row-msg-bulletin']);
  group.apply([{ ...msg, id: 'next', pos: msg.pos + 1, seq: msg.seq + 1 }]);
  flushSync();
  expect(text('.latest')).toBe('New messages');
  expect(document.querySelectorAll('.latest .dot').length).toBe(1);
  one<HTMLButtonElement>('.latest').click();
  flushSync();
  expect(scroll.mock.calls).toEqual([[{ top: 0, behavior: 'smooth' }]]);
  expect(document.activeElement).toBe(scroller);
  vi.spyOn(prefersReducedMotion, 'current', 'get').mockReturnValue(true);
  one<HTMLButtonElement>('.latest').click();
  expect(scroll.mock.calls).toEqual([
    [{ top: 0, behavior: 'smooth' }],
    [{ top: 0, behavior: 'auto' }],
  ]);
});

it('closes the team member drawer with its toggle', async () => {
  const { group } = setup();
  openPage(`#/groups/${group.g}`, 800);
  one<HTMLButtonElement>('.side-open').click();
  flushSync();
  await tick();
  expect(one('.side-open').getAttribute('aria-expanded')).toBe('true');
  one<HTMLButtonElement>('.side-open').click();
  flushSync();
  await tick();
  expect(one('.side-open').getAttribute('aria-expanded')).toBe('false');
});

it('starts mentions, selects people with the mouse and handles all completion keys', async () => {
  const { group } = setup();
  const ai = p.decode(p.AIMemberRow, goldens['row-member-ai']);
  group.apply([{ ...ai, name: 'Helper', cli: 'other', secretary: true }]);
  openPage(`#/groups/${group.g}`);
  one<HTMLButtonElement>('.composer button[aria-label="Mention a person or an AI"]').click();
  flushSync();
  await tick();
  expect(field().value).toBe('@');
  type('@Helper');
  expect(text('[role=option] .badge')).toBe('Secretary AI');
  const option = one<HTMLButtonElement>('[role=option]');
  const down = new MouseEvent('mousedown', { bubbles: true, cancelable: true });
  option.dispatchEvent(down);
  expect(down.defaultPrevented).toBe(true);
  option.click();
  flushSync();
  await tick();
  expect(field().value).toBe('@Helper ');
  type('@owner');
  expect(text('[role=option] .cut')).toBe('owner');
  press('x');
  expect(document.querySelectorAll('[role=option]').length).toBe(1);
  press('Escape');
  expect(document.querySelectorAll('[role=option]').length).toBe(0);
  type('say @owner');
  press('Tab');
  await tick();
  expect(field().value).toBe('say @owner ');
});

it('shows a task event after its task has disappeared', () => {
  const { group } = setup();
  const msg = p.decode(p.MsgRow, goldens['row-msg-task']);
  const taskCard = cards.get('task');
  (cards as Map<string, typeof taskCard>).delete('task');
  show(Message, { group, msg, joined: false });
  expect(document.querySelector('.note .cut')).toBeNull();
  expect(text('.note .badge')).toBe('Task');
  if (taskCard) (cards as Map<string, typeof taskCard>).set('task', taskCard);
});

it('uses the AI name as initials for an unrecognised CLI in terminal information', () => {
  const ai = { ...p.decode(p.AIMemberRow, goldens['row-member-ai']), cli: 'other' };
  show(TerminalInfo, { ai, task: undefined, link: { st: 'ok', text: 'Direct' } });
  expect(text('.who .av')).toBe('C');
});
