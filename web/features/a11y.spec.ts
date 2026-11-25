import 'fake-indexeddb/auto';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { IDBFactory } from 'fake-indexeddb';
import { flushSync, tick } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { expect, vi } from 'vitest';
import { find, type Home, homes } from '../src/lib/groups.svelte';
import type { Path, Source } from '../src/lib/net.svelte';
import { nodes } from '../src/lib/nodes.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import * as p from '../src/lib/proto';
import { ActHome, browser, example, ownedNode, owner, user } from './home';
import { all, closePage, field, fill, go, one, openPage, read, text, texts, type } from './page';
import { FakePC } from './peer-fakes';

const feature = await loadFeature('../features/web_a11y.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const docs = 'ZG9jcy1ncm91cC0wMDAwMQ';
const secretary = 'qBSFMsr2hHYKOMblEA_kdA';
const codex = '1PC8WineBrUQ-apCjx7tug';
const claude = 'LXCZmuGAXkvO-bSrOkuCfw';
const taskId = 'DrtCn6htSBwmMPrFPbHJHA';
const briefId = 'KaiCW9JC8UOG7lKNduDo8Q';
const approvalId = 'netluHiPQf-UoheEIBfbLw';
const decisionId = 'hq411Ypqo7V0LflO-dcWIg';
const paths = new SvelteMap<Home, Path | undefined>();
const tab = new SvelteMap<'other', boolean>();

vi.mock('../src/lib/net.svelte', async (actual) => {
  const real = await actual<typeof import('../src/lib/net.svelte')>();
  return {
    ...real,
    start: (source: Source) => ({
      net: {
        source,
        homes,
        path: (home: Home) => (home.ctl ? paths.get(home) : undefined),
        get otherTab() {
          return tab.get('other') ?? false;
        },
        useHere: () => tab.set('other', false),
      },
      stop: () => {},
    }),
  };
});

class QuietSocket {
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  send() {}
  close() {}
}

/** The text of the elements that `aria-labelledby` or `aria-describedby` of an element names. */
const pointed = (el: Element, attr: 'aria-labelledby' | 'aria-describedby') =>
  (el.getAttribute(attr) ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => read(document.getElementById(id)!))
    .join(' ');
const described = (el: Element) => pointed(el, 'aria-describedby');
/** What names an element: the parts it points at, its `aria-label`, its label, or its text. */
const named = (el: Element) =>
  pointed(el, 'aria-labelledby') ||
  el.getAttribute('aria-label') ||
  read((el as HTMLInputElement).labels?.[0] ?? el);

const buttonNamed = (root: ParentNode, label: string) =>
  [...root.querySelectorAll<HTMLButtonElement>('button')].find((btn) =>
    named(btn).includes(label),
  )!;

type Rgb = [number, number, number];

function parseHex(hex: string): Rgb {
  const raw = hex.trim().slice(1);
  return [0, 2, 4].map((idx) => Number.parseInt(raw.slice(idx, idx + 2), 16) / 255) as Rgb;
}

const mixRgb = (fg: Rgb, weight: number, bg: Rgb): Rgb =>
  fg.map((ch, idx) => ch * weight + bg[idx] * (1 - weight)) as Rgb;

const channelLum = (v: number) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);

function contrast(a: Rgb, b: Rgb): number {
  const lumA = 0.2126 * channelLum(a[0]) + 0.7152 * channelLum(a[1]) + 0.0722 * channelLum(a[2]);
  const lumB = 0.2126 * channelLum(b[0]) + 0.7152 * channelLum(b[1]) + 0.0722 * channelLum(b[2]);
  return (Math.max(lumA, lumB) + 0.05) / (Math.min(lumA, lumB) + 0.05);
}

function parsePalette(css: string, mode: 'light' | 'dark'): Record<string, Rgb> {
  const palette: Record<string, Rgb> = {};
  for (const match of css.matchAll(
    /--([\w-]+):\s*light-dark\((#[0-9a-f]{6}),\s*(#[0-9a-f]{6})\)/gi,
  )) {
    palette[match[1]] = parseHex(mode === 'light' ? match[2] : match[3]);
  }
  for (const match of css.matchAll(/--([\w-]+):\s*(#[0-9a-f]{6})\s*;/gi)) {
    palette[match[1]] = parseHex(match[2]);
  }
  for (const match of css.matchAll(
    /--([\w-]+):\s*color-mix\(in srgb,\s*var\(--([\w-]+)\)\s*(\d+)%,\s*var\(--([\w-]+)\)\)/gi,
  )) {
    palette[match[1]] = mixRgb(palette[match[2]], Number(match[3]) / 100, palette[match[4]]);
  }
  return palette;
}

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let home: ActHome;
  let reader: ReturnType<typeof browser>;

  async function syncHome() {
    const expected = home.synced + home.groups.size;
    await reader.connect();
    await vi.waitFor(() => expect(home.synced).toBe(expected));
    await new Promise((later) => setTimeout(later));
    flushSync();
  }

  async function seedGroups() {
    home = new ActHome('workstation');
    home.add(website, 'Website');
    home.add(docs, 'Docs');
    home.write(
      website,
      example('row-member-ai', {
        id: secretary,
        name: 'Secretary',
        cli: 'claude',
        secretary: true,
      }),
    );
    home.write(website, example('row-member-ai', { id: codex, name: 'Codex', cli: 'codex' }));
    home.write(
      website,
      example('row-member-ai', { id: claude, name: 'Claude Code', cli: 'claude' }),
    );
    home.write(
      website,
      example('row-member-human', { id: 'mina-m', user: 'mina-u', name: 'Mina' }),
    );
    home.write(website, example('row-task'));
    home.write(website, example('row-msg-task', { id: 'msg-task-12', body: { task: taskId } }));
    home.write(website, example('row-msg-brief-pending', { id: briefId, by: secretary }));
    home.write(website, example('row-msg-approval-pending', { id: approvalId }));
    home.write(
      website,
      example('row-msg-approval-pending', {
        id: 'ap-mina',
        body: {
          ...(example('row-msg-approval-pending').body as Record<string, unknown>),
          approvers: ['mina-u'],
        },
      }),
    );
    home.write(
      website,
      example('row-msg-decision-pending', {
        id: decisionId,
        body: {
          task: taskId,
          question: 'Where should the path appear?',
          options: [
            { id: 'A', label: 'Top banner', effect: 'Always visible.' },
            { id: 'B', label: 'Connection indicator', effect: 'Quiet.' },
          ],
          recommended: 'B',
          to: [user],
        },
      }),
    );
    home.write(website, example('row-msg-summary', { id: 'sum-1', text: 'Merged cleanly.' }));
    home.say(docs, owner, 'Docs ready.');
    reader = browser(home);
    homes.push(reader.store);
    paths.set(reader.store, { kind: 'direct', rtt: 12, since: Date.now() });
    await syncHome();
  }

  AfterEachScenario(async () => {
    vi.useRealTimers();
    prefs.lang = 'en';
    prefs.theme = 'auto';
    await closePage();
    paths.clear();
    tab.clear();
    nodes.infos.clear();
    localStorage.clear();
  });

  Background(({ Given }) => {
    Given('a browser set to English', () => {
      prefs.lang = 'en';
    });
  });

  Scenario(
    'Onboarding and settings expose landmarks, live announcements, names and language marks',
    ({ Given, When, Then, And }) => {
      Given('a fresh pairing browser on a 390 px phone', () => {
        vi.useFakeTimers();
        FakePC.reset();
        vi.stubGlobal('indexedDB', new IDBFactory());
        vi.stubGlobal('WebSocket', QuietSocket);
        vi.stubGlobal('RTCPeerConnection', FakePC);
        Object.defineProperty(navigator, 'clipboard', {
          configurable: true,
          value: { writeText: vi.fn().mockResolvedValue(undefined) },
        });
      });
      When('the person opens the client at "#/onboarding"', () => {
        openPage('#/onboarding', 390);
      });
      Then(
        'the live status region stands outside the hidden top bar and announces theme, language and copy changes',
        async () => {
          expect(one('.app').dataset.full).toBe('always');
          const live = one('.app > [role="status"]');
          expect(live.closest('.bar')).toBeNull();
          buttonNamed(one('.onboarding .top'), 'Toggle theme').click();
          flushSync();
          expect(live.textContent).toBe('Theme: Dark');
          buttonNamed(one('.onboarding .top'), '中').click();
          flushSync();
          expect(live.textContent).toBe('语言: 简体中文');
          buttonNamed(one('.onboarding .top'), 'EN').click();
          flushSync();
          one<HTMLButtonElement>('.prompt header button').click();
          await vi.advanceTimersByTimeAsync(0);
          flushSync();
          expect(live.textContent).toBe('Copied');
        },
      );
      And(
        'the pairing prompt region is focusable and named "Prompt for your AI", and the pairing steps keep their list role inside a polite status region',
        () => {
          const prompt = one('#prompt');
          expect(prompt.tabIndex).toBe(0);
          expect(prompt.getAttribute('role')).toBe('region');
          expect(named(prompt)).toBe('Prompt for your AI');
          const steps = one('ol.link-steps');
          expect(steps.getAttribute('role')).toBeNull();
          expect(steps.parentElement?.getAttribute('role')).toBe('status');
          expect(steps.parentElement?.getAttribute('aria-live')).toBe('polite');
          expect(steps.querySelectorAll('li')).toHaveLength(4);
        },
      );
      And(
        'every control on onboarding has an accessible name, and renewing an expired code keeps focus on the prompt action button',
        async () => {
          for (const el of all('.onboarding button, .onboarding a')) {
            expect(named(el).length).toBeGreaterThan(0);
          }
          const langBtn = all<HTMLButtonElement>('.onboarding .top button')[0];
          expect(named(langBtn).startsWith('中')).toBe(true);
          expect(
            [...langBtn.querySelectorAll<HTMLElement>('[lang]')].map((part) => part.lang),
          ).toEqual(['zh-CN', 'en', 'zh-CN']);

          const actionBtn = one<HTMLButtonElement>('.prompt header button');
          actionBtn.focus();
          await vi.advanceTimersByTimeAsync(600_000);
          flushSync();
          expect(named(actionBtn)).toBe('New pairing code');
          expect(document.activeElement).toBe(actionBtn);
          actionBtn.click();
          flushSync();
          expect(named(actionBtn)).toBe('Copy prompt');
          expect(document.activeElement).toBe(actionBtn);
        },
      );
      When('the person opens "#/settings" on a 1440 px desktop', async () => {
        vi.useRealTimers();
        await closePage();
        openPage('#/onboarding', 1440);
        go('#/settings');
      });
      Then(
        'focus moves to the main landmark named "Settings", the skip button returns focus there, and each language choice is marked in its own language',
        () => {
          const main = one('main');
          expect(document.activeElement).toBe(main);
          expect(named(main)).toBe('Settings');
          const skip = one<HTMLButtonElement>('button.skip');
          skip.focus();
          skip.click();
          expect(document.activeElement).toBe(main);
          expect(texts('fieldset legend')).toEqual(['Theme', 'Language']);
          const langMarks = all('input[name="lang"]').map((input) =>
            input.closest('[lang]')?.getAttribute('lang'),
          );
          expect(langMarks).toEqual(['zh-CN', 'en']);
        },
      );
      And(
        'the reminder switch is named "Browser notifications" and its choices form a group named "Notify me when"',
        () => {
          expect(named(one('[role="switch"]'))).toBe('Browser notifications');
          const choices = one('[aria-labelledby="s-noti"] [role="group"]');
          expect(named(choices)).toBe('Notify me when');
          expect(choices.querySelectorAll('input[type="checkbox"]')).toHaveLength(6);
        },
      );
    },
  );

  Scenario(
    'Group list, waiting digest, create-group dialog, chat and cards name every control and link descriptions',
    ({ Given, When, Then }) => {
      Given(
        'two groups "Website" and "Docs" with waiting brief, approval and decision cards, a locked approval, a task and a summary',
        seedGroups,
      );
      When('the person opens "#/groups" on a 1280 px desktop', () => {
        openPage('#/groups', 1280);
      });
      Then(
        'the group list and waiting digest expose named landmarks, and each digest action starts with "Open" and names its card and group',
        () => {
          expect(named(one('section.list'))).toBe('Group list');
          expect(named(one('section.digest'))).toBe('Waiting on you');
          const openLinks = all('.digest .todo a');
          expect(openLinks).toHaveLength(3);
          expect(openLinks.map((link) => named(link))).toEqual([
            'Open · Confirm start · Show connection paths · Website',
            'Open · Approval · Time the path change in a browser. · Website',
            'Open · Your call · Where should the path appear? · Website',
          ]);
          for (const link of openLinks) expect(read(link)).toBe('Open');
        },
      );
      When('the person opens "New group"', () => {
        buttonNamed(one('section.list'), 'New group').click();
        flushSync();
      });
      Then(
        'the dialog is named "New group" and every field is named by its label without concatenating its hint',
        () => {
          const dlg = one<HTMLDialogElement>('dialog');
          expect(dlg.open).toBe(true);
          expect(named(dlg)).toBe('New group');
          const controls = [...dlg.querySelectorAll('input, select, textarea')];
          expect(controls.map((el) => named(el))).toEqual([
            'Project group',
            'Team group',
            'Group name',
            'Git repository',
            'Base branch',
            'Your name in the group',
            'Pull requests and hosted checks',
            'Check commands',
            'Home computer',
          ]);
          expect(described(controls[0])).toBe('AIs work on one repository.');
          expect(described(controls[1])).toBe('Notices and talk. No tasks.');
          expect(described(controls[3])).toContain('clone the project from');
          expect(described(controls[7])).toContain('One per line');
        },
      );
      When('the person opens the chat of "Website" on a 1280 px desktop', () => {
        go(`#/groups/${website}`);
      });
      Then(
        'the chat, main line, waiting bar, composer and every card expose localized accessible names and linked status descriptions in English and Chinese',
        () => {
          expect(named(one('section.chat'))).toBe('Website');
          expect(named(one('.scroller'))).toBe('Main line');
          expect(named(one('nav.needbar'))).toBe('Waiting on you');
          expect(named(field())).toBe('Message');
          for (const btn of all('.chat button, .chat a')) {
            expect(named(btn).length).toBeGreaterThan(0);
          }

          type('a'.repeat(9000));
          expect(field().getAttribute('aria-invalid')).toBe('true');
          expect(described(field())).toContain('Too long to send');
          type('');
          expect(field().getAttribute('aria-invalid')).toBeNull();

          expect(named(one(`#m-${briefId} section.card`))).toBe(
            'Task brief · Show connection paths',
          );
          expect(named(one(`#m-${approvalId} section.card`))).toBe('Approval');
          expect(described(one(`#m-${approvalId} .btn-dawn`))).toBe('Applies to this one request');
          expect(described(one('#m-ap-mina .card-foot button'))).toBe(
            "Only the computer's owner Mina can approve",
          );
          expect(named(one(`#m-${decisionId} section.card`))).toBe('Your call');
          const decInput = one<HTMLInputElement>(`#m-${decisionId} input`);
          fill(decInput, 'z'.repeat(1023));
          flushSync();
          expect(decInput.getAttribute('aria-invalid')).toBe('true');
          expect(described(decInput)).toBe('Too long: shorten your answer to fit 1 KB.');
          fill(decInput, '');
          flushSync();
          expect(named(one('#m-msg-task-12 section.card'))).toBe('T-12 Show connection path');
          expect(named(one('#m-sum-1 section.card'))).toBe('Done · T-12 · Show connection path');

          prefs.lang = 'zh-CN';
          flushSync();
          expect(named(one('.scroller'))).toBe('主线');
          expect(named(one(`#m-${briefId} section.card`))).toBe('任务说明 · Show connection paths');
          expect(named(one('#m-sum-1 section.card'))).toBe(
            '完成总结 · T-12 · Show connection path',
          );
        },
      );
    },
  );

  Scenario(
    'Acting on cards, threads and computer accounts never drops focus to the document body',
    ({ Given, And, When, Then }) => {
      Given(
        'two groups "Website" and "Docs" with waiting brief, approval and decision cards, a locked approval, a task and a summary',
        seedGroups,
      );
      And(
        'the chat of "Website" is open with 60 earlier progress messages on task 12',
        async () => {
          reader.drop();
          homes.length = 0;
          for (let i = 1; i <= 60; i++) {
            home.write(
              website,
              example('row-msg-progress', {
                id: `p-${i}`,
                thread: taskId,
                by: codex,
                text: `progress ${i}`,
              }),
            );
          }
          reader = browser(home);
          homes.push(reader.store);
          await syncHome();
          openPage(`#/groups/${website}`, 1280);
        },
      );
      When(
        'the person opens "Change it" on the brief card, presses Escape, reopens, cancels, and then confirms the brief',
        async () => {
          const briefCard = one(`#m-${briefId} section.card`);
          const change = buttonNamed(briefCard, 'Change it');
          change.focus();
          change.click();
          flushSync();
          const area = briefCard.querySelector<HTMLTextAreaElement>('textarea')!;
          expect(document.activeElement).toBe(area);

          const esc = new KeyboardEvent('keydown', {
            key: 'Escape',
            bubbles: true,
            cancelable: true,
          });
          area.dispatchEvent(esc);
          flushSync();
          expect(esc.defaultPrevented).toBe(true);
          expect(document.activeElement).toBe(buttonNamed(briefCard, 'Change it'));

          const reopen = buttonNamed(briefCard, 'Change it');
          reopen.focus();
          reopen.click();
          flushSync();
          buttonNamed(briefCard, 'Cancel').click();
          flushSync();
          expect(document.activeElement).toBe(buttonNamed(briefCard, 'Change it'));

          const confirm = buttonNamed(briefCard, 'Confirm & start');
          confirm.focus();
          confirm.click();
          await vi.waitFor(() => expect(briefCard.querySelector('summary')).not.toBeNull());
          await tick();
          await tick();
          flushSync();
        },
      );
      Then(
        'focus moves to the note field, returns to "Change it" on Escape and Cancel while marking Escape handled, and lands on the folded brief summary on confirm',
        () => {
          expect(document.activeElement).toBe(one(`#m-${briefId} summary`));
          expect(document.activeElement).not.toBe(document.body);
        },
      );
      When(
        'the person approves the approval card and chooses an option on the decision card from the keyboard',
        async () => {
          const apCard = one(`#m-${approvalId} section.card`);
          home.refuse = 'conflict';
          const approveBtn = buttonNamed(apCard, 'Approve');
          approveBtn.focus();
          approveBtn.click();
          await vi.waitFor(() => expect(apCard.querySelector('[role="alert"]')).not.toBeNull());
          await tick();
          flushSync();
          expect(document.activeElement).toBe(buttonNamed(apCard, 'Approve'));

          home.refuse = undefined;
          home.hold = true;
          buttonNamed(apCard, 'Approve').click();
          flushSync();
          expect(document.activeElement).toBe(apCard);
          home.release?.();
          home.hold = false;
          await vi.waitFor(() => expect(apCard.querySelector('.card-foot')).toBeNull());
          await tick();
          flushSync();
          expect(document.activeElement).toBe(apCard);

          const decCard = one(`#m-${decisionId} section.card`);
          const optA = buttonNamed(decCard, 'A · Top banner');
          optA.focus();
          optA.click();
          await vi.waitFor(() => expect(decCard.querySelector('.opt.chosen')).not.toBeNull());
          await tick();
          await tick();
          flushSync();
          expect(document.activeElement).toBe(decCard);
        },
      );
      Then(
        'focus stays on each card instead of falling to the document body, and returns to the action button when the home refuses an action',
        () => {
          expect(document.activeElement).toBe(one(`#m-${decisionId} section.card`));
          expect(document.activeElement).not.toBe(document.body);
        },
      );
      When(
        'the person scrolls back, activates "Back to the latest", expands task 12\'s thread and loads the remaining earlier messages',
        async () => {
          const scroller = one('.scroller');
          scroller.scrollTo = ((opts?: ScrollToOptions) => {
            scroller.scrollTop = opts?.top ?? 0;
            scroller.dispatchEvent(new Event('scroll'));
          }) as typeof scroller.scrollTo;
          scroller.scrollTop = -400;
          scroller.dispatchEvent(new Event('scroll'));
          flushSync();
          const latest = one<HTMLButtonElement>('.latest');
          latest.focus();
          latest.click();
          flushSync();
          expect(document.activeElement).toBe(scroller);

          const fold = one<HTMLDetailsElement>('#m-msg-task-12 details.fold');
          fold.open = true;
          flushSync();
          await vi.waitFor(() => {
            flushSync();
            const btn = document.querySelector<HTMLButtonElement>('#m-msg-task-12 .tpage button');
            expect(btn?.disabled).toBe(false);
          });
          const olderBtn = one<HTMLButtonElement>('#m-msg-task-12 .tpage button');
          olderBtn.focus();
          olderBtn.click();
          await vi.waitFor(() => {
            flushSync();
            expect(all('#m-msg-task-12 .tpage')).toEqual([]);
          });
          await tick();
          await tick();
          flushSync();
        },
      );
      Then(
        '"Back to the latest" returns focus to the main line and finishing thread paging moves focus to the task\'s thread summary',
        () => {
          expect(document.activeElement).toBe(one('#m-msg-task-12 details.fold > summary'));
          expect(document.activeElement).not.toBe(document.body);
        },
      );
      When(
        'the person opens "#/nodes", declares an AI account type from the keyboard, copies the link of "Add a browser", closes the dialog and opens it again',
        async () => {
          await closePage();
          const rig = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q');
          rig.home.add(website, 'Website');
          homes.push(rig.store);
          await rig.connect();
          paths.set(rig.store, { kind: 'direct', rtt: 5, since: Date.now() });
          Object.defineProperty(navigator, 'clipboard', {
            configurable: true,
            value: { writeText: vi.fn().mockResolvedValue(undefined) },
          });
          openPage('#/nodes', 1280);
          await vi.waitFor(() => {
            flushSync();
            expect(all('.card.node input[value="team"]')).toHaveLength(1);
          });

          const radio = one<HTMLInputElement>('.card.node input[value="team"]');
          radio.focus();
          radio.checked = true;
          radio.dispatchEvent(new Event('change', { bubbles: true }));
          await vi.waitFor(() => {
            flushSync();
            expect(document.activeElement).toBe(radio);
          });
          expect(radio.disabled || radio.closest('fieldset')!.disabled).toBe(false);
          expect(radio.checked).toBe(true);

          buttonNamed(one('.page-head'), 'Add a browser').click();
          flushSync();
          one('dialog form').dispatchEvent(
            new Event('submit', { bubbles: true, cancelable: true }),
          );
          await vi.waitFor(() => {
            flushSync();
            expect(all('dialog .pair-box button')).toHaveLength(1);
          });
          one<HTMLButtonElement>('dialog .pair-box button').click();
          await tick();
          flushSync();
          one<HTMLButtonElement>('dialog .btn-primary').click();
          flushSync();
          expect(one<HTMLDialogElement>('dialog').open).toBe(false);
          buttonNamed(one('.page-head'), 'Add a browser').click();
          flushSync();
        },
      );
      Then(
        'choosing the account keeps focus on the choice, copying the pairing link announces "Copied", and the dialog is open again',
        () => {
          expect(text('.app > [role="status"]')).toBe('Copied');
          expect(one<HTMLDialogElement>('dialog').open).toBe(true);
        },
      );
    },
  );

  Scenario(
    "Usage bars, calls, project memory and notifications name their parts and keep the keyboard's place",
    ({ Given, And, When, Then }) => {
      const closers = () => all<HTMLButtonElement>('.toast:not([inert]) .icon-btn');
      const mention = (id: string) =>
        home.write(
          website,
          example('row-msg-text', {
            id,
            thread: taskId,
            by: codex,
            mentions: [owner],
            ts: Date.now(),
          }),
        );

      Given(
        'two groups "Website" and "Docs" with waiting brief, approval and decision cards, a locked approval, a task and a summary',
        seedGroups,
      );
      And(
        'a waiting call in task 12\'s thread and a note, with the chat of "Website" open on a 1280 px desktop',
        async () => {
          home.write(
            website,
            example('row-member-ai', {
              id: claude,
              name: 'Claude Code',
              usage: { window_pct: 100 },
            }),
          );
          home.write(website, example('row-msg-call-waiting', { id: 'call-1', ts: Date.now() }));
          home.write(website, example('row-note'));
          openPage(`#/groups/${website}`, 1280);
          one<HTMLDetailsElement>('#m-msg-task-12 details.fold').open = true;
          const memory = one<HTMLDetailsElement>('.groupinfo details.fold');
          memory.open = true;
          memory.dispatchEvent(new Event('toggle'));
          await vi.waitFor(() => {
            flushSync();
            expect(all('.thread .call')).toHaveLength(1);
          });
        },
      );
      Then(
        'each usage bar is a meter named "Window used" whose value text is its percentage, with its reset time when one is known',
        () => {
          const meters = all('.members [role="meter"]').map((meter) => [
            named(meter),
            meter.getAttribute('aria-valuenow'),
            meter.getAttribute('aria-valuetext'),
          ]);
          const resets = expect.stringMatching(/^63% · Resets \d\d:\d\d$/);
          expect(meters).toEqual([
            ['Window used', '63', resets],
            ['Window used', '100', '100%'],
            ['Window used', '63', resets],
          ]);
        },
      );
      And(
        'the call\'s waiting line stands in no live region and keeps "Depth 1" in one piece',
        () => {
          expect(text('.thread .call .depth')).toBe('Depth 1');
          expect(one('.thread .call').closest('[aria-live], [role="status"]')).toBeNull();
        },
      );
      And('each delete action of the project memory names its entry', () => {
        expect(all('.groupinfo .del').map(named)).toEqual([
          'Delete note: Path events include the connection id.',
        ]);
      });
      When("a decision and two mentions arrive in task 12's thread", () => {
        home.write(
          website,
          example('row-msg-decision-pending', { id: 'dec-2', thread: taskId, main: false }),
        );
        mention('men-1');
        mention('men-2');
        flushSync();
      });
      Then(
        'the notifications hang under the chat\'s head in a polite region of their own, and "Open" and "Dismiss notification" are described by their notification',
        () => {
          const stack = one('.notify');
          expect(stack.classList.contains('chat')).toBe(true);
          expect(stack.getAttribute('aria-live')).toBe('polite');
          expect(stack.contains(one('.app > [role="status"]'))).toBe(false);
          const title = 'Your call · Where should the path appear?';
          expect(texts('.toast:not([inert]) b')[0]).toBe(title);
          expect(described(one('.toast a'))).toBe(title);
          expect(named(closers()[0])).toBe('Dismiss notification');
          expect(described(closers()[0])).toBe(title);
        },
      );
      const landed: number[] = [];
      When(
        'the person closes the second notification from the keyboard, then the last, then the one that is left',
        () => {
          for (const at of [1, 1, 0]) {
            const closer = closers()[at];
            closer.focus();
            closer.click();
            flushSync();
            landed.push(closers().indexOf(document.activeElement as HTMLButtonElement));
          }
        },
      );
      Then(
        'focus moves to the next close button, then to the one before, then to the main landmark',
        () => {
          expect(landed).toEqual([1, 0, -1]);
          expect(document.activeElement).toBe(one('main'));
        },
      );
      When('two more mentions arrive and the person closes the first with the pointer', () => {
        mention('men-3');
        mention('men-4');
        flushSync();
        closers()[0].click();
        flushSync();
      });
      Then('focus stays on the main landmark', () => {
        expect(closers()).toHaveLength(1);
        expect(document.activeElement).toBe(one('main'));
      });
    },
  );

  Scenario(
    'The other-tab notice is named and can be left from the keyboard',
    ({ Given, When, Then }) => {
      Given(
        'two groups "Website" and "Docs" with waiting brief, approval and decision cards, a locked approval, a task and a summary',
        seedGroups,
      );
      When(
        'the person opens the group list on a 1280 px desktop and another tab takes the connection',
        () => {
          openPage('#/groups', 1280);
          tab.set('other', true);
          flushSync();
        },
      );
      Then(
        'the notice is a status region, the connection indicator is named "Connection status: inactive", and no notification hangs under a chat\'s head',
        () => {
          expect(one('.tab-ban').getAttribute('role')).toBe('status');
          expect(named(one('.bar .conn'))).toBe('Connection status: inactive');
          expect(one('.notify').classList.contains('chat')).toBe(false);
        },
      );
      When('the person chooses "Use here"', () => {
        const use = buttonNamed(one('.tab-ban'), 'Use here');
        use.focus();
        use.click();
        flushSync();
      });
      Then('focus moves to the main landmark', () => {
        expect(all('.tab-ban')).toEqual([]);
        expect(document.activeElement).toBe(one('main'));
      });
    },
  );

  Scenario(
    'Phone, tablet and desktop widths adapt whole-screen mode, connection indicator placement and side panel reachability',
    ({ Given, When, Then, And }) => {
      interface WidthSnap {
        full: string | undefined;
        barConn: number;
        headConn: string[];
        openBehindInert: boolean[];
        afterEscFocus: string | null;
      }
      const snaps = new Map<number, WidthSnap>();

      Given('a group "Website" with a side panel area and an active home connection', async () => {
        const rig = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q');
        rig.home.add(website, 'Website');
        homes.push(rig.store);
        await rig.connect();
        paths.set(rig.store, { kind: 'direct', rtt: 14, since: Date.now() });
        await vi.waitFor(() => expect(find(website)?.info).toBeDefined());
      });
      When(
        'the person opens the chat of "Website" at 390, 820, 1024 and 1440 px widths',
        async () => {
          const saved = [...homes];
          for (const width of [390, 820, 1024, 1440]) {
            await closePage();
            homes.push(...saved);
            openPage(`#/groups/${website}`, width);
            one<HTMLButtonElement>('.side-open').click();
            await tick();
            flushSync();
            const openBehindInert = [...all('.chat > :not(.side)'), one('.notify')].map((el) =>
              el.hasAttribute('inert'),
            );
            window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
            flushSync();
            await tick();
            await tick();
            flushSync();
            snaps.set(width, {
              full: one('.app').dataset.full,
              barConn: all('.bar .conn').length,
              headConn: all('.chat-head .conn').map((el) => named(el)),
              openBehindInert,
              afterEscFocus: document.activeElement?.getAttribute('aria-label') ?? null,
            });
          }
        },
      );
      Then(
        'on a 390 px phone the shell uses phone whole-screen mode and places the named connection indicator in the chat head',
        () => {
          const phone = snaps.get(390)!;
          expect(phone.full).toBe('phone');
          expect(phone.barConn).toBe(0);
          expect(phone.headConn).toEqual(['Connection status: direct, 14 ms']);
        },
      );
      And(
        'at 390 and 820 px the open side panel makes the parts behind it and the notifications inert and Escape returns focus to "People and details"',
        () => {
          for (const width of [390, 820]) {
            const snap = snaps.get(width)!;
            expect(snap.openBehindInert).toEqual([true, true, true, true, true]);
            expect(snap.afterEscFocus).toBe('People and details');
          }
          expect(snaps.get(820)!.barConn).toBe(1);
          expect(snaps.get(820)!.headConn).toEqual([]);
        },
      );
      And(
        'at 1024 and 1440 px the side panel stands beside the main line and nothing is inert, the notifications included',
        () => {
          for (const width of [1024, 1440]) {
            expect(snaps.get(width)!.openBehindInert).toEqual([false, false, false, false, false]);
          }
        },
      );
    },
  );

  Scenario(
    'Design tokens meet WCAG 2.1 AA contrast in both themes and reduced motion stills scripted scrolling and transitions',
    ({ When, Then }) => {
      let light: Record<string, Rgb>;
      let dark: Record<string, Rgb>;

      When(
        'the colour tokens of "src/styles/tokens.css" are evaluated in light and dark themes',
        () => {
          const css = readFileSync(join(process.cwd(), 'src/styles/tokens.css'), 'utf8');
          light = parsePalette(css, 'light');
          dark = parsePalette(css, 'dark');
        },
      );
      Then(
        'every text and badge colour pair meets at least 4.5 to 1 contrast and every control border and focus ring meets at least 3 to 1 contrast',
        () => {
          const textPairs: [string, string][] = [
            ['text', 'bg'],
            ['text', 'surface'],
            ['text', 'surface-2'],
            ['text-2', 'bg'],
            ['text-2', 'surface'],
            ['text-2', 'surface-2'],
            ['text-3', 'bg'],
            ['text-3', 'surface'],
            ['text-3', 'surface-2'],
            ['sky', 'surface'],
            ['sky', 'sky-soft'],
            ['dawn-text', 'surface'],
            ['dawn-text', 'dawn-soft'],
            ['ok', 'surface'],
            ['ok', 'ok-soft'],
            ['danger', 'surface'],
            ['danger', 'danger-soft'],
            ['on-sky', 'sky'],
            ['on-dawn', 'dawn'],
            ['on-ok', 'ok'],
            ['on-danger', 'danger'],
            ['term-fg', 'term-bg'],
            ['term-dim', 'term-bg'],
            ['term-green', 'term-bg'],
            ['term-yellow', 'term-bg'],
            ['term-blue', 'term-bg'],
            ['term-red', 'term-bg'],
          ];
          const uiPairs: [string, string][] = [
            ['line-strong', 'bg'],
            ['line-strong', 'surface'],
            ['sky', 'bg'],
            ['sky', 'surface'],
          ];
          for (const [mode, palette] of [
            ['light', light],
            ['dark', dark],
          ] as const) {
            for (const [fg, bg] of textPairs) {
              expect(
                contrast(palette[fg], palette[bg]),
                `${mode} ${fg} on ${bg}`,
              ).toBeGreaterThanOrEqual(4.5);
            }
            for (const [fg, bg] of uiPairs) {
              expect(
                contrast(palette[fg], palette[bg]),
                `${mode} ${fg} on ${bg}`,
              ).toBeGreaterThanOrEqual(3);
            }
          }
        },
      );
      When('the system asks for reduced motion', () => {
        vi.resetModules();
        vi.stubGlobal('matchMedia', (query: string) => ({
          matches: query.includes('prefers-reduced-motion: reduce') || query.includes('min-width'),
          addEventListener() {},
          removeEventListener() {},
        }));
      });
      Then(
        'jumping to a message, returning to the latest message and switching preferences run without smooth scrolling or view transitions, and the stylesheet keeps a single reduced-motion rule',
        async () => {
          const freshSvelte = await import('svelte');
          const { jumpToMessage: jump } = await import('../src/lib/jump');
          const { shift: runShift } = await import('../src/lib/motion');
          const { default: Chat } = await import('../src/components/chat/Chat.svelte');
          await seedGroups();
          vi.stubGlobal(
            'IntersectionObserver',
            class {
              observe() {}
              disconnect() {}
            },
          );
          const mounted = freshSvelte.mount(Chat, {
            target: document.body,
            props: {
              group: reader.store.groups[0],
              net: {
                source: 'mock',
                homes,
                path: (home: Home) => (home.ctl ? paths.get(home) : undefined),
              },
            },
          });
          freshSvelte.flushSync();

          const msgEl = one(`#m-${approvalId}`);
          let jumpBehavior: ScrollBehavior | undefined;
          msgEl.scrollIntoView = (opts) => {
            jumpBehavior = (opts as ScrollIntoViewOptions)?.behavior;
          };
          jump(new MouseEvent('click', { cancelable: true }), approvalId, 'approval');
          expect(jumpBehavior).toBe('auto');

          const scroller = one('.scroller');
          let scrollBehavior: ScrollBehavior | undefined;
          scroller.scrollTo = ((opts?: ScrollToOptions) => {
            scrollBehavior = opts?.behavior;
            scroller.scrollTop = 0;
            scroller.dispatchEvent(new Event('scroll'));
          }) as typeof scroller.scrollTo;
          scroller.scrollTop = -360;
          scroller.dispatchEvent(new Event('scroll'));
          freshSvelte.flushSync();
          one<HTMLButtonElement>('.latest').click();
          expect(scrollBehavior).toBe('auto');

          const startTransition = vi.fn();
          Object.defineProperty(document, 'startViewTransition', {
            configurable: true,
            value: startTransition,
          });
          let ran = false;
          runShift(() => {
            ran = true;
          });
          expect(ran).toBe(true);
          expect(startTransition).not.toHaveBeenCalled();
          Reflect.deleteProperty(document, 'startViewTransition');
          await freshSvelte.unmount(mounted);

          const baseCss = readFileSync(join(process.cwd(), 'src/styles/base.css'), 'utf8');
          expect(baseCss.match(/@media\s*\(prefers-reduced-motion:\s*reduce\)/g)).toHaveLength(1);
        },
      );
    },
  );
});
