import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync, tick } from 'svelte';
import { expect, vi } from 'vitest';
import Message from '../src/components/chat/Message.svelte';
import MemberPanel from '../src/components/member/MemberPanel.svelte';
import { Group, Home } from '../src/lib/groups.svelte';
import type { Net } from '../src/lib/net.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import type { AIMemberRow, Ctl, Envelope, MsgRow } from '../src/lib/proto';
import { clock } from '../src/lib/when';
import { example, owner, user } from './home';
import { all, closePage, one, read, show, text, texts } from './page';

const feature = await loadFeature('../features/web_confirm.feature');
const g = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const aiId = '1PC8WineBrUQ-apCjx7tug';
const requester = 'p-user';
const requesterMember = 'p-member';
const failed = { err: { code: 'forbidden' } };

type Confirm = Extract<MsgRow, { kind: 'confirm' }>;

function setup(reader: string, account = 'team') {
  const home = new Home({
    user: reader,
    sign: async (payload) => ({ p: payload, k: reader, s: 'signature' }),
  });
  const group = new Group(home, g);
  const ai = example('row-member-ai', {
    id: aiId,
    user,
    name: 'Codex',
    account,
    allow: [requester],
    approvers: [user],
  }) as AIMemberRow;
  group.apply([
    example('row-group'),
    example('row-member-human', { name: 'Owner' }),
    example('row-member-human', { id: requesterMember, name: 'Requester', user: requester }),
    example('row-msg-text', { id: 'request-pending', by: requesterMember }),
    ai,
  ]);
  const sent: Envelope[] = [];
  function connect(reply: (env: Envelope) => object = () => ({ ok: true })) {
    home.ctl = {
      request: async ({ env }: { env: Envelope }) => {
        sent.push(env);
        return reply(env);
      },
    } as unknown as Ctl;
  }
  return { group, connect, sent, ai };
}

function confirmation(status: Confirm['state']['status'], what: string): Confirm {
  const row = example(`row-msg-confirm-${status}`) as Confirm;
  return {
    ...row,
    by: what === 'watch' ? requesterMember : 'system',
    re: what === 'watch' ? null : `request-${status}`,
    body: { ai: aiId, what, to: [user] },
  };
}

function showCard(group: Group, msg: Confirm) {
  show(Message, { group, msg, joined: false });
}

function showPanel(group: Group) {
  show(MemberPanel, { group, net: { homes: [], path: () => undefined } as unknown as Net });
}

function command(env: Envelope) {
  const { n, ts, ...fields } = JSON.parse(env.p);
  expect(n).toMatch(/^[\w-]{22}$/);
  expect(ts).toBeTypeOf('number');
  expect(env.k).toBe(user);
  expect(env.s).toBe('signature');
  return fields;
}

const save = () => one<HTMLButtonElement>('form button[type="submit"]');
const accessValues = () =>
  all('.member > .kv dt')
    .filter((dt) => ['Allow list', 'Approval delegates'].includes(read(dt)))
    .map((dt) => read(dt.nextElementSibling!));

async function edit(card = 0) {
  const buttons = all<HTMLButtonElement>('.member button').filter(
    (button) => button.textContent === 'Edit access',
  );
  buttons[card].click();
  await tick();
}

function change(field: number) {
  all<HTMLInputElement>('form input[type="checkbox"]')[field].click();
  flushSync();
}

describeFeature(feature, ({ BeforeEachScenario, AfterEachScenario, Scenario }) => {
  BeforeEachScenario(() => {
    prefs.lang = 'en';
  });
  AfterEachScenario(closePage);

  Scenario('Only the person asked can confirm or deny (A9.8, A9.9)', ({ Given, Then, When }) => {
    const msg = confirmation('pending', 'direct');
    const readers = [setup(user), setup(user)];
    Given("a direct request on another person's AI", () => {
      showCard(setup(requester, 'personal').group, msg);
    });
    Then('the requester sees whom it waits for and no actions', () => {
      expect(text('.card p')).toBe('Let Codex act on the request from Requester.');
      expect(text('.card .badge')).toBe('Waiting for Owner');
      expect(all('.card button')).toHaveLength(0);
    });
    When('the person asked confirms on either browser', () => {
      for (const { group, connect } of readers) {
        connect();
        showCard(group, msg);
        const [confirm] = all<HTMLButtonElement>('.need button').slice(-2);
        confirm.click();
        flushSync();
        expect(confirm.disabled).toBe(true);
      }
    });
    Then('each browser sends the signed confirm action', async () => {
      for (const { sent } of readers) {
        await vi.waitFor(() => expect(sent).toHaveLength(1));
        expect(command(sent[0])).toEqual({ t: 'act', g, msg: msg.id, action: 'confirm' });
      }
      expect(text('.need .badge')).toBe('Needs you');
    });
    When('the person asked denies on one browser', () => {
      all<HTMLButtonElement>('.need button').at(-1)!.click();
    });
    Then('that browser sends the signed deny action', async () => {
      const { sent } = readers[1];
      await vi.waitFor(() => expect(sent).toHaveLength(2));
      expect(command(sent[1])).toEqual({ t: 'act', g, msg: msg.id, action: 'deny' });
    });
  });

  Scenario('Requests show their kind and outcome (A9.9, A9.11)', ({ Given, Then, When }) => {
    const { group, ai } = setup(requester);
    const at = clock(1763600000000, 'en');
    const states = [
      confirmation('pending', 'watch'),
      confirmation('confirmed', 'direct'),
      confirmation('denied', 'ask'),
      confirmation('expired', 'direct'),
    ];
    Given('the examples of all four confirmation states', () => {
      group.apply([
        example('row-member-human', { id: owner, name: 'Owner' }),
        example('row-msg-text', { id: 'request-confirmed', by: requesterMember }),
        { ...ai, id: 'caller', name: 'Caller' },
        example('row-msg-call-waiting', { id: 'request-denied', by: 'caller' }),
        { r: 'msg', id: 'request-expired', seq: 1000, gone: true },
      ]);
      for (const msg of states) showCard(group, msg);
    });
    Then('the cards show the three kinds, four states and no actions for the observer', () => {
      expect(texts('.card p')).toEqual([
        'Let Requester watch Codex’s terminal.',
        'Let Codex act on the request from Requester.',
        'Let Codex answer the call from Caller.',
        'Let Codex act on the request from an unknown requester.',
      ]);
      expect(texts('.card .badge')).toEqual([
        'Waiting for Owner',
        `Confirmed by Owner · ${at}`,
        `Denied by Owner · ${at}`,
        `Expired · ${at}`,
      ]);
      expect(all('.card button')).toHaveLength(0);
    });
    When('the person asked is no longer in the group', () => {
      flushSync(() => group.apply([{ r: 'member', id: owner, seq: 1000, gone: true }]));
    });
    Then("the decisions still show their outcomes, times and the person's name", () => {
      expect(texts('.card .badge')).toEqual([
        'Waiting for Owner',
        `Confirmed by Owner · ${at}`,
        `Denied by Owner · ${at}`,
        `Expired · ${at}`,
      ]);
    });
    When('the referenced call has not been loaded', () => {
      showCard(setup(requester).group, states[2]);
    });
    Then('the card identifies the requester as unknown', () => {
      expect(texts('.card p').at(-1)).toBe('Let Codex answer the call from an unknown requester.');
    });
  });

  Scenario("Only each AI's owner can change its lists (A9.14)", ({ Given, Then, When }) => {
    const ownerView = setup(user);
    const observer = setup(requester);
    function addPersonal({ group, ai }: ReturnType<typeof setup>) {
      group.apply([{ ...ai, id: 'personal', name: 'Personal AI', account: 'personal' }]);
    }
    Given('a team AI and a personal AI with the same owner', () => {
      addPersonal(observer);
      addPersonal(ownerView);
      showPanel(observer.group);
    });
    Then('a different member sees the access lists without edit controls', () => {
      expect(all('.member > dl')).toHaveLength(2);
      expect(texts('.member > dl dt')).toEqual([
        'Computer',
        'Account',
        'Model',
        'Level',
        'Tier',
        'Skills',
        'Allow list',
        'Approval delegates',
        'Computer',
        'Account',
        'Model',
        'Level',
        'Tier',
        'Skills',
        'Allow list',
        'Approval delegates',
      ]);
      expect(accessValues()).toEqual(['Requester', 'None', 'Requester', 'None']);
      // The group's admin, who may remove the AI and edit nothing of it.
      expect(texts('.member button')).toEqual(['Remove AI', 'Remove AI']);
    });
    When('the owner changes both lists on the team AI', async () => {
      const { group, connect } = ownerView;
      connect((env) => {
        const { id, allow, approvers } = JSON.parse(env.p);
        const member = group.member(id) as AIMemberRow;
        group.apply([
          { ...member, seq: member.seq + 1, allow, approvers: approvers ?? member.approvers },
        ]);
        return { ok: true };
      });
      showPanel(group);
      expect(texts('.member footer button').filter((label) => label === 'Edit access')).toEqual([
        'Edit access',
        'Edit access',
      ]);
      await edit();
      expect(save().disabled).toBe(true);
      change(0);
      change(1);
      save().click();
    });
    Then('the browser sends both lists to the home and shows the update', async () => {
      const { sent } = ownerView;
      await vi.waitFor(() => expect(document.querySelector('form')).toBeNull());
      expect(command(sent[0])).toEqual({
        t: 'member',
        g,
        op: 'set',
        id: aiId,
        allow: [],
        approvers: [user, requester],
      });
      expect(accessValues().slice(-4)).toEqual(['None', 'Requester', 'Requester', 'None']);
      expect(document.activeElement?.textContent).toBe('Edit access');
    });
    When('the owner clears the allow list on the personal AI', async () => {
      await edit(1);
      expect(text('form .hint')).toBe(
        'The allow list is saved but has no effect on a personal account.',
      );
      const personal = ownerView.group.member('personal') as AIMemberRow;
      ownerView.group.apply([{ ...personal, seq: personal.seq + 1, approvers: [user, requester] }]);
      change(0);
      save().click();
    });
    Then('the browser sends only the empty allow list', async () => {
      const { sent } = ownerView;
      await vi.waitFor(() => expect(sent).toHaveLength(2));
      expect(command(sent[1])).toEqual({ t: 'member', g, op: 'set', id: 'personal', allow: [] });
    });
  });

  Scenario('Offline and refused edits do not appear saved', ({ Given, Then, When }) => {
    const { group, connect } = setup(user);
    Given('a pending confirmation and one owned AI', async () => {
      showCard(group, confirmation('pending', 'watch'));
      showPanel(group);
      await edit();
    });
    Then('offline actions and access edits cannot be submitted', () => {
      expect(text('.card .hint')).toBe('The group home is offline.');
      expect(all<HTMLButtonElement>('.need button').map((button) => button.disabled)).toEqual([
        true,
        true,
      ]);
      expect(all<HTMLFieldSetElement>('form fieldset').map((field) => field.disabled)).toEqual([
        true,
        true,
      ]);
      expect(save().disabled).toBe(true);
    });
    When('the home refuses the confirmation and access changes', async () => {
      connect(() => failed);
      flushSync();
      one<HTMLButtonElement>('.need button').click();
      change(0);
      save().click();
      await vi.waitFor(() => expect(all('[role="alert"]')).toHaveLength(2));
    });
    Then('both errors are shown and the access changes can be cancelled', async () => {
      expect(texts('[role="alert"]')).toEqual([
        'Not sent: this device may not do that.',
        'Not sent: this device may not do that.',
      ]);
      expect(save().disabled).toBe(false);
      one<HTMLButtonElement>('form button[type="button"]').click();
      await tick();
      expect(accessValues()).toEqual(['Requester', 'None']);
    });
  });

  Scenario('An AI in a one-person group has no other person to list', ({ Given, Then }) => {
    const { group, connect, ai } = setup(user);
    Given('the owner opens the editor with no other members', async () => {
      group.apply([
        { r: 'member', id: 'p-member', seq: 1000, gone: true },
        { ...ai, seq: ai.seq + 1, cli: 'gemini', allow: [] },
      ]);
      connect();
      showPanel(group);
      await edit();
    });
    Then('both lists have no choices and Save is disabled', () => {
      expect(text('.member .av')).toBe('C');
      expect(texts('form fieldset .hint')).toEqual(['None', 'None']);
      expect(all('form input')).toHaveLength(0);
      expect(save().disabled).toBe(true);
    });
  });
});
