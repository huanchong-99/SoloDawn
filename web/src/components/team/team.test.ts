import { afterEach, beforeEach, expect, it } from 'vitest';
import { closePage, show } from '../../../features/page';
import { Group, Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import Bulletin from './Bulletin.svelte';
import TeamMembers from './TeamMembers.svelte';
import mounts from './mounts';

const text = () => document.body.textContent!.replace(/\s+/g, ' ').trim();
function setup() {
  const group = new Group(
    new Home({ user: 'viewer', sign: async () => ({ p: '', k: '', s: '' }) }),
    'team',
  );
  return group;
}
beforeEach(() => {
  prefs.lang = 'en';
});
afterEach(closePage);

it('mounts the bulletin card', () => {
  expect(mounts.cards.bulletin).toBe(Bulletin);
});

it('attributes a bulletin to the named AI and project on the signer’s computer', () => {
  const group = setup();
  const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
  group.apply([person]);
  const msg = p.decode(p.MsgRow, goldens['row-msg-bulletin']) as Extract<
    p.MsgRow,
    { kind: 'bulletin' }
  >;
  show(Bulletin, { group, msg });
  expect(text()).toBe(
    `Claude Code Website On a computer belonging to ${person.name} The website is ready.`,
  );
  expect(document.querySelector('a')!.getAttribute('href')).toBe(`#/groups/${msg.body.from}`);
});

it('shows a missing signer explicitly and supports an empty message', () => {
  const group = setup();
  const msg = { ...p.decode(p.MsgRow, goldens['row-msg-bulletin']), text: '' } as Extract<
    p.MsgRow,
    { kind: 'bulletin' }
  >;
  show(Bulletin, { group, msg });
  expect(text()).toBe('Claude Code Website On a computer belonging to Former member');
});

it('shows only people, their live state, and no AI controls', () => {
  const group = setup();
  const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
  group.apply([
    person,
    { ...person, id: 'other', name: 'Reader', online: false },
    p.decode(p.AIMemberRow, goldens['row-member-ai']),
  ]);
  show(TeamMembers, { group });
  expect(document.querySelectorAll('li').length).toBe(2);
  expect([...document.querySelectorAll('.st')].map((el) => el.textContent!.trim())).toEqual([
    'Online',
    'Offline',
  ]);
  expect(document.querySelector('button')).toBeNull();
});

it('shows an empty member list', () => {
  show(TeamMembers, { group: setup() });
  expect(text()).toBe('People · 0');
  expect(document.querySelectorAll('li').length).toBe(0);
});
