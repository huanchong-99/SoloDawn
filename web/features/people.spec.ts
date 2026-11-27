import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi, type MockInstance } from 'vitest';
import { homes, type Home } from '../src/lib/groups.svelte';
import { JoinError, readCode, readInvitation } from '../src/lib/join';
import { goldens } from '../src/lib/mock';
import type { Net, Source } from '../src/lib/net.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import { href } from '../src/lib/route.svelte';
import { codeUntil, invitationText } from '../src/lib/members';
import { clock } from '../src/lib/when';
import { browser, example, owner, TestHome } from './home';
import { all, closePage, fill, go, one, openPage, read, text, texts } from './page';

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
  let client: ReturnType<typeof browser>;
  let command: MockInstance<Home['command']>;

  /** Opens the chat of "Website" as a person of `role`; `also` adds to the home first. */
  async function openAs(role: string, also = () => {}) {
    home = new TestHome();
    home.add(g, 'Website');
    also();
    home.write(g, example('row-member-human', { id: owner, g, name: 'Reader', role }));
    if (role !== 'owner') addPerson('Owner', 'owner');
    addPerson('Member', 'member');
    addPerson('Admin', 'admin');
    client = browser(home);
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
      // Each opening reads the group's code first: none is open.
      command.mockResolvedValueOnce(success);
      command.mockResolvedValueOnce(success);
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
      const show = [{ t: 'code', g, op: 'show' }];
      expect(command.mock.calls).toEqual([show, show, [{ t: 'invite', g }], [{ t: 'invite', g }]]);
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
      expect(buttons(activeDialog()).map((item) => item.textContent!.trim())).toEqual([
        'Create code',
        'Done',
      ]);
      button('Done').click();
      await vi.waitFor(() => expect(activeDialog()).toBeNull());
    });
  });

  Scenario(
    'An owner adds a person of another group on this computer without a link',
    ({ Given, When, Then, And }) => {
      const docs = 'ZG9jcy1ncm91cC0wMDAwMQ';
      const row = (name: string) =>
        all('dialog[open] .pick-row').find(
          (each) => each.querySelector('b')!.textContent === name,
        )!;
      Given(
        'I am the group owner, and "Mina" is a member of another group on its home, and "Sam" was in one that is gone',
        async () => {
          await openAs('owner', () => {
            home.add(docs, 'Docs');
            for (const name of ['Mina', 'Sam'])
              home.write(
                docs,
                example('row-member-human', { id: name, g: docs, user: name, name }),
              );
          });
          // The browser read Sam in "Docs"; the home has since forgotten him.
          home.groups.get(docs)!.rows.delete('member Sam');
        },
      );
      When('I open "Invite"', () => {
        button('Invite').click();
        flushSync();
      });
      Then(
        'the dialog lists "Mina" and "Sam" under "In your other groups here", each with "Add", and offers "Cancel"',
        () => {
          expect(texts('dialog[open] h3')).toEqual([
            'Invitation code',
            'In your other groups here · 2',
            'One-time link',
          ]);
          expect(all('dialog[open] .pick-row b').map((each) => each.textContent)).toEqual([
            'Mina',
            'Sam',
          ]);
          expect(buttons(row('Mina')).map((each) => each.textContent!.trim())).toEqual(['Add']);
          expect(
            buttons(activeDialog().querySelector('footer')!).map((b) => b.textContent!.trim()),
          ).toEqual(['Cancel']);
        },
      );
      When('I add Mina, and then Sam, whom the home no longer knows', async () => {
        button('Add', row('Mina')).click();
        await vi.waitFor(() => expect(row('Mina').querySelector('.badge')).not.toBeNull());
        button('Add', row('Sam')).click();
        await vi.waitFor(() => expect(row('Sam').querySelector('small')).not.toBeNull());
        flushSync();
      });
      Then(
        'the home has added Mina with "add_person", her row says "Added", and Sam\'s row says to send the code or the link',
        () => {
          const sent = home.asked
            .filter((req) => req.t === 'cmd')
            .map((req) => JSON.parse((req.env as { p: string }).p) as { t: string })
            .filter((fields) => fields.t === 'member');
          expect(sent).toMatchObject([
            { t: 'member', g, op: 'add_person', user: 'Mina', name: 'Mina' },
            { t: 'member', g, op: 'add_person', user: 'Sam', name: 'Sam' },
          ]);
          expect(read(row('Mina').querySelector('.badge')!)).toBe('Added');
          expect(row('Mina').querySelector('.badge')!.getAttribute('data-tone')).toBe('ok');
          expect(buttons(row('Mina'))).toEqual([]);
          expect(read(row('Sam').querySelector('small')!)).toBe(
            'Not in any of your groups here any more: send them the code or the link below.',
          );
        },
      );
      And(
        'Mina is listed among the people of the group, and the dialog offers "Done" in place of "Cancel"',
        async () => {
          await vi.waitFor(() => expect(person('Mina')).toBeDefined());
          expect(
            buttons(activeDialog().querySelector('footer')!).map((b) => b.textContent!.trim()),
          ).toEqual(['Done']);
        },
      );
    },
  );

  const opened = JSON.parse(goldens['cmd-code']) as { code: string; expires: number };
  const open = { code: opened.code, expires: opened.expires };
  const renewed = { code: 'ABCD-EFGH-JKMN-PQRS', expires: opened.expires + 3_600_000 };
  const codeShown = () => document.querySelector('dialog .code-card output')?.textContent;
  const hourChoices = () =>
    all<HTMLInputElement>('dialog .seg input').map((input) => [
      input.closest('label')!.textContent!.trim(),
      input.checked,
    ]);

  Scenario(
    "An owner makes, replaces and turns off the group's invitation code",
    ({ Given, When, Then, And }) => {
      Given('I am the group owner', () => openAs('owner'));
      When('I open "Invite" while the group has no invitation code open', async () => {
        command.mockResolvedValueOnce(success);
        button('Invite').click();
        flushSync();
        await vi.waitFor(() => expect(command).toHaveBeenCalledOnce());
        expect(command.mock.calls).toEqual([[{ t: 'code', g, op: 'show' }]]);
      });
      Then(
        'the dialog has asked the home for the code, and offers to make one for 1 hour, 24 hours or 7 days, with 24 hours chosen',
        () => {
          expect(text('dialog section p.lead')).toBe(
            'Anyone who has the code can join while it is valid. Give it only to the people you invite, and turn it off when they are in.',
          );
          expect(hourChoices()).toEqual([
            ['1 hour', false],
            ['24 hours', true],
            ['7 days', false],
          ]);
          expect(codeShown()).toBeUndefined();
          expect(button('Create code', activeDialog()).disabled).toBe(false);
        },
      );
      When('I make a code for 7 days, which the home first refuses', async () => {
        all<HTMLInputElement>('dialog .seg input')[2].click();
        flushSync();
        command.mockResolvedValueOnce(refusal('forbidden'));
        command.mockResolvedValueOnce({ ...success, ...open });
        button('Create code', activeDialog()).click();
        await vi.waitFor(() =>
          expect(text('dialog [role=alert]')).toBe('Not sent: this device may not do that.'),
        );
        button('Create code', activeDialog()).click();
        await vi.waitFor(() => expect(codeShown()).toBe(open.code));
        const made = [{ t: 'code', g, op: 'new', hours: 168 }];
        expect(command.mock.calls).toEqual([made, made]);
      });
      Then('the dialog shows the code and until when it is valid', () => {
        expect(text('dialog .code-card .until')).toBe(codeUntil(open, 'en'));
        expect(document.querySelector('dialog [role=alert]')).toBeNull();
        expect(all('dialog .seg')).toEqual([]);
      });
      And('I can copy the code, and the invitation, whose last line is the code', async () => {
        const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
        button('Copy code').click();
        await vi.waitFor(() => expect(text('.app [role=status]')).toBe('Copied'));
        button('Copy invitation').click();
        await vi.waitFor(() => expect(writeText).toHaveBeenCalledTimes(2));
        const copied = invitationText('Website', open, 'en');
        expect(writeText.mock.calls).toEqual([[open.code], [copied]]);
        expect(copied.split('\n')).toEqual([expect.stringContaining('"Website"'), open.code]);
        writeText.mockRejectedValueOnce(new Error('denied'));
        button('Copy code').click();
        await vi.waitFor(() => expect(text('.app [role=status]')).toBe('Could not copy.'));
      });
      When(
        'I choose "New code", am told that the current code stops working at once, and confirm',
        async () => {
          button('New code', activeDialog()).click();
          flushSync();
          expect(text('dialog .note[role=status]')).toBe('The current code stops working at once.');
          expect(codeShown()).toBeUndefined();
          button('Cancel', activeDialog()).click();
          flushSync();
          expect(codeShown()).toBe(open.code);
          button('New code', activeDialog()).click();
          flushSync();
          expect(hourChoices().find(([, on]) => on)).toEqual(['7 days', true]);
          command.mockResolvedValueOnce({ ...success, ...renewed });
          button('New code', activeDialog()).click();
          await vi.waitFor(() => expect(command).toHaveBeenCalledOnce());
          expect(command.mock.calls).toEqual([[{ t: 'code', g, op: 'new', hours: 168 }]]);
        },
      );
      Then('the dialog shows the new code', async () => {
        await vi.waitFor(() => expect(codeShown()).toBe(renewed.code));
        expect(document.querySelector('dialog .note')).toBeNull();
      });
      When('I turn the code off', async () => {
        command.mockResolvedValueOnce(success);
        button('Turn off', activeDialog()).click();
        await vi.waitFor(() => expect(codeShown()).toBeUndefined());
        expect(command.mock.calls).toEqual([[{ t: 'code', g, op: 'off' }]]);
      });
      Then('the dialog shows no code and offers to make one', () => {
        expect(button('Create code', activeDialog())).toBeDefined();
        expect(hourChoices().map(([label]) => label)).toEqual(['1 hour', '24 hours', '7 days']);
      });
    },
  );

  Scenario(
    'The dialog shows the open code, and reads it again when the home is reached again',
    ({ Given, When, Then }) => {
      Given('I am the group owner', () => openAs('owner'));
      When('I open "Invite" while the group\'s code is open', () => {
        command.mockResolvedValueOnce({ ...success, ...open });
        button('Invite').click();
        flushSync();
      });
      Then('the dialog shows the code, and offers "Done"', async () => {
        await vi.waitFor(() => expect(codeShown()).toBe(open.code));
        expect(
          buttons(activeDialog().querySelector('footer')!).map((b) => b.textContent!.trim()),
        ).toEqual(['Done']);
      });
      const icons = () => all('dialog .code-card button svg').map((svg) => svg.innerHTML);
      When('I copy the code, it is replaced elsewhere, and the home is reached again', async () => {
        vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
        button('Copy code').click();
        await vi.waitFor(() => expect(icons()[0]).not.toBe(icons()[1]));
        command.mockResolvedValueOnce({ ...success, ...renewed });
        client.drop();
        await client.connect();
        await vi.waitFor(() => expect(codeShown()).toBe(renewed.code));
      });
      Then('the dialog shows the new code, with neither copy ticked', () => {
        flushSync();
        expect(icons()[0]).toBe(icons()[1]);
      });
      When('the code is turned off elsewhere and the home is reached again', async () => {
        command.mockResolvedValueOnce(success);
        client.drop();
        await client.connect();
        await vi.waitFor(() => expect(command).toHaveBeenCalledOnce());
        expect(command.mock.calls).toEqual([[{ t: 'code', g, op: 'show' }]]);
      });
      Then('the dialog shows no code', () => vi.waitFor(() => expect(codeShown()).toBeUndefined()));
    },
  );

  Scenario(
    'Removing a person says that an open code ends with it, and leaving does not',
    ({ Given, When, Then }) => {
      const notice = () => document.querySelector('.person [role=status]')?.textContent;
      Given('I am a group admin', () => openAs('admin'));
      When('I press "Remove" for a member', () => {
        expect(notice()).toBeUndefined();
        button('Remove', person('Member')).click();
        flushSync();
      });
      Then(
        'the confirmation says that an open invitation code of the group is ended as well',
        () => {
          expect(actions('Member')).toEqual(['Confirm removal']);
          expect(notice()).toBe('If the group has an invitation code open, it is ended as well.');
        },
      );
      When('I press "Leave" for myself', () => {
        button('Confirm removal', person('Member')).dispatchEvent(
          new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
        );
        flushSync();
        expect(notice()).toBeUndefined();
        button('Leave', person('Reader')).click();
        flushSync();
      });
      Then('the confirmation says nothing of a code', () => {
        expect(actions('Reader')).toEqual(['Confirm leaving']);
        expect(notice()).toBeUndefined();
        expect(command).not.toHaveBeenCalledWith(expect.objectContaining({ op: 'remove' }));
      });
    },
  );

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
      flushSync();
      expect(command).not.toHaveBeenCalledWith(expect.objectContaining({ op: 'remove' }));
      expect(actions('Member')).toEqual(['Make member', 'Confirm removal']);
      button('Confirm removal', person('Member')).click();
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
      flushSync();
      button('Confirm removal', person('Member')).click();
      await vi.waitFor(() => expect(person('Member').querySelector('.fail')).toBeTruthy());
      button('Remove', person('Member')).click();
      flushSync();
      button('Confirm removal', person('Member')).click();
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
      response.resolve({ g, notifications: [] });
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
      join.mockResolvedValueOnce({ g, notifications: [] });
      submit();
      expect(join.mock.calls).toEqual([[readInvitation(hash), 'Reader', expect.any(AbortSignal)]]);
    });
    Then('the client opens the joined group', async () => {
      await vi.waitFor(() => expect(location.hash).toBe(href('chat', { g })));
    });
  });

  Scenario('An invalid invitation never starts a join', ({ Given, Then }) => {
    Given('I open a malformed invitation', () => openPage('#join=invalid'));
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
            g,
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

  /** Types a code and a name on the join page, and presses "Join group". */
  function joinWith(code: string) {
    fill(codeField(), code);
    fill(one('main input:not(.code)'), 'Reader');
    flushSync();
    button('Join group').click();
    flushSync();
  }
  const codeField = () => one<HTMLInputElement>('main input.code');
  const openJoin = () => {
    openPage(href('join'));
    expect(text('main .hero p')).toBe(
      'Enter the code, then choose the name people in the group will see.',
    );
    expect(texts('main label span')).toEqual(['Invitation code', 'Your name in this group']);
    expect(codeField().placeholder).toBe('XXXX-XXXX-XXXX-XXXX');
    expect(button('Join group').disabled).toBe(true);
  };
  const typed = '7k2m 9qxd 4f8t wb3n';

  Scenario(
    'A person joins by an invitation code pasted in lower case with spaces',
    ({ Given, When, Then }) => {
      Given('I open "Join a group" with no link', openJoin);
      When(
        'I paste the code in lower case with spaces and no hyphens, give my name, and the home admits me',
        () => {
          join.mockResolvedValueOnce({ g, notifications: [] });
          joinWith(` ${typed} `);
          // Each step is a test of its own, which starts with no calls.
          expect(join.mock.calls).toEqual([
            [{ secret: '7K2M9QXD4F8TWB3N' }, 'Reader', expect.any(AbortSignal)],
          ]);
          expect(join.mock.calls[0][0]).toEqual(readCode(typed));
        },
      );
      Then(
        'the client is asked to join with that code alone, and opens the group the home named',
        () => vi.waitFor(() => expect(location.hash).toBe(href('chat', { g }))),
      );
    },
  );

  Scenario('A text that is no code is refused on the page', ({ Given, When, Then }) => {
    Given('I open "Join a group" with no link', openJoin);
    When('I type a code with the letter O and my name, and join', () => {
      joinWith('7K2M-9QXD-4F8T-WB3O');
      expect(join).not.toHaveBeenCalled();
    });
    Then(
      'the page says that a code has 16 characters and never the letters I, L, O or U, and nothing is sent',
      () => {
        expect(text('main [role=alert]')).toBe(
          'This is not an invitation code: a code has 16 characters and never the letters I, L, O or U.',
        );
        expect(codeField().getAttribute('aria-invalid')).toBe('true');
        fill(codeField(), '7K2M-9QXD-4F8T-WB3N');
        flushSync();
        expect(document.querySelector('main [role=alert]')).toBeNull();
      },
    );
  });

  Scenario('While nothing answers a code, the page says so', ({ Given, When, Then }) => {
    const attempt = Promise.withResolvers<Awaited<ReturnType<NonNullable<Net['join']>>>>();
    Given('I open "Join a group" with no link', openJoin);
    When('I type a code and my name, and join', () => {
      join.mockReturnValueOnce(attempt.promise);
      joinWith(typed);
    });
    Then(
      "the page says that it looks for the group's home, and what to check while nothing answers",
      () => {
        expect(text('main .seek[role=status]')).toBe(
          "Looking for the group's home… If nothing answers: check the code; if it is right, the group's home may be off, or the code may have ended. This waits up to 10 minutes.",
        );
        expect(codeField().disabled).toBe(true);
        expect(button('Joining…').disabled).toBe(true);
      },
    );
    When('nothing has answered after 10 minutes', async () => {
      attempt.reject(new JoinError('room', 'timeout'));
      await vi.waitFor(() => expect(document.querySelector('main .seek')).toBeNull());
    });
    Then("the page says that the group's home never answered", () => {
      expect(text('main [role=alert]')).toBe(
        "The group's home never answered. Check the code, or ask an admin whether the group's home is on and the code is still valid.",
      );
      expect(codeField().disabled).toBe(false);
    });
    When('I join again and the home refuses the code', async () => {
      join.mockRejectedValueOnce(new JoinError('join', 'forbidden'));
      button('Join group').click();
      await vi.waitFor(() => expect(join).toHaveBeenCalledOnce());
    });
    Then('the page says that the code has ended', async () => {
      await vi.waitFor(() =>
        expect(text('main [role=alert]')).toBe(
          'This code has ended: it expired, was turned off or was replaced. Ask an admin for a new one.',
        ),
      );
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
