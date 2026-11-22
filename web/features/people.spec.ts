import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi, type MockInstance } from 'vitest';
import { homes, type Home } from '../src/lib/groups.svelte';
import { JoinError, readInvitation } from '../src/lib/join';
import { goldens } from '../src/lib/mock';
import type { Net, Source } from '../src/lib/net.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import { href } from '../src/lib/route.svelte';
import { clock } from '../src/lib/when';
import { browser, example, owner, TestHome } from './home';
import { all, closePage, fill, go, one, openPage, text } from './page';

const feature = await loadFeature('../features/web_people.feature');
const join = vi.fn<NonNullable<Net['join']>>();
const invitation = JSON.parse(goldens['cmd-invite-link']) as { link: string; expires: number };
const hash = new URL(invitation.link).hash;
const { g } = readInvitation(hash)!;

vi.mock('../src/lib/net.svelte', async (actual) => ({
  ...(await actual<typeof import('../src/lib/net.svelte')>()),
  start: (source: Source) => ({ net: { source, homes, path: () => undefined, join }, stop() {} }),
}));

const refusal = (code: string) => ({ re: 1, err: { code, msg: 'Refused' } });
const success = { re: 1, ok: true as const };
const buttons = (scope: ParentNode) => [...scope.querySelectorAll('button')];
const button = (label: string, scope: ParentNode = document) =>
  buttons(scope).find((item) => item.textContent?.trim() === label)!;
const person = (name: string) =>
  all('.person').find((item) =>
    item.querySelector('.person-name')?.textContent?.trim().startsWith(name),
  )!;
const actions = (name: string) => buttons(person(name)).map((item) => item.textContent!.trim());
const activeDialog = () => document.querySelector<HTMLDialogElement>('dialog[open]')!;

function submit() {
  fill(one('main input'), '  Reader  ');
  flushSync();
  button('Join group').click();
  flushSync();
}

describeFeature(feature, ({ Scenario, AfterEachScenario }) => {
  let home: TestHome;
  let command: MockInstance<Home['command']>;

  async function openAs(role: string) {
    home = new TestHome();
    home.add(g, 'Website');
    home.write(g, example('row-member-human', { id: owner, g, name: 'Reader', role }));
    if (role !== 'owner') addPerson('Owner', 'owner');
    addPerson('Member', 'member');
    addPerson('Admin', 'admin');
    const client = browser(home);
    homes.push(client.store);
    await client.connect();
    command = vi.spyOn(client.store, 'command');
    openPage(href('chat', { g }));
    await vi.waitFor(() => expect(person('Reader')).toBeDefined());
  }

  function addPerson(name: string, role: string) {
    home.write(g, example('row-member-human', { id: name, g, name, role, user: name }));
  }

  AfterEachScenario(async () => {
    await closePage();
    join.mockReset();
    vi.useRealTimers();
  });

  Scenario('An owner invites someone with an expiring link', ({ Given, When, Then, And }) => {
    Given('I am the group owner', async () => {
      await openAs('owner');
      expect(text('.members > .hint')).toBe('No AI in this group yet.');
    });
    When('I create an invitation', async () => {
      vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] });
      vi.setSystemTime(invitation.expires - 60_000);
      command.mockResolvedValueOnce(refusal('forbidden'));
      command.mockResolvedValueOnce({ ...success, ...invitation });
      button('Invite').click();
      flushSync();
      button('Cancel', activeDialog()).click();
      await vi.waitFor(() => expect(activeDialog()).toBeNull());
      button('Invite').click();
      flushSync();
      button('Create link', activeDialog()).click();
      await vi.waitFor(() =>
        expect(text('dialog [role=alert]')).toBe('Not sent: this device may not do that.'),
      );
      button('Create link', activeDialog()).click();
      await vi.waitFor(() => expect(text('dialog pre')).toBe(invitation.link));
      expect(command.mock.calls).toEqual([[{ t: 'invite', g }], [{ t: 'invite', g }]]);
    });
    Then('I can copy its link before it expires', async () => {
      expect(text('dialog .hint')).toBe(
        `1 min left · expires at ${clock(invitation.expires, 'en')}`,
      );
      const writeText = vi
        .spyOn(navigator.clipboard, 'writeText')
        .mockRejectedValueOnce(new Error('denied'))
        .mockResolvedValueOnce(undefined);
      button('Copy link').click();
      await vi.waitFor(() => expect(text('.app [role=status]')).toBe('Could not copy the link.'));
      button('Copy link').click();
      await vi.waitFor(() => expect(text('.app [role=status]')).toBe('Link copied'));
      expect(writeText.mock.calls).toEqual([[invitation.link], [invitation.link]]);
    });
    And('after expiry the link is not offered for copying', async () => {
      await vi.advanceTimersByTimeAsync(60_000);
      flushSync();
      expect(text('dialog .expired')).toBe('This link has expired.');
      expect(buttons(activeDialog()).map((item) => item.textContent!.trim())).toEqual(['Done']);
      button('Done').click();
      await vi.waitFor(() => expect(activeDialog()).toBeNull());
    });
  });

  Scenario('An owner changes roles and removes a person', ({ Given, When, Then }) => {
    Given('I am the group owner', () => openAs('owner'));
    When('I promote a person and the home updates their role', async () => {
      command.mockResolvedValueOnce(success);
      button('Make admin', person('Member')).click();
      await vi.waitFor(() =>
        expect(command.mock.calls).toEqual([
          [{ t: 'member', g, op: 'set', id: 'Member', role: 'admin' }],
        ]),
      );
      addPerson('Member', 'admin');
    });
    Then('their row shows that they are an admin', async () => {
      await vi.waitFor(() => expect(actions('Member')).toEqual(['Make member', 'Remove']));
      expect(person('Member').querySelector('.person-name')?.textContent?.trim()).toBe(
        'Member · Admin',
      );
      expect(actions('Reader')).toEqual([]);
    });
    When('I remove the person', () => {
      command.mockResolvedValueOnce(refusal('conflict'));
      button('Remove', person('Member')).click();
    });
    Then('a task conflict is shown without removing their row', async () => {
      await vi.waitFor(() =>
        expect(text('.person [role=alert]')).toBe(
          'A person cannot leave or be removed while one of their AIs is assigned to or reviewing an unfinished task.',
        ),
      );
      expect(actions('Member')).toEqual(['Make member', 'Remove']);
    });
    When('the home accepts the removal', async () => {
      command.mockResolvedValueOnce(refusal('forbidden')).mockResolvedValueOnce(success);
      button('Remove', person('Member')).click();
      await vi.waitFor(() => expect(person('Member').querySelector('.fail')).toBeTruthy());
      button('Remove', person('Member')).click();
      expect(command.mock.lastCall).toEqual([{ t: 'member', g, op: 'remove', id: 'Member' }]);
      home.remove(g, 'member', 'Member');
    });
    Then('the person is no longer listed', () =>
      vi.waitFor(() => expect(person('Member')).toBeUndefined()),
    );
  });

  Scenario('An admin can invite and remove members but cannot set roles', ({ Given, Then }) => {
    Given('I am a group admin', () => openAs('admin'));
    Then(
      'I can invite, remove a member, or leave, but cannot remove the owner or set roles',
      () => {
        expect(button('Invite').disabled).toBe(false);
        expect(actions('Reader')).toEqual(['Leave']);
        expect(actions('Member')).toEqual(['Remove']);
        expect(actions('Admin')).toEqual([]);
        expect(actions('Owner')).toEqual([]);
      },
    );
  });

  Scenario('A member cannot invite or remove other people', ({ Given, Then }) => {
    Given('I am a group member', () => openAs('member'));
    Then('I can only leave the group', () => {
      expect(button('Invite')).toBeUndefined();
      expect(actions('Reader')).toEqual(['Leave']);
      expect(actions('Member')).toEqual([]);
      expect(actions('Admin')).toEqual([]);
      expect(actions('Owner')).toEqual([]);
    });
  });

  Scenario('An unpaired browser opens an invitation', ({ Given, When, Then }) => {
    Given('I open a valid invitation link', () => openPage(hash));
    When('I submit my name on an unpaired browser', () => {
      join.mockRejectedValueOnce(new JoinError('onboarding', 'unpaired'));
      submit();
    });
    Then('the page offers to connect a computer', async () => {
      await vi.waitFor(() =>
        expect(text('main [role=alert]')).toBe(
          'Connect a computer before joining. Then return to this invitation.',
        ),
      );
      expect(one<HTMLAnchorElement>('main a[href="#/onboarding"]').textContent).toBe(
        'Connect a computer',
      );
      expect(one<HTMLInputElement>('main input').disabled).toBe(false);
    });
  });

  Scenario('Leaving a pending join cancels it', ({ Given, When, Then }) => {
    let signal: AbortSignal;
    type Admission = Awaited<ReturnType<NonNullable<Net['join']>>>;
    const response = Promise.withResolvers<Admission>();
    Given('I open a valid invitation link', () => openPage(hash));
    When('I submit my name and navigate away while joining', () => {
      join.mockReturnValueOnce(response.promise);
      const field = one<HTMLInputElement>('main input');
      expect(field.disabled).toBe(false);
      expect(field.value).toBe('');
      expect(button('Join group').disabled).toBe(true);
      fill(field, '   ');
      flushSync();
      expect(button('Join group').disabled).toBe(true);
      submit();
      expect(field.disabled).toBe(true);
      expect(button('Joining…').disabled).toBe(true);
      expect(join.mock.calls).toEqual([[readInvitation(hash), 'Reader', expect.any(AbortSignal)]]);
      signal = join.mock.lastCall![2];
      go(href('groups'));
    });
    Then('the invitation attempt is cancelled', async () => {
      expect(signal.aborted).toBe(true);
      response.resolve({ notifications: [] });
      await response.promise;
      expect(location.hash).toBe(href('groups'));
    });
  });

  Scenario(
    'A refusal arriving after navigation does not reopen the join page',
    ({ Given, When, Then }) => {
      const response = Promise.withResolvers<Awaited<ReturnType<NonNullable<Net['join']>>>>();
      Given('I open a valid invitation link', () => openPage(hash));
      When('I submit my name and the home refuses after I navigate away', async () => {
        join.mockReturnValueOnce(response.promise);
        submit();
        go(href('groups'));
        response.reject(new JoinError('room', 'cancelled'));
        await expect(response.promise).rejects.toBeInstanceOf(JoinError);
      });
      Then('the groups page stays open', () => {
        expect(location.hash).toBe(href('groups'));
        expect(document.querySelector('main form.card')).toBeNull();
      });
    },
  );

  Scenario('Admission opens the joined group', ({ Given, When, Then }) => {
    Given('I open a valid invitation link', () => openPage(hash));
    When('I submit my name and the home admits me', () => {
      join.mockResolvedValueOnce({ notifications: [] });
      submit();
      expect(join.mock.calls).toEqual([[readInvitation(hash), 'Reader', expect.any(AbortSignal)]]);
    });
    Then('the client opens the joined group', async () => {
      await vi.waitFor(() => expect(location.hash).toBe(href('chat', { g })));
    });
  });

  Scenario('An invalid invitation never starts a join', ({ Given, Then }) => {
    Given('I open a malformed invitation', () => {
      openPage('#/join');
      expect(text('main [role=alert]')).toBe('This invitation link is invalid.');
      go('#join=invalid');
    });
    Then('the page rejects the link without showing the form', () => {
      expect(text('main [role=alert]')).toBe('This invitation link is invalid.');
      expect(document.querySelector('main form')).toBeNull();
      expect(join.mock.calls).toEqual([]);
    });
  });

  Scenario(
    'An admitted browser can open its group after a notification failure',
    ({ Given, When, Then }) => {
      Given('I open a valid invitation link', () => openPage(hash));
      When(
        'the home first refuses and then admits me but another computer cannot be notified',
        async () => {
          join.mockRejectedValueOnce(new JoinError('join', 'expired'));
          submit();
          await vi.waitFor(() =>
            expect(text('main [role=alert]')).toBe('Could not join (join: expired).'),
          );
          join.mockResolvedValueOnce({
            notifications: [{ node: 'own', error: new JoinError('home.add', 'lost') }],
          });
          submit();
        },
      );
      Then('the notification warning links to the group', async () => {
        await vi.waitFor(() =>
          expect(text('main [role=status]')).toBe(
            'You joined the group, but not all of your connected computers were notified.',
          ),
        );
        expect(one<HTMLAnchorElement>('main .card a').getAttribute('href')).toBe(
          href('chat', { g }),
        );
      });
    },
  );

  Scenario('The join page uses the shell language and theme controls', ({ Given, When, Then }) => {
    let lightIcon: Element;
    Given('I open a valid invitation link', () => openPage(hash));
    When('I change theme and language from the join page header', () => {
      prefs.theme = 'light';
      flushSync();
      expect(text('main .badge')).toBe('Group invitation');
      expect(text('main h1')).toBe('Join a group');
      expect(text('main .hero p')).toBe('Choose the name people in this group will see.');
      expect(text('main label span')).toBe('Your name in this group');
      expect(text('main .card a')).toBe('Cancel');
      expect(one('main .top .only-wide')).toBeNull();
      expect(text('main .top .swap[lang="zh-CN"]')).toBe('中');
      expect(text('main .top .sr [lang=en]')).toBe('Switch language');
      expect(text('main .top .sr [lang=zh-CN]')).toBe('切换语言');
      lightIcon = one('main .top .swap svg');
      expect(lightIcon.querySelector('path')).not.toBeNull();
      one('main button[aria-label="Toggle theme"]').click();
      flushSync();
      one('main .top button:first-of-type').click();
      flushSync();
    });
    Then('the join page uses dark mode and Chinese text', () => {
      expect(prefs.theme).toBe('dark');
      expect(prefs.lang).toBe('zh-CN');
      expect(text('main h1')).toBe('加入群聊');
      expect(text('main .top .swap[lang=en]')).toBe('EN');
      expect(one('main .top .swap svg')).not.toBe(lightIcon);
      expect(one('main .top .swap svg').querySelector('circle')).not.toBeNull();
    });
  });

  Scenario('The panel still hands back an owned AI', ({ Given, When, Then }) => {
    Given('I am the group owner with an AI in takeover', async () => {
      await openAs('owner');
      home.write(
        g,
        example('row-member-ai', {
          g,
          id: 'ai-driver',
          name: 'Driver',
          cli: 'custom',
          host: homes[0].device,
          state: 'takeover',
        }),
      );
      await vi.waitFor(() => {
        expect(button('Hand back')).toBeDefined();
        expect(text('.av-wrap .av')).toBe('D');
        expect(text('.card-foot .hint')).toBe('Messages to it are queued until you hand back');
      });
    });
    When('I hand back from its member row', () => {
      button('Hand back').click();
    });
    Then('its computer receives the hand-back command', async () => {
      const sent = () =>
        home.asked
          .filter(({ t }) => t === 'cmd')
          .map(({ env }) => JSON.parse((env as { p: string }).p));
      await vi.waitFor(() =>
        expect(sent()).toEqual([
          {
            t: 'takeover.end',
            to: homes[0].device,
            g,
            member: 'ai-driver',
            n: expect.any(String),
            ts: expect.any(Number),
          },
        ]),
      );
    });
  });
});
