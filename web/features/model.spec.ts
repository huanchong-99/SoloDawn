import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { chromium } from 'playwright';
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
const editor = () => one<HTMLFormElement>('form[aria-label="Edit profile"]');
const edit = () =>
  [...card().querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => read(b) === 'Edit profile',
  )!;
const refresh = () => one<HTMLButtonElement>('.member button[aria-label="Refresh the models"]');
const save = () => one<HTMLButtonElement>('form[aria-label="Edit profile"] .btn-primary');
const cancel = () => one<HTMLButtonElement>('form[aria-label="Edit profile"] .btn-ghost');
const selects = (within: ParentNode) => [...within.querySelectorAll<HTMLSelectElement>('select')];
const submit = (form: HTMLFormElement) =>
  form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
const options = (select: HTMLSelectElement) => [...select.options].map((o) => [o.value, read(o)]);
const kv = (within: Element, label: string) =>
  [...within.querySelectorAll('dt')].find((dt) => read(dt) === label)!.nextElementSibling!;
const modelText = (within: ParentNode) =>
  within.querySelector<HTMLInputElement>('input[placeholder="e.g. opus"]')!;
const effortText = (within: ParentNode) =>
  within.querySelector<HTMLInputElement>('input[placeholder="e.g. high"]')!;

async function nativeLimits(form: HTMLFormElement) {
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage();
    await page.setContent(form.outerHTML);
    return await page.evaluate(
      (values) => {
        const form = document.querySelector('form')!;
        form.querySelectorAll('input').forEach((input, index) => {
          input.value = values[index];
        });
        const input = form.querySelector<HTMLInputElement>('input[pattern=".{0,100}"]')!;
        let submits = 0;
        form.addEventListener('submit', (event) => {
          event.preventDefault();
          submits++;
        });
        return [101, 100].map((length) => {
          input.value = '😀'.repeat(length);
          const mismatch = input.validity.patternMismatch;
          form.requestSubmit();
          return { length, mismatch, submits };
        });
      },
      [...form.querySelectorAll('input')].map((input) => input.value),
    );
  } finally {
    await browser.close();
  }
}

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
    await vi.waitFor(() =>
      expect(one('dialog .path-row code').textContent?.trim()).toBe('No folder chosen'),
    );
    expect(one<HTMLButtonElement>('dialog footer .btn-primary').disabled).toBe(true);
    one<HTMLButtonElement>('dialog .path-row .change').click();
    flushSync();
    await vi.waitFor(() => {
      expect(one<HTMLDialogElement>('dialog dialog').open).toBe(true);
      expect(one<HTMLButtonElement>('dialog dialog footer .btn-primary').disabled).toBe(false);
    });
    one<HTMLButtonElement>('dialog dialog footer .btn-primary').click();
    flushSync();
    expect(one('dialog .path-row code').textContent).toContain('/');
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
    Then(
      'the existing CLI says In this group · adds another member and has a free member name',
      () => {
        expect(
          all<HTMLInputElement>('dialog .choice input').map((i) => [i.disabled, i.checked]),
        ).toEqual([
          [false, true],
          [false, false],
        ]);
        expect(one<HTMLInputElement>('dialog input[required]').value).toBe('Claude Code');
        expect(texts('dialog .choice .sub')[0]).toBe('In this group · adds another member');
        expect(options(selects(one('dialog'))[0])).toEqual([
          ['', "The CLI's default"],
          ['opus', 'Opus'],
          ['haiku', 'Haiku'],
        ]);
      },
    );
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
          workspace: '/home/dev/website',
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
          workspace: '/home/dev/website',
          model: 'custom/model',
          effort: 'deep',
          tier: 'standard',
          skills: '',
        },
      ]),
    );
  });

  Scenario(
    'Model text limits count code points and block invalid submissions',
    ({ Given, When, Then, And }) => {
      Given('the CLI lists no models', () => setModels([]));
      When('the owner opens the profile editor', openEditor);
      Then('101 astral model code points cannot be saved but 100 are valid', async () => {
        fill(modelText(editor()), '😀'.repeat(101));
        flushSync();
        expect(modelText(editor()).getAttribute('pattern')).toBe('.{0,100}');
        expect(effortText(editor()).getAttribute('pattern')).toBe('.{0,32}');
        expect(modelText(editor()).validity.patternMismatch).toBe(true);
        editor().requestSubmit();
        await tick();
        expect(body()).toEqual([]);
        expect(await nativeLimits(editor())).toEqual([
          { length: 101, mismatch: true, submits: 0 },
          { length: 100, mismatch: false, submits: 1 },
        ]);
        cancel().click();
        await tick();
      });
      When('the owner opens Add AI', openAdd);
      And('101 astral model code points cannot join but 100 are valid', async () => {
        const form = one<HTMLFormElement>('dialog form');
        fill(modelText(form), '😀'.repeat(101));
        flushSync();
        expect(modelText(form).validity.patternMismatch).toBe(true);
        form.requestSubmit();
        await tick();
        expect(body()).toEqual([]);
        expect(await nativeLimits(form)).toEqual([
          { length: 101, mismatch: true, submits: 0 },
          { length: 100, mismatch: false, submits: 1 },
        ]);
      });
    },
  );

  Scenario(
    'Model choices reset dependent values and candidates reset the draft',
    ({ When, And, Then }) => {
      When('the owner opens Add AI', openAdd);
      And('the owner chooses Opus and high', pickOpus);
      And('chooses Haiku without levels', () => {
        choose(selects(one('dialog'))[0], 'Haiku');
        flushSync();
      });
      Then(
        "the level offers only the CLI's default, says the model has none, and the tier is Light",
        () => {
          expect(options(selects(one('dialog'))[1])).toEqual([['', "The CLI's default"]]);
          expect(selects(one('dialog'))[1].disabled).toBe(true);
          expect(texts('dialog .why')).toEqual(['This model has no levels']);
          expect(texts('dialog .fast')).toEqual([]);
          expect(one<HTMLInputElement>('dialog input[value="light"]').checked).toBe(true);
        },
      );
      When('the owner returns to the CLI defaults', () => {
        choose(selects(one('dialog'))[0], "The CLI's default");
        flushSync();
      });
      Then(
        "the level offers only the CLI's default, asks for a model first, and the tier is Standard",
        () => {
          expect(options(selects(one('dialog'))[1])).toEqual([['', "The CLI's default"]]);
          expect(selects(one('dialog'))[1].disabled).toBe(true);
          expect(texts('dialog .why')).toEqual(['Choose a model to see its levels']);
          expect(one<HTMLInputElement>('dialog input[value="standard"]').checked).toBe(true);
        },
      );
      When('the owner changes computer CLI candidates', () => {
        pickOpus();
        all<HTMLInputElement>('dialog .choice input')[1].click();
        flushSync();
        one<HTMLButtonElement>('dialog [role="switch"]').click();
        flushSync();
      });
      Then('the model and level draft return to the CLI defaults', () => {
        expect(selects(one('dialog'))[0].value).toBe('');
        expect(selects(one('dialog'))[1].value).toBe('');
        expect(selects(one('dialog'))[1].disabled).toBe(true);
        expect(one<HTMLInputElement>('dialog input[value="standard"]').checked).toBe(true);
      });
    },
  );

  Scenario(
    'Everyone sees model names and levels but only the owner can edit',
    ({ Then, And, When }) => {
      Then('the owner card shows Opus and high and the other card shows CLI defaults', () => {
        expect(read(kv(card(), 'Model').querySelector('.name')!)).toBe('Opus');
        expect(read(kv(card(), 'Thinking level'))).toBe('high');
        expect(kv(card(), 'Model').classList.contains('line')).toBe(false);
        expect(read(kv(card(), 'Model').querySelector('.what')!)).toBe('Opus');
        expect(
          kv(card(), 'Model').querySelector('[aria-label="Refresh the models"]'),
        ).not.toBeNull();
        expect(read(kv(all('.member')[1], 'Model'))).toBe("The CLI's default");
        expect(read(kv(all('.member')[1], 'Thinking level'))).toBe("The CLI's default");
      });
      And('the other card has no profile editor or refresh', () => {
        expect([...all('.member')[1].querySelectorAll('button')].map(read)).not.toContain(
          'Edit profile',
        );
        expect(all('.member')[1].querySelector('[aria-label="Refresh the models"]')).toBeNull();
      });
      When('the browser switches to Chinese', () => {
        prefs.lang = 'zh-CN';
        flushSync();
      });
      Then('model labels and defaults are translated', async () => {
        expect(read(kv(card(), '模型').querySelector('.name')!)).toBe('Opus');
        expect(read(kv(card(), '思考等级'))).toBe('high');
        expect(read(kv(all('.member')[1], '模型'))).toBe('CLI 默认');
        expect(one('[aria-label="刷新模型列表"]').getAttribute('title')).toBe('刷新模型列表');
        [...card().querySelectorAll<HTMLButtonElement>('button')]
          .find((b) => read(b) === '编辑档案')!
          .click();
        await tick();
        flushSync();
        choose(selects(one('form[aria-label="编辑档案"]'))[0], 'Opus');
        flushSync();
        expect(texts('form[aria-label="编辑档案"] .fast')).toEqual(['快速模式']);
        // The owner's AI is the secretary, which takes no tasks: no work share.
        expect(all('form[aria-label="编辑档案"] .work')).toEqual([]);
        setModels([]);
        expect(one<HTMLInputElement>('input[placeholder="如 opus"]').placeholder).toBe('如 opus');
        expect(one<HTMLInputElement>('input[placeholder="如 high"]').placeholder).toBe('如 high');
        one<HTMLButtonElement>('form[aria-label="编辑档案"] .btn-ghost').click();
        await tick();
        await openAdd();
        expect(texts('dialog .choice .sub')[0]).toBe('已在群里 · 再加一位');
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
        cancel().click();
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
    When('the owner opens the profile editor', openEditor);
    Then('Save is disabled and the editor has focus', () => {
      expect(save().disabled).toBe(true);
      expect(document.activeElement).toBe(editor());
    });
    When('the owner chooses Haiku in the editor', () => {
      choose(selects(editor())[0], 'Haiku');
      flushSync();
    });
    And('saves the profile editor', async () => {
      submit(editor());
      await vi.waitFor(() => {
        flushSync();
        expect(all('form[aria-label="Edit profile"]')).toHaveLength(0);
      });
    });
    Then('exactly one member set changes only model and level', () =>
      expect(body()).toEqual([{ t: 'member', g, op: 'set', id, model: 'haiku', effort: '' }]),
    );
    When('the owner opens the profile editor again and cancels', async () => {
      await openEditor();
      cancel().click();
      await tick();
      flushSync();
    });
    Then('focus returns to Edit profile', () => expect(document.activeElement).toBe(edit()));
  });

  Scenario(
    'A failed edit stays open and offline controls are disabled',
    ({ Given, When, And, Then }) => {
      Given('the CLI lists no models', () => setModels([]));
      When('the owner opens the profile editor', openEditor);
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
        workspace: '/home/dev/website',
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
