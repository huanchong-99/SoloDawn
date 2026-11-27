import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { all, closePage, one, show } from '../../../features/page';
import { Group, Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import WaitingBar from './WaitingBar.svelte';

beforeEach(() => {
  prefs.lang = 'en';
  Element.prototype.scrollIntoView = vi.fn();
});
afterEach(closePage);

it('jumps to the first card that waits when the label is clicked, as its chip does', () => {
  const human = p.decode(p.HumanMemberRow, goldens['row-member-human']);
  const approval = p.decode(p.MsgRow, goldens['row-msg-approval-pending']);
  const me = {
    user: human.user,
    sign: async (payload: string) => ({ p: payload, k: 'k', s: 's' }),
  };
  const group = new Group(new Home(me), human.g);
  group.apply([
    human,
    { ...approval, id: 'second', seq: 800, pos: 800 },
    { ...approval, id: 'first', seq: 700, pos: 700 },
  ]);
  const cards = ['first', 'second'].map((id) => {
    const card = document.body.appendChild(document.createElement('div'));
    card.id = `m-${id}`;
    card.append(document.createElement('button'));
    return card;
  });
  show(WaitingBar, { group });
  expect(one('nav.needbar b').textContent?.trim()).toBe('Waiting on you · 2');
  expect(all('nav.needbar a').map((link) => link.getAttribute('href'))).toEqual([
    '#m-first',
    '#m-first',
    '#m-second',
  ]);
  one<HTMLAnchorElement>('nav.needbar b a').click();
  expect(document.activeElement).toBe(cards[0].querySelector('button'));
  expect(location.hash).not.toBe('#m-first');
});
