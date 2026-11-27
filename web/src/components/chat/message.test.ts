import { afterEach, beforeEach, expect, it } from 'vitest';
import { all, closePage, show } from '../../../features/page';
import { Group, Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import Message from './Message.svelte';

beforeEach(() => {
  prefs.lang = 'en';
});
afterEach(closePage);

it("draws the picture of a member that left from the name it left with, not '?'", () => {
  const me = { user: 'alex', sign: async (payload: string) => ({ p: payload, k: 'k', s: 's' }) };
  const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
  const decoded = p.decode(p.MsgRow, goldens['row-msg-text']);
  const text = { ...decoded, by: 'mina' };
  const group = new Group(new Home(me), person.g);
  group.apply([
    { ...person, id: 'mina', name: 'Mina', seq: 1 },
    { ...text, seq: 2 },
  ]);
  group.apply([{ r: 'member', id: 'mina', seq: 3, gone: true }]);
  show(Message, { group, msg: text, joined: false });
  expect(all('.av').map((av) => av.textContent?.trim())).toEqual(['M']);
});
