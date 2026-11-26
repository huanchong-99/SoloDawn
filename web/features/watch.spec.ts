import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { expect } from 'vitest';
import { show, closePage, openPage, texts } from './page';
import { Group, Home, homes } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import * as p from '../src/lib/proto';
import ConfirmCard from '../src/components/confirm/ConfirmCard.svelte';
import { prefs } from '../src/lib/prefs.svelte';

const feature = await loadFeature('../features/web_watch.feature');
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
    'A group with people and no AI has a main line, its people and a way to add an AI',
    ({ Given, When, Then }) => {
      Given('a group with two people and no AI', () => {
        const person = setup();
        const row = { ...p.decode(p.GroupRow, goldens['row-group']), id: group.g };
        delete row.secretary;
        group.apply([row, { ...person, id: 'reader', name: 'Reader', online: false }]);
        group.name = row.name;
        group.paging().more = false;
      });
      When('the group is opened', () => {
        openPage(`#/groups/${group.g}`);
      });
      Then(
        'the main line asks for a message to the people, the member list shows both people and offers to add an AI, and no secretary warning appears',
        () => {
          expect(document.querySelector('textarea')!.placeholder).toBe(
            'Message the people of this group',
          );
          expect(document.querySelectorAll('.people li').length).toBe(2);
          expect(texts('.members .head button')).toEqual(['Add AI']);
          expect(document.querySelector('.member')).toBeNull();
          expect(document.querySelector('.nosec')).toBeNull();
          expect(document.querySelector('.chat-head')!.textContent).not.toContain('Team');
        },
      );
    },
  );
  Scenario("The terminal of another person's AI is titled as watched", ({ Given, When, Then }) => {
    let ai: p.AIMemberRow;
    Given('the AI "Codex" of another person in the group', () => {
      setup();
      ai = { ...p.decode(p.AIMemberRow, goldens['row-member-ai']), user: 'someone-else' };
      group.apply([ai]);
    });
    When('its terminal is opened', () => {
      openPage(`#/terminal/${group.g}/${ai.id}`);
    });
    Then('the page is titled "Watching a terminal"', () => {
      expect(texts('#page-title')).toEqual(['Watching a terminal']);
      expect(document.title).toBe('Watching a terminal · SoloDawn');
    });
  });
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
