import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import { Group, Home } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import { prefs } from '../src/lib/prefs.svelte';
import * as p from '../src/lib/proto';
import GroupDetails from '../src/components/groupinfo/GroupDetails.svelte';
import { closePage, one, show, text, texts } from './page';

const feature = await loadFeature('../features/web_trust.feature');
describeFeature(feature, ({ AfterEachScenario, Scenario }) => {
  AfterEachScenario(closePage);
  function setup(secretary: Partial<p.AIMemberRow>) {
    prefs.lang = 'en';
    const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
    const home = new Home({ user: person.user, sign: async () => ({ p: '', k: '', s: '' }) });
    home.owner = true;
    home.name = 'Computer';
    home.ctl = { request: vi.fn().mockResolvedValue({ re: 1, ok: true }) as p.Ctl['request'] };
    const group = new Group(home, person.g);
    const ai = { ...p.decode(p.AIMemberRow, goldens['row-member-ai']), ...secretary };
    group.apply([person, ai, { ...p.decode(p.GroupRow, goldens['row-group']), secretary: ai.id }]);
    show(GroupDetails, { group });
  }
  Scenario(
    'Group info shows the warning of a personal secretary without a consent gate',
    ({ Given, Then }) => {
      Given('a group whose secretary is the personal AI of another person', () =>
        setup({ user: 'another-owner', account: 'personal' }),
      );
      Then('the secretary warning is visible without blocking group settings', () => {
        expect(text('section.field > .note')).toBe(
          "Every request to this personal-subscription secretary from someone other than its owner waits for the owner's confirmation, each time. With a team plan or an API key, the owner can list who may direct it.",
        );
        expect(one<HTMLButtonElement>('section.field > button').disabled).toBe(false);
      });
    },
  );
  Scenario('Group settings name no command that runs on a computer', ({ Given, When, Then }) => {
    Given('a group of the owner', () => setup({}));
    When('the owner opens group settings', () => {
      one<HTMLButtonElement>('section.field > button').click();
      flushSync();
    });
    Then(
      'the settings offer its name, its secretary, the pass score and the foundation tasks, and no command',
      () => {
        expect(texts('dialog summary b')).toEqual(['Limits']);
        expect(texts('dialog legend')).toEqual(['Review scores']);
        expect(document.querySelectorAll('dialog select').length).toBe(1);
        expect(texts('dialog label > span:first-child')).toEqual([
          'Group name',
          'Secretary',
          'Pass score',
          'Foundation tasks',
        ]);
        expect(document.querySelector('dialog textarea')).toBeNull();
      },
    );
  });
});
