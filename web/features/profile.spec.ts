import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync, tick } from 'svelte';
import { expect, vi } from 'vitest';
import { homes } from '../src/lib/groups.svelte';
import type { Source } from '../src/lib/net.svelte';
import { nodes } from '../src/lib/nodes.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import type * as p from '../src/lib/proto';
import { example, ownedNode, user, type OwnedNode } from './home';
import { all, closePage, fill, one, openPage, read, texts } from './page';

const feature = await loadFeature('../features/web_profile.feature');
const g = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const device = 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
const aiId = 'profile-owner-ai';
const taskId = 'DrtCn6htSBwmMPrFPbHJHA';
const agent: p.HostInfo['agents'][number] = {
  agent: 'claude',
  cli_version: '1.0.0',
  account: 'personal',
  login_ok: true,
  caps: {},
  driver: { accounts: ['personal'], takeover: 'new' },
  groups: [],
  models: [{ model: 'listed', name: 'Listed', efforts: [], tier: 'strong' }],
};

vi.mock('../src/lib/net.svelte', async (actual) => ({
  ...(await actual<typeof import('../src/lib/net.svelte')>()),
  start: (source: Source) => ({ net: { source, homes, path: () => undefined }, stop: () => {} }),
}));

/** The card of the AI named `name`: the members are ordered by name, the secretary first. */
const cardOf = (name: string) => all('.member').find((m) => read(m.querySelector('h3')!) === name)!;
const card = () => cardOf('Owner AI');
const other = () => cardOf('Other AI');
const both = (selector: string) => [card(), other()].map((m) => read(m.querySelector(selector)!));
const button = (label: string, within: ParentNode = card()) =>
  [...within.querySelectorAll<HTMLButtonElement>('button')].find((b) => read(b) === label)!;
const form = () => one<HTMLFormElement>('.member form.editor');
const skills = () => one<HTMLInputElement>('.member .editor input.input');
const submit = (target: HTMLFormElement) =>
  target.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
const commands = (node: OwnedNode) =>
  node.home.asked
    .filter((req) => req.t === 'cmd')
    .map((req) => JSON.parse((req.env as p.Envelope).p) as Record<string, unknown>)
    .filter((cmd) => cmd.t === 'member' || cmd.t === 'host.join');
const withoutEnvelope = ({ n, ts, ...body }: Record<string, unknown>) => body;

function assertSkillsLimit(input: HTMLInputElement, within: HTMLElement, save: HTMLButtonElement) {
  expect(input.hasAttribute('maxlength')).toBe(false);
  fill(input, '😀'.repeat(200));
  flushSync();
  const count = within.querySelector('.count')!;
  expect(read(count)).toBe('200/200');
  expect(count.classList.contains('over')).toBe(false);
  expect(input.getAttribute('aria-invalid')).toBe('false');
  expect(save.disabled).toBe(false);
  fill(input, '😀'.repeat(201));
  flushSync();
  expect(read(count)).toBe('201/200');
  expect(count.classList.contains('over')).toBe(true);
  expect(input.getAttribute('aria-invalid')).toBe('true');
  expect(save.disabled).toBe(true);
}

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let node: OwnedNode;
  let resolveAnswer: (answer: p.CmdResult | p.ErrorResponse) => void;

  const writeAI = (changes: Record<string, unknown> = {}) => {
    node.home.write(
      g,
      example('row-member-ai', {
        id: aiId,
        user,
        name: 'Owner AI',
        host: device,
        cli: 'claude',
        state: 'dormant',
        secretary: false,
        model: 'listed',
        effort: '',
        tier: 'standard',
        skills: 'Protocol design',
        ...changes,
      }),
    );
    flushSync();
  };
  const setSuggestion = (tier: string, model = 'listed') => {
    node.info.agents[0].models = [{ model, name: 'Listed', efforts: [], tier }];
    nodes.infos.set(node.store, structuredClone(node.info));
    flushSync();
  };
  const openEditor = async () => {
    button('Edit profile').click();
    await tick();
    flushSync();
  };

  AfterEachScenario(async () => {
    await closePage();
    nodes.infos.clear();
    prefs.lang = 'en';
  });
  Background(({ Given }) => {
    Given("the Website group open in its owner's browser in English", async () => {
      node = ownedNode('workstation', device, { agents: [structuredClone(agent)] });
      node.home.add(g, 'Website');
      node.home.reply = () => ({ ok: true });
      writeAI();
      node.home.write(
        g,
        example('row-member-ai', {
          id: 'other-ai',
          user: 'other-user',
          name: 'Other AI',
          host: device,
          cli: 'claude',
          model: 'listed',
          tier: 'strong',
          skills: '',
          secretary: false,
        }),
      );
      homes.push(node.store);
      await node.connect();
      openPage(`#/groups/${g}`);
      await vi.waitFor(() => {
        flushSync();
        expect(all('.member')).toHaveLength(2);
        expect(nodes.infos.has(node.store)).toBe(true);
      });
    });
  });

  Scenario('Everyone sees profiles but only the owner can edit', ({ Then, And, When }) => {
    Then('the cards show exact tier and skills values with an empty-skills fallback', () => {
      expect(both('.tier')).toEqual(['Normal', 'More']);
      expect(both('.skills')).toEqual(['Protocol design', 'Not set']);
      expect([...card().querySelectorAll('dt')].map(read)).toContain('Work share');
      expect([...card().querySelectorAll('dt')].map(read)).toContain('Skills');
      expect(card().querySelectorAll('.tier .bars b.on')).toHaveLength(2);
      expect(other().querySelectorAll('.tier .bars b.on')).toHaveLength(3);
    });
    And("the other person's card has no profile editor", () =>
      expect(button('Edit profile', other())).toBeUndefined(),
    );
    When('the browser switches to Chinese', () => {
      prefs.lang = 'zh-CN';
      flushSync();
    });
    Then(
      'the profile labels are 派活 and 擅长, the strong tier is 多, and the share sits with the quota',
      () => {
        expect(both('.tier')).toEqual(['适中', '多']);
        expect(texts('.member .work dt')).toEqual(['派活', '派活']);
        expect(all('.member .work .meter, .member .work .quota-none')).toHaveLength(2);
        expect(other().querySelector('.work')!.getAttribute('aria-label')).toBe('派活与额度');
        expect([...other().querySelectorAll('dt')].map(read)).toContain('派活');
        expect([...other().querySelectorAll('dt')].map(read)).toContain('擅长');
        expect(read(other().querySelector('.skills')!)).toBe('未填写');
        const suggestion = card().querySelector('.suggest')!;
        expect(read(suggestion)).toBe('多 采用');
        expect(suggestion.getAttribute('aria-label')).toBeNull();
        expect(suggestion.getAttribute('title')).toBe('建议：多');
      },
    );
  });

  Scenario(
    "The secretary's card and editor offer no work share, and it comes back with its stored value",
    ({ Given, Then, When }) => {
      Given("the owner's AI is the group's secretary", () => writeAI({ secretary: true }));
      Then(
        'its card shows only the quota in its block, with no work share and no suggestion',
        () => {
          const block = card().querySelector('.work')!;
          expect(block.getAttribute('aria-label')).toBe('Quota');
          expect(block.querySelectorAll('dt, .tier, .suggest')).toHaveLength(0);
          expect(block.querySelectorAll('.meter, .quota-none')).toHaveLength(1);
          expect(other().querySelector('.work .tier')).not.toBeNull();
        },
      );
      When('the owner opens the profile editor', openEditor);
      Then('the editor offers model, thinking level and skills and no work share', () => {
        expect(form().querySelectorAll('select')).toHaveLength(2);
        expect(skills().value).toBe('Protocol design');
        expect(form().querySelectorAll('fieldset, .tiers, input[type=radio]')).toHaveLength(0);
      });
      When(
        'the owner cancels the profile editor and the AI is no longer the secretary',
        async () => {
          button('Cancel').click();
          await tick();
          writeAI({ secretary: false });
        },
      );
      Then('its card shows its stored work share Normal and the suggestion More again', () => {
        expect(card().querySelector('.work')!.getAttribute('aria-label')).toBe(
          'Work share and quota',
        );
        expect(read(card().querySelector('.work .tier')!)).toBe('Normal');
        expect(read(card().querySelector('.suggest')!)).toBe('More Use');
      });
    },
  );

  Scenario('The owner edits a profile and cancels without losing focus', ({ When, Then, And }) => {
    When('the owner opens the profile editor', openEditor);
    Then('Save is disabled until something changes', () => {
      expect(button('Save').disabled).toBe(true);
      expect(document.activeElement).toBe(form());
      expect(skills().hasAttribute('maxlength')).toBe(false);
    });
    When('the owner chooses Light and writes Documentation', () => {
      one<HTMLInputElement>('.editor input[value="light"]').click();
      fill(skills(), 'Documentation');
      flushSync();
      expect(button('Save').disabled).toBe(false);
      expect(texts('.editor .count')).toEqual(['13/200']);
      expect(one<HTMLInputElement>('.editor input[value="light"]').checked).toBe(true);
      expect(one('.editor .seg').getAttribute('style')).toContain('--at: 0');
    });
    And('saves the profile', async () => {
      submit(form());
      await vi.waitFor(() => {
        flushSync();
        expect(all('.editor')).toHaveLength(0);
      });
    });
    Then('exactly one member set sends light and Documentation', () =>
      expect(commands(node).map(withoutEnvelope)).toEqual([
        { t: 'member', g, op: 'set', id: aiId, tier: 'light', skills: 'Documentation' },
      ]),
    );
    When('the owner reopens the profile editor', openEditor);
    And('cancels the profile editor', async () => {
      button('Cancel').click();
      await tick();
      flushSync();
    });
    Then('focus returns to Edit profile', () =>
      expect(document.activeElement).toBe(button('Edit profile')),
    );
  });

  Scenario('Profile skills count Unicode code points at the limit', ({ When, Then }) => {
    When('the owner opens the profile editor', openEditor);
    Then('200 astral characters allow Save and 201 mark skills invalid and disable Save', () => {
      assertSkillsLimit(skills(), form(), button('Save'));
      expect(commands(node)).toEqual([]);
    });
  });

  Scenario('A model suggestion is an explicit owner choice', ({ Then, When, And }) => {
    Then("the owner's card offers More and Use labelled Suggested: More", () => {
      const suggestion = card().querySelector('.suggest')!;
      expect(read(suggestion)).toBe('More Use');
      expect(suggestion.getAttribute('aria-label')).toBeNull();
      expect(suggestion.getAttribute('title')).toBe('Suggested: More');
    });
    When('the owner uses the suggestion', async () => {
      one<HTMLButtonElement>('.suggest').click();
      await vi.waitFor(() => expect(commands(node)).toHaveLength(1));
    });
    Then('exactly one member set sends only tier strong', () =>
      expect(commands(node).map(withoutEnvelope)).toEqual([
        { t: 'member', g, op: 'set', id: aiId, tier: 'strong' },
      ]),
    );
    When('the suggestion matches the stored tier', () => setSuggestion('standard'));
    Then('no suggestion is offered', () => expect(all('.suggest')).toHaveLength(0));
    When('the model is no longer listed', () => setSuggestion('strong', 'different'));
    Then('no suggestion is offered for the unlisted model', () =>
      expect(all('.suggest')).toHaveLength(0),
    );
    And("the other person's card has no suggestion", () => {
      setSuggestion('light');
      expect(other().querySelector('.suggest')).toBeNull();
    });
  });

  Scenario('A stuck task asks for a stronger AI', ({ When, Then }) => {
    When('a task card arrives with stuck ask true', () => {
      node.home.write(
        g,
        example('row-task', {
          state: 'active',
          wait: null,
          review: undefined,
          stuck: { kind: 'review', n: 2, ask: true },
        }),
      );
      node.home.write(g, example('row-msg-task'));
      flushSync();
    });
    Then('its badge is Must ask a stronger AI', () =>
      expect(texts('.msg.sys .card-head .badge')).toEqual(['Must ask a stronger AI']),
    );
    When("the task's stuck ask becomes false", () => {
      node.home.write(
        g,
        example('row-task', {
          id: taskId,
          state: 'active',
          wait: null,
          review: undefined,
          stuck: { kind: 'review', n: 2, ask: false },
        }),
      );
      flushSync();
    });
    Then('its badge is In progress', () =>
      expect(texts('.msg.sys .card-head .badge')).toEqual(['In progress']),
    );
  });

  Scenario('Adding an AI sends the chosen profile with CLI defaults', ({ When, And, Then }) => {
    When('the owner opens Add AI and chooses Light with skills Documentation', async () => {
      node.info.agents.push({ ...structuredClone(agent), agent: 'codex', models: [] });
      one<HTMLButtonElement>('.members .head button').click();
      await vi.waitFor(() => {
        flushSync();
        expect(all('dialog .choice')).toHaveLength(2);
      });
      all<HTMLInputElement>('dialog .choice input')[1].click();
      flushSync();
      await vi.waitFor(() =>
        expect(one('dialog .path-row code').textContent?.trim()).toBe('No folder chosen'),
      );
      expect(one<HTMLButtonElement>('dialog .btn-primary').disabled).toBe(true);
      one<HTMLButtonElement>('dialog .path-row .change').click();
      flushSync();
      await vi.waitFor(() =>
        expect(one<HTMLButtonElement>('dialog dialog[open] footer .btn-primary').disabled).toBe(
          false,
        ),
      );
      one<HTMLButtonElement>('dialog dialog[open] footer .btn-primary').click();
      flushSync();
      expect(one('dialog .path-row code').textContent).toContain('/');
      one<HTMLButtonElement>('dialog [role="switch"]').click();
      flushSync();
      one<HTMLInputElement>('dialog input[value="light"]').click();
      const input = one<HTMLInputElement>(
        'dialog input[placeholder="What this AI is good at, in one line"]',
      );
      assertSkillsLimit(input, one('dialog'), one<HTMLButtonElement>('dialog .btn-primary'));
      fill(input, 'Documentation');
      flushSync();
      expect(one<HTMLButtonElement>('dialog .btn-primary').disabled).toBe(false);
    });
    And('submits the add-AI dialog', async () => {
      submit(one<HTMLFormElement>('dialog form'));
      await vi.waitFor(() => expect(commands(node)).toHaveLength(1));
    });
    Then('host.join contains the exact profile and CLI defaults', () =>
      expect(commands(node).map(withoutEnvelope)).toEqual([
        {
          t: 'host.join',
          to: device,
          g,
          home: device,
          agent: 'codex',
          name: 'Codex',
          secretary: false,
          workspace: '/home/dev/website',
          model: '',
          effort: '',
          tier: 'light',
          skills: 'Documentation',
        },
      ]),
    );
  });

  Scenario('Profile errors, busy state and offline controls are visible', ({ When, And, Then }) => {
    When('the owner opens the profile editor', openEditor);
    And('saves changed skills while the home holds its answer', () => {
      vi.spyOn(node.store, 'command').mockImplementation(
        () =>
          new Promise((resolve) => {
            resolveAnswer = resolve;
          }),
      );
      fill(skills(), 'Changed');
      flushSync();
      submit(form());
      flushSync();
    });
    Then('the editor and suggestion are disabled while sending', () => {
      expect(skills().disabled).toBe(true);
      expect(button('Save').disabled).toBe(true);
      expect(one<HTMLFieldSetElement>('.editor fieldset').disabled).toBe(true);
      expect(one<HTMLButtonElement>('.suggest').disabled).toBe(true);
    });
    When('the home refuses the change', async () => {
      resolveAnswer({ re: 1, err: { code: 'forbidden', msg: 'refused' } });
      await tick();
      flushSync();
    });
    Then('the profile error is visible and the editor stays open', () => {
      expect(all('.editor')).toHaveLength(1);
      expect(all('.member [role="alert"]')).toHaveLength(1);
      expect(button('Save').disabled).toBe(false);
    });
    When('the owner cancels and reopens the editor', async () => {
      button('Cancel').click();
      await tick();
      await openEditor();
    });
    Then('the error is cleared and the stored skills are restored', () => {
      expect(all('.member [role="alert"]')).toHaveLength(0);
      expect(skills().value).toBe('Protocol design');
    });
    When('the group home disconnects', () => {
      node.drop();
      flushSync();
    });
    Then('the editor and suggestion are disabled offline', () => {
      expect(skills().disabled).toBe(true);
      expect(button('Save').disabled).toBe(true);
      expect(one<HTMLButtonElement>('.suggest').disabled).toBe(true);
    });
  });

  Scenario(
    'The card says whether the group follows, and restores the defaults',
    ({ Given, Then, When, And }) => {
      Given('the owner\'s Claude Code in "Website" follows the defaults', () => {
        writeAI({ follow: true });
      });
      Then('its card says "Follows the defaults on "workstation""', () => {
        expect([...card().querySelectorAll('dt')].map(read)).toContain('Setting');
        const row = card().querySelector('.setting')!;
        expect(read(row)).toBe('Follows the defaults on "workstation"');
        expect(row.classList.contains('muted')).toBe(true);
        expect(row.querySelector('.badge')).toBeNull();
        expect(button('Restore defaults')).toBeUndefined();
      });
      When('the owner opens the profile editor', openEditor);
      Then('the editor says saving gives this group its own setting', () => {
        expect(read(form().querySelector('.hint')!)).toBe(
          'Saving gives this group its own setting; it stops following the defaults.',
        );
      });
      When('the owner chooses Less and saves the profile', async () => {
        one<HTMLInputElement>('.editor input[value="light"]').click();
        flushSync();
        submit(form());
        await vi.waitFor(() => {
          flushSync();
          expect(all('.editor')).toHaveLength(0);
        });
      });
      Then('exactly one member set sends tier "light" and no follow', () => {
        const sent = commands(node).map(withoutEnvelope);
        expect(sent).toEqual([{ t: 'member', g, op: 'set', id: aiId, tier: 'light' }]);
        expect('follow' in sent[0]).toBe(false);
      });
      When('the row comes back without follow', () => {
        writeAI({ tier: 'light' });
      });
      Then('the card shows "Own setting in this group" and "Restore defaults"', () => {
        const row = card().querySelector('.setting')!;
        expect(row.hasAttribute('data-changed')).toBe(true);
        expect(row.classList.contains('muted')).toBe(false);
        expect(read(row.querySelector('.badge')!)).toBe('Own setting in this group');
        expect(button('Restore defaults').disabled).toBe(false);
      });
      When('the owner restores the defaults', async () => {
        button('Restore defaults').click();
        await vi.waitFor(() => expect(commands(node)).toHaveLength(2));
      });
      Then('exactly one member set sends follow alone', () => {
        expect(commands(node).map(withoutEnvelope).slice(1)).toEqual([
          { t: 'member', g, op: 'set', id: aiId, follow: true },
        ]);
      });
      And('another person\'s card shows the setting line but no "Restore defaults"', () => {
        const row = other().querySelector('.setting')!;
        expect(row.hasAttribute('data-changed')).toBe(false);
        expect(read(row.querySelector('.badge')!)).toBe('Own setting in this group');
        expect(button('Restore defaults', other())).toBeUndefined();
        node.home.write(
          g,
          example('row-member-ai', {
            id: 'other-ai',
            user: 'other-user',
            name: 'Other AI',
            host: device,
            cli: 'claude',
            model: 'listed',
            tier: 'strong',
            skills: '',
            secretary: false,
            follow: true,
          }),
        );
        flushSync();
        expect(read(other().querySelector('.setting')!)).toBe(
          'Follows the defaults on "workstation"',
        );
        expect(other().querySelector('.setting')!.hasAttribute('data-changed')).toBe(true);
      });
      And("an external AI's card has no setting line", () => {
        node.home.write(
          g,
          example('row-member-ai-external', { id: 'external-ai', user, name: 'External AI' }),
        );
        flushSync();
        const ext = cardOf('External AI');
        expect([...ext.querySelectorAll('dt')].map(read)).not.toContain('Setting');
        expect(ext.querySelector('.setting')).toBeNull();
      });
    },
  );

  Scenario(
    'Restoring defaults handles busy state, refusal, offline state and Chinese labels',
    ({ Given, Then, When }) => {
      Given('the owner\'s Claude Code in "Website" has its own setting', () => {});
      Then('the setting row starts without a change marker', () => {
        expect(card().querySelector('.setting')!.hasAttribute('data-changed')).toBe(false);
      });
      When('the owner restores the defaults while the home holds its answer', () => {
        vi.spyOn(node.store, 'command').mockImplementation(
          () =>
            new Promise((resolve) => {
              resolveAnswer = resolve;
            }),
        );
        button('Restore defaults').click();
        flushSync();
      });
      Then('Restore defaults is disabled while sending', () => {
        expect(button('Restore defaults').disabled).toBe(true);
      });
      When('the home refuses the restore', async () => {
        resolveAnswer({ re: 1, err: { code: 'forbidden', msg: 'refused' } });
        await tick();
        flushSync();
      });
      Then('the profile error is visible and Restore defaults is enabled again', () => {
        expect(all('.member [role="alert"]')).toHaveLength(1);
        expect(button('Restore defaults').disabled).toBe(false);
      });
      When('the browser switches to Chinese', () => {
        prefs.lang = 'zh-CN';
        flushSync();
      });
      Then('the setting row and follow note use the Chinese labels', async () => {
        expect([...card().querySelectorAll('dt')].map(read)).toContain('设置');
        expect(read(card().querySelector('.setting .badge')!)).toBe('本群单独设置');
        expect(read(card().querySelector('.setting button')!)).toBe('恢复默认');
        writeAI({ follow: true });
        expect(read(card().querySelector('.setting')!)).toBe('跟随「workstation」上的默认设置');
        button('编辑档案').click();
        await tick();
        flushSync();
        expect(read(form().querySelector('.hint')!)).toBe(
          '保存后，这个群用自己的设置，不再跟随默认。',
        );
        button('取消').click();
        await tick();
        writeAI();
      });
      When('the group home disconnects', () => {
        node.drop();
        flushSync();
      });
      Then('Restore defaults is disabled offline', () => {
        expect(one<HTMLButtonElement>('.setting button').disabled).toBe(true);
      });
    },
  );
});
