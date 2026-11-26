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

const card = () => one<HTMLElement>('.member');
const other = () => all('.member')[1];
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
        secretary: true,
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
      expect(texts('.member .tier')).toEqual(['Standard', 'Strong']);
      expect(texts('.member .skills')).toEqual(['Protocol design', 'Not set']);
      expect([...card().querySelectorAll('dt')].map(read)).toContain('Tier');
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
    Then('the profile labels are 档位 and 擅长 and the strong tier is 强', () => {
      expect(texts('.member .tier')).toEqual(['标准', '强']);
      expect([...other().querySelectorAll('dt')].map(read)).toContain('档位');
      expect([...other().querySelectorAll('dt')].map(read)).toContain('擅长');
      expect(read(other().querySelector('.skills')!)).toBe('未填写');
      const suggestion = card().querySelector('.suggest')!;
      expect(read(suggestion)).toBe('强 采用');
      expect(suggestion.getAttribute('aria-label')).toBeNull();
      expect(suggestion.getAttribute('title')).toBe('建议：强');
    });
  });

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
    Then("the owner's card offers Strong and Use labelled Suggested: Strong", () => {
      const suggestion = card().querySelector('.suggest')!;
      expect(read(suggestion)).toBe('Strong Use');
      expect(suggestion.getAttribute('aria-label')).toBeNull();
      expect(suggestion.getAttribute('title')).toBe('Suggested: Strong');
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
      await vi.waitFor(() => expect(one('dialog .path-row code').textContent).toContain('/'));
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
      await vi.waitFor(() => expect(one('dialog .path-row code').textContent).toContain('/'));
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
});
