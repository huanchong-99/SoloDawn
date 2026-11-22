import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync, tick } from 'svelte';
import { expect, vi } from 'vitest';
import { homes } from '../src/lib/groups.svelte';
import type { Source } from '../src/lib/net.svelte';
import { nodes } from '../src/lib/nodes.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import type * as p from '../src/lib/proto';
import { example, ownedNode, user, type OwnedNode } from './home';
import { all, choose, closePage, fill, go, one, openPage, read, texts } from './page';

const feature = await loadFeature('../features/web_model.feature');
const g = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const device = 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
const id = 'model-owner';
const catalog = [
  { model: 'opus', name: 'Opus', efforts: ['low', 'high'], fast: true as const, tier: 'strong' },
  { model: 'haiku', name: 'Haiku', efforts: [], tier: 'light' },
];
const agent: p.HostInfo['agents'][number] = {
  agent: 'claude',
  cli_version: '1.0.0',
  account: 'personal',
  login_ok: true,
  caps: {},
  driver: { accounts: ['personal'], takeover: 'new' },
  groups: [],
  models: catalog,
};

vi.mock('../src/lib/net.svelte', async (actual) => ({
  ...(await actual<typeof import('../src/lib/net.svelte')>()),
  start: (source: Source) => ({ net: { source, homes, path: () => undefined }, stop: () => {} }),
}));

const card = () => one<HTMLElement>('.member');
const editor = () => one<HTMLFormElement>('form[aria-label="Change model"]');
const edit = () => one<HTMLButtonElement>('.member button[aria-label="Change model"]');
const refresh = () => one<HTMLButtonElement>('.member button[aria-label="Refresh the models"]');
const save = () => one<HTMLButtonElement>('form[aria-label="Change model"] .btn-primary');
const selects = (within: ParentNode) => [...within.querySelectorAll<HTMLSelectElement>('select')];
const submit = (form: HTMLFormElement) =>
  form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
const options = (select: HTMLSelectElement) => [...select.options].map((o) => [o.value, read(o)]);
const kv = (within: Element, label: string) =>
  [...within.querySelectorAll('dt')].find((dt) => read(dt) === label)!.nextElementSibling!;
const modelText = (within: ParentNode) =>
  within.querySelector<HTMLInputElement>(
    'input[placeholder="Model id; empty for the CLI\'s default"]',
  )!;
const effortText = (within: ParentNode) =>
  within.querySelector<HTMLInputElement>(
    'input[placeholder="Level; empty for the CLI\'s default"]',
  )!;

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let node: OwnedNode;
  let sent: Record<string, unknown>[];
  let resolveAnswer: (answer: p.CmdResult | p.ErrorResponse) => void;
  const stored = () => node.store.groups[0].member(id) as p.AIMemberRow;
  const body = () => sent.map(({ n, ts, ...payload }) => payload);
  const setModels = (models: p.HostInfo['agents'][number]['models']) => {
    node.info.agents[0].models = models;
    nodes.infos.set(node.store, structuredClone(node.info));
    flushSync();
  };
  const openEditor = async () => {
    edit().click();
    await tick();
    flushSync();
  };
  const openAdd = async () => {
    one<HTMLButtonElement>('.members .head button').click();
    await vi.waitFor(() => {
      flushSync();
      expect(all('dialog .choice')).toHaveLength(2);
    });
  };
  const submitAdd = async () => {
    submit(one<HTMLFormElement>('dialog form'));
    await vi.waitFor(() => expect(sent).toHaveLength(1));
  };
  const pickOpus = () => {
    choose(selects(one('dialog'))[0], 'Opus');
    flushSync();
    expect(options(selects(one('dialog'))[1])).toEqual([
      ['', "The CLI's default"],
      ['low', 'low'],
      ['high', 'high'],
    ]);
    choose(selects(one('dialog'))[1], 'high');
    flushSync();
  };

  AfterEachScenario(async () => {
    await closePage();
    nodes.infos.clear();
    prefs.lang = 'en';
  });
  Background(({ Given }) => {
    Given('the Website group and its model catalog open in English', async () => {
      node = ownedNode('workstation', device, {
        agents: [structuredClone(agent), { ...structuredClone(agent), agent: 'codex' }],
      });
      sent = [];
      node.home.reply = (payload) => {
        sent.push(payload);
        return { ok: true };
      };
      node.home.add(g, 'Website');
      node.home.write(
        g,
        example('row-member-ai', {
          id,
          name: 'Opus member',
          user,
          host: device,
          cli: 'claude',
          secretary: true,
          model: 'opus',
          effort: 'high',
          tier: 'light',
          skills: 'Protocol design',
        }),
      );
      node.home.write(
        g,
        example('row-member-ai', {
          id: 'model-other',
          name: 'Default member',
          user: 'other-user',
          host: device,
          cli: 'claude',
          secretary: false,
          model: '',
          effort: '',
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

  Scenario('Adding another member of the same CLI chooses a listed model', ({ When, Then }) => {
    When('the owner opens Add AI', openAdd);
    Then('the existing CLI is selectable again with a free member name', () => {
      expect(
        all<HTMLInputElement>('dialog .choice input').map((i) => [i.disabled, i.checked]),
      ).toEqual([
        [false, true],
        [false, false],
      ]);
      expect(one<HTMLInputElement>('dialog input[required]').value).toBe('Claude Code');
      expect(options(selects(one('dialog'))[0])).toEqual([
        ['', "The CLI's default"],
        ['opus', 'Opus'],
        ['haiku', 'Haiku'],
      ]);
    });
    When('the owner chooses Opus and high', pickOpus);
    Then('fast mode is shown and the suggested tier is Strong', () => {
      expect(texts('dialog .fast')).toEqual(['Fast mode']);
      expect(one<HTMLInputElement>('dialog input[value="strong"]').checked).toBe(true);
    });
    When('the owner submits Add AI', submitAdd);
    Then('host.join sends the exact listed model and level', () =>
      expect(body()).toEqual([
        {
          t: 'host.join',
          to: device,
          g,
          home: device,
          agent: 'claude',
          name: 'Claude Code',
          secretary: false,
          model: 'opus',
          effort: 'high',
          tier: 'strong',
          skills: '',
        },
      ]),
    );
  });

  Scenario('An empty catalog offers model and level text fields', ({ Given, When, And, Then }) => {
    Given('the CLI lists no models', () => setModels([]));
    When('the owner opens Add AI', openAdd);
    And('types a custom model and level', () => {
      expect(selects(one('dialog'))).toEqual([]);
      fill(modelText(one('dialog')), 'custom/model');
      fill(effortText(one('dialog')), 'deep');
      flushSync();
    });
    And('the owner submits Add AI', submitAdd);
    Then('host.join sends the exact typed model and level', () =>
      expect(body()).toEqual([
        {
          t: 'host.join',
          to: device,
          g,
          home: device,
          agent: 'claude',
          name: 'Claude Code',
          secretary: false,
          model: 'custom/model',
          effort: 'deep',
          tier: 'standard',
          skills: '',
        },
      ]),
    );
  });

  Scenario(
    'Model choices reset dependent values and candidates reset the draft',
    ({ When, And, Then }) => {
      When('the owner opens Add AI', openAdd);
      And('the owner chooses Opus and high', pickOpus);
      And('chooses Haiku without levels', () => {
        choose(selects(one('dialog'))[0], 'Haiku');
        flushSync();
      });
      Then('no level is offered and the tier is Light', () => {
        expect(selects(one('dialog'))).toHaveLength(1);
        expect(texts('dialog .fast')).toEqual([]);
        expect(one<HTMLInputElement>('dialog input[value="light"]').checked).toBe(true);
      });
      When('the owner returns to the CLI defaults', () => {
        choose(selects(one('dialog'))[0], "The CLI's default");
        flushSync();
      });
      Then('no level is offered and the tier is Standard', () => {
        expect(selects(one('dialog'))).toHaveLength(1);
        expect(one<HTMLInputElement>('dialog input[value="standard"]').checked).toBe(true);
      });
      When('the owner changes computer CLI candidates', () => {
        pickOpus();
        all<HTMLInputElement>('dialog .choice input')[1].click();
        flushSync();
      });
      Then('the model and level draft return to the CLI defaults', () => {
        expect(selects(one('dialog'))[0].value).toBe('');
        expect(selects(one('dialog'))).toHaveLength(1);
        expect(one<HTMLInputElement>('dialog input[value="standard"]').checked).toBe(true);
      });
    },
  );

  Scenario(
    'Everyone sees model names and levels but only the owner can edit',
    ({ Then, And, When }) => {
      Then('the owner card shows Opus and high and the other card shows CLI defaults', () => {
        expect(read(kv(card(), 'Model').querySelector('.name')!)).toBe('Opus');
        expect(read(kv(card(), 'Level'))).toBe('high');
        expect(read(kv(all('.member')[1], 'Model'))).toBe("The CLI's default");
        expect(read(kv(all('.member')[1], 'Level'))).toBe("The CLI's default");
      });
      And('the other card has no model editor or refresh', () => {
        expect(all('.member')[1].querySelector('[aria-label="Change model"]')).toBeNull();
        expect(all('.member')[1].querySelector('[aria-label="Refresh the models"]')).toBeNull();
      });
      When('the browser switches to Chinese', () => {
        prefs.lang = 'zh-CN';
        flushSync();
      });
      Then('model labels and defaults are translated', async () => {
        expect(read(kv(card(), '模型').querySelector('.name')!)).toBe('Opus');
        expect(read(kv(card(), '思考级别'))).toBe('high');
        expect(read(kv(all('.member')[1], '模型'))).toBe('CLI 默认');
        expect(one('[aria-label="刷新模型列表"]').getAttribute('title')).toBe('刷新模型列表');
        one<HTMLButtonElement>('[aria-label="更改模型"]').click();
        await tick();
        flushSync();
        choose(selects(one('form[aria-label="更改模型"]'))[0], 'Opus');
        flushSync();
        expect(texts('form[aria-label="更改模型"] .fast')).toEqual(['快速模式']);
      });
    },
  );

  Scenario(
    'Refresh changes the catalog without changing the member',
    ({ Given, Then, When, And }) => {
      Given('the stored model is no longer listed', () => setModels([catalog[1]]));
      Then('the card and editor preserve the unlisted model', async () => {
        expect(read(kv(card(), 'Model').querySelector('.badge')!)).toBe('Not listed');
        await openEditor();
        expect(options(selects(editor())[0])).toEqual([
          ['', "The CLI's default"],
          ['haiku', 'Haiku'],
          ['opus', 'opus · Not listed'],
        ]);
        one<HTMLButtonElement>('form[aria-label="Change model"] .btn-ghost').click();
        await tick();
        flushSync();
      });
      When('the owner refreshes after the catalog gains the stored model', async () => {
        node.info.agents[0].models = [{ ...catalog[0], name: 'Opus updated' }, catalog[1]];
        const request = vi.spyOn(node.store.ctl!, 'request');
        refresh().click();
        flushSync();
        expect(refresh().disabled).toBe(true);
        await vi.waitFor(() => {
          flushSync();
          expect(read(kv(card(), 'Model').querySelector('.name')!)).toBe('Opus updated');
        });
        expect(request.mock.calls.map(([req]) => req)).toEqual([{ t: 'host.info', refresh: true }]);
      });
      Then('refresh true is sent and the model name and badge update without a restart', () => {
        expect(kv(card(), 'Model').querySelector('.badge')).toBeNull();
        expect(refresh().disabled).toBe(false);
      });
      And('the stored model level tier and skills are unchanged', () => {
        expect({
          model: stored().model,
          effort: stored().effort,
          tier: stored().tier,
          skills: stored().skills,
        }).toEqual({
          model: 'opus',
          effort: 'high',
          tier: 'light',
          skills: 'Protocol design',
        });
        expect(body()).toEqual([]);
      });
    },
  );

  Scenario('The owner changes only model and level and can cancel', ({ When, Then, And }) => {
    When('the owner opens the model editor', openEditor);
    Then('Save is disabled and the editor has focus', () => {
      expect(save().disabled).toBe(true);
      expect(document.activeElement).toBe(editor());
    });
    When('the owner chooses Haiku in the editor', () => {
      choose(selects(editor())[0], 'Haiku');
      flushSync();
    });
    And('saves the model editor', async () => {
      submit(editor());
      await vi.waitFor(() => {
        flushSync();
        expect(all('form[aria-label="Change model"]')).toHaveLength(0);
      });
    });
    Then('exactly one member set changes only model and level', () =>
      expect(body()).toEqual([{ t: 'member', g, op: 'set', id, model: 'haiku', effort: '' }]),
    );
    When('the owner opens the model editor again and cancels', async () => {
      await openEditor();
      one<HTMLButtonElement>('form[aria-label="Change model"] .btn-ghost').click();
      await tick();
      flushSync();
    });
    Then('focus returns to Change model', () => expect(document.activeElement).toBe(edit()));
  });

  Scenario(
    'A failed edit stays open and offline controls are disabled',
    ({ Given, When, And, Then }) => {
      Given('the CLI lists no models', () => setModels([]));
      When('the owner opens the model editor', openEditor);
      And('saves typed model and level while the home holds its answer', () => {
        vi.spyOn(node.store, 'command').mockImplementation(
          () =>
            new Promise((resolve) => {
              resolveAnswer = resolve;
            }),
        );
        fill(modelText(editor()), 'typed/model');
        fill(effortText(editor()), 'deep');
        flushSync();
        submit(editor());
        flushSync();
      });
      Then('the editor is disabled while saving', () => {
        expect(save().disabled).toBe(true);
        expect(modelText(editor()).disabled).toBe(true);
        expect(effortText(editor()).disabled).toBe(true);
      });
      When('the home refuses the model change', async () => {
        resolveAnswer({ re: 1, err: { code: 'model_denied', msg: 'refused' } });
        await tick();
        flushSync();
      });
      Then('the exact model error is shown', () => {
        expect(texts('.member [role="alert"]')).toEqual([
          'Not sent. The home answered: model_denied',
        ]);
        expect(save().disabled).toBe(false);
      });
      When('the home disconnects', () => {
        node.drop();
        flushSync();
      });
      Then('Save and refresh are disabled', () => {
        expect(save().disabled).toBe(true);
        expect(refresh().disabled).toBe(true);
        expect(modelText(editor()).disabled).toBe(true);
      });
    },
  );

  Scenario('Computers lists two members of one CLI in one group', ({ When, Then }) => {
    When('the browser opens Computers with two hosted members of one CLI', () => {
      node.info.agents[0].groups = [id, 'model-other'].map((member) => ({
        g,
        member,
        level: 1,
        state: 'dormant',
        queued: 0,
        watchers: [],
      }));
      nodes.infos.set(node.store, structuredClone(node.info));
      go('#/nodes');
      flushSync();
    });
    Then('both rows show Website and their distinct member names', () => {
      expect(texts('.perm .row')).toEqual([
        'Website · Opus member Secretary',
        'Website · Default member',
      ]);
      expect(texts('.perm a')).toEqual(['Website', 'Website']);
    });
  });
});
