import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { expect } from 'vitest';
import { show, closePage, openPage } from './page';
import { Group, Home, homes } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import * as p from '../src/lib/proto';
import ConfirmCard from '../src/components/confirm/ConfirmCard.svelte';
import { prefs } from '../src/lib/prefs.svelte';

const feature = await loadFeature('../features/web_watch_team.feature');
describeFeature(feature, ({ AfterEachScenario, Scenario }) => {
  let group: Group;
  AfterEachScenario(closePage);
  const setup = () => {
    const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
    const home = new Home({ user: person.user, sign: async () => ({ p: '', k: '', s: '' }) });
    group = new Group(home, person.g);
    group.apply([person]);
    home.groups = [group];
    homes.push(home);
    prefs.lang = 'en';
    return person;
  };
  Scenario(
    'A team group has a main line and people without project controls',
    ({ Given, When, Then }) => {
      Given('a team group with a bulletin and two people', () => {
        const person = setup();
        const row = { ...p.decode(p.GroupRow, goldens['row-group-team']), id: group.g };
        group.apply([
          row,
          { ...person, id: 'reader', name: 'Reader', online: false },
          p.decode(p.MsgRow, goldens['row-msg-bulletin']),
        ]);
        group.name = row.name;
        group.paging().more = false;
      });
      When('the team group is opened', () => {
        openPage(`#/groups/${group.g}`);
      });
      Then('the bulletin names its AI project and signer and no project controls appear', () => {
        expect(
          document
            .querySelector('[aria-label="Bulletin"]')!
            .textContent!.replace(/\s+/g, ' ')
            .trim(),
        ).toBe('Claude Code Website On a computer belonging to owner The website is ready.');
        expect(document.querySelectorAll('.people li').length).toBe(2);
        expect(document.querySelector('.member')).toBeNull();
        expect(document.querySelector('.groupinfo')).not.toBeNull();
        expect(document.querySelector('.members .head')).toBeNull();
        expect(document.querySelector('.nosec')).toBeNull();
        expect(document.querySelector('.chat-head')!.textContent).toContain('Team group');
        expect(document.querySelector('textarea')!.placeholder).toBe('Message the team');
      });
    },
  );
  Scenario('A watch request names its AI and requester', ({ Given, Then }) => {
    Given('a watch confirmation addressed to the owner', () => {
      const person = setup();
      const ai = p.decode(p.AIMemberRow, goldens['row-member-ai']);
      group.apply([ai]);
      const msg = p.decode(p.MsgRow, goldens['row-msg-confirm-pending']) as Extract<
        p.MsgRow,
        { kind: 'confirm' }
      >;
      show(ConfirmCard, {
        group,
        msg: { ...msg, by: person.id, body: { what: 'watch', ai: ai.id, to: [person.user] } },
      });
    });
    Then('the owner sees who wants to watch which AI', () => {
      expect(document.querySelector('.card > p')!.textContent).toBe(
        'Let owner watch Codex’s terminal.',
      );
      expect(
        [...document.querySelectorAll('button')].map((button) => button.textContent!.trim()),
      ).toEqual(['Confirm', 'Deny']);
    });
  });
});
