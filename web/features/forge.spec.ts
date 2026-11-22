import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import { Group, Home } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import { prefs } from '../src/lib/prefs.svelte';
import * as p from '../src/lib/proto';
import GroupDetails from '../src/components/groupinfo/GroupDetails.svelte';
import { all, closePage, one, show, text } from './page';

const feature = await loadFeature('../features/web_forge.feature');
describeFeature(feature, ({ AfterEachScenario, Scenario }) => {
  AfterEachScenario(closePage);
  function setup(forge: string) {
    prefs.lang = 'en';
    const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
    const home = new Home({ user: person.user, sign: async () => ({ p: '', k: '', s: '' }) });
    home.owner = true;
    home.name = 'Computer';
    home.ctl = { request: vi.fn().mockResolvedValue({ re: 1, ok: true }) as p.Ctl['request'] };
    const group = new Group(home, person.g);
    const ai = p.decode(p.AIMemberRow, goldens['row-member-ai']);
    group.apply([
      person,
      { ...ai, user: 'another-owner', account: 'personal' },
      {
        ...p.decode(p.GroupRow, goldens['row-group-custom']),
        forge,
        secretary: ai.id,
        checks: ['pnpm check', 'pnpm test'],
      },
    ]);
    show(GroupDetails, { group });
  }
  Scenario(
    'Group info discloses custom commands without a consent gate',
    ({ Given, When, Then, And }) => {
      Given(
        'a project with custom forge templates and a personal secretary owned by another person',
        () => setup('custom'),
      );
      When("the reader opens the group's commands", () => {
        one<HTMLDetailsElement>('details').open = true;
      });
      Then(
        'the repository base forge checks and argument lists are disclosed with the execution warning',
        () => {
          expect(all('details dd').map((el) => el.textContent)).toEqual([
            'https://git.example/team/website.git',
            'main',
            'Custom command templates',
          ]);
          expect(all('details section > p code').map((el) => el.textContent)).toEqual([
            'pnpm check',
            'pnpm test',
          ]);
          expect(
            all('details .arguments').map((list) =>
              [...list.querySelectorAll('code')].map((el) => el.textContent),
            ),
          ).toEqual([
            ['git', 'push', 'origin', '{branch}'],
            ['forge', 'open', '--title={title}', '--body={summary}', '{base}', '{commit}'],
            ['forge', 'checks', '{request}', '{commit}'],
            ['forge', 'merge', '{request}', '{commit}'],
          ]);
          expect(text('details .note')).toBe(
            'These commands run on this computer as you, outside any sandbox, whenever an AI of yours works on a task of this group. Join only groups whose commands you would run yourself.',
          );
        },
      );
      And('the secretary warning is visible without blocking group settings', () => {
        expect(text('section.field > .note')).toBe(
          "Every request to this personal-subscription secretary from someone other than its owner waits for the owner's confirmation, each time. With a team plan or an API key, the owner can list who may direct it.",
        );
        expect(one<HTMLButtonElement>('section.field > button').disabled).toBe(false);
      });
    },
  );
  Scenario(
    'A built-in forge remains fixed when editing group settings',
    ({ Given, When, Then }) => {
      Given('a project using the built-in GitHub forge', () => setup('github'));
      When('the owner opens group settings', () => {
        one<HTMLButtonElement>('section.field > button').click();
        flushSync();
      });
      Then('no forge selector or template editor is offered', () => {
        expect(document.querySelector('dialog > form > p.hint')).toBeNull();
        expect(document.querySelector('dialog fieldset')).toBeNull();
        expect(document.querySelectorAll('dialog select').length).toBe(1);
        expect(text('dialog label > span')).toBe('Secretary');
      });
    },
  );
});
