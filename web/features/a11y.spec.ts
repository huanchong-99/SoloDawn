import 'fake-indexeddb/auto';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { IDBFactory } from 'fake-indexeddb';
import { flushSync, tick } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { expect, vi } from 'vitest';
import { find, Home, homes } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import type { Path, Source } from '../src/lib/net.svelte';
import { nodes } from '../src/lib/nodes.svelte';
import type { AccountKind } from '../src/lib/onboard.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import * as p from '../src/lib/proto';
import { browser, example, owner, TestHome, user } from './home';
import { all, closePage, field, fill, go, one, openPage, texts, type } from './page';
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

vi.mock('../src/lib/net.svelte', async (actual) => {
  const real = await actual<typeof import('../src/lib/net.svelte')>();
  return {
    ...real,
    start: (source: Source) => ({
      net: {
        source,
        homes,
        path: (home: Home) => (home.ctl ? paths.get(home) : undefined),
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

class A11yHome extends TestHome {
  holdAct = false;
  releaseAct: (() => void) | undefined;

  override connect(push: (frame: p.Push) => void) {
    const conn = super.connect(push);
    const base = conn.ctl.request as (req: Record<string, unknown>) => Promise<unknown>;
    const ctl = {
      request: (req: Record<string, unknown>) => this.#request(base, req),
    } as p.Ctl;
    return { ctl, close: conn.close };
  }

  async #request(
    base: (req: Record<string, unknown>) => Promise<unknown>,
    req: Record<string, unknown>,
  ) {
    if (req.t !== 'cmd') return base(req);
    const env = req.env as p.Envelope;
    const raw = JSON.parse(env.p) as Record<string, unknown>;
    if (raw.t !== 'act') return base(req);
    if (this.holdAct) await new Promise<void>((resolve) => (this.releaseAct = resolve));
    else await new Promise((later) => setTimeout(later));
    this.asked.push(req);
    const re = this.asked.length;
    if (this.refuse) return { re, err: { code: this.refuse, msg: 'refused' } };
    return { re, ...this.#applyAct(env, p.decode(p.Act, env.p)) };
  }

  #applyAct(env: p.Envelope, act: p.Act) {
    const current = this.groups.get(act.g)?.rows.get(`msg ${act.msg}`) as
      (p.MsgRow & Record<string, unknown>) | undefined;
    if (!current) return { err: { code: 'not_found', msg: 'missing' } };
    const state = stateForAct(env, act);
    const updated = p.decode(p.MsgRow, JSON.stringify({ ...current, state }));
    this.write(act.g, updated as p.Row & Record<string, unknown>);
    return { ok: true };
  }
}

function stateForAct(env: p.Envelope, act: p.Act): Record<string, unknown> {
  const base = { by: owner, at: act.ts, act: env };
  const map: Record<string, Record<string, unknown>> = {
    confirm: { status: 'confirmed', ...base },
    revise: { status: 'revise', ...base, ...(act.text ? { note: act.text } : {}) },
    approve: { status: 'approved', ...base },
    deny: { status: 'denied', ...base },
    choose: {
      status: 'decided',
      ...base,
      ...(act.arg ? { choice: act.arg } : {}),
      ...(act.text ? { text: act.text } : {}),
    },
  };
  return map[act.action];
}

const cleanText = (value: string | null | undefined) => (value ?? '').replace(/\s+/g, ' ').trim();

function labelledText(el: Element): string {
  const ids = el.getAttribute('aria-labelledby');
  if (!ids) return '';
  return cleanText(
    ids
      .split(/\s+/)
      .map((id) => document.getElementById(id)?.textContent ?? '')
      .join(' '),
  );
}

function accessibleName(el: Element): string {
  const byId = labelledText(el);
  if (byId) return byId;
  const direct = cleanText(el.getAttribute('aria-label'));
  if (direct) return direct;
  if ('labels' in el && (el as HTMLInputElement).labels?.length) {
    return cleanText(
      [...(el as HTMLInputElement).labels!].map((label) => label.textContent).join(' '),
    );
  }
  return cleanText(el.textContent);
}

function describedText(el: Element): string {
  const ids = el.getAttribute('aria-describedby');
  if (!ids) return '';
  return cleanText(
    ids
      .split(/\s+/)
      .map((id) => document.getElementById(id)?.textContent ?? '')
      .join(' '),
  );
}

const buttonNamed = (root: ParentNode, label: string) =>
  [...root.querySelectorAll<HTMLButtonElement>('button')].find((btn) =>
    accessibleName(btn).includes(label),
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

const sign = async (payload: string) => ({ p: payload, k: 'this-browser', s: 'signature' });

function makeNodeRig(name: string, device: string) {
  const home = new TestHome(name);
  const store = new Home({ user, sign });
  const info: p.HostInfo = {
    ...p.decode(p.HostInfo, goldens['host-info-answer']),
    device,
    name,
  };
  const answerCmd = (env: p.Envelope): p.CmdResult | undefined => {
    const parsed = JSON.parse(env.p) as { t: string; agent?: string; account?: AccountKind };
    if (parsed.t === 'pair.browser') return p.decode(p.CmdResult, goldens['cmd-pair-link']);
    if (parsed.t !== 'agent.set') return undefined;
    const target = info.agents.find((item) => item.agent === parsed.agent);
    if (target && parsed.account) target.account = parsed.account;
    return { re: 1, ok: true };
  };
  return {
    home,
    store,
    async connect() {
      const link = store.connect();
      const base = home.connect(link.push);
      const ctl = {
        request: async (req: { t: string; env?: p.Envelope }) => {
          if (req.t === 'host.info') return structuredClone(info);
          const cmd = req.t === 'cmd' && req.env ? answerCmd(req.env) : undefined;
          return cmd ?? base.ctl.request(req as never);
        },
      } as p.Ctl;
      const welcome = (await base.ctl.request({
        t: 'hello',
        v: 1,
        kind: 'browser',
        name: 'test',
        addrs: [],
      })) as p.Welcome;
      link.online(ctl, { ...welcome, device });
    },
  };
}

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let home: A11yHome;
  let reader: ReturnType<typeof browser>;

  async function syncHome() {
    const expected = home.synced + home.groups.size;
    await reader.connect();
    await vi.waitFor(() => expect(home.synced).toBe(expected));
    await new Promise((later) => setTimeout(later));
    flushSync();
  }

  async function seedGroups() {
    home = new A11yHome('workstation');
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
          expect(accessibleName(prompt)).toBe('Prompt for your AI');
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
            expect(accessibleName(el).length).toBeGreaterThan(0);
          }
          const langBtn = all<HTMLButtonElement>('.onboarding .top button')[0];
          expect(accessibleName(langBtn).startsWith('中')).toBe(true);
          expect(
            [...langBtn.querySelectorAll<HTMLElement>('[lang]')].map((part) => part.lang),
          ).toEqual(['zh-CN', 'en', 'zh-CN']);

          const actionBtn = one<HTMLButtonElement>('.prompt header button');
          actionBtn.focus();
          await vi.advanceTimersByTimeAsync(600_000);
          flushSync();
          expect(accessibleName(actionBtn)).toBe('New pairing code');
          expect(document.activeElement).toBe(actionBtn);
          actionBtn.click();
          flushSync();
          expect(accessibleName(actionBtn)).toBe('Copy prompt');
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
          expect(accessibleName(main)).toBe('Settings');
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
          expect(accessibleName(one('section.list'))).toBe('Group list');
          expect(accessibleName(one('section.digest'))).toBe('Waiting on you');
          const openLinks = all('.digest .todo a');
          expect(openLinks).toHaveLength(3);
          expect(openLinks.map((link) => accessibleName(link))).toEqual([
            'Open · Confirm start · Show connection paths · Website',
            'Open · Approval · Time the path change in a browser. · Website',
            'Open · Your call · Where should the path appear? · Website',
          ]);
          for (const link of openLinks) expect(cleanText(link.textContent)).toBe('Open');
        },
      );
      When(
        'the person opens "New project group", checks its field names and descriptions, and presses Escape',
        () => {
          const opener = buttonNamed(one('section.list'), 'New project group');
          opener.focus();
          opener.click();
          flushSync();
        },
      );
      Then(
        'every dialog field is named by its label without concatenating its hint, and Escape closes the dialog with focus back on "New project group"',
        () => {
          const dlg = one<HTMLDialogElement>('dialog');
          expect(dlg.open).toBe(true);
          expect(accessibleName(dlg)).toBe('New project group');
          expect(dlg.contains(document.activeElement)).toBe(true);
          const controls = [...dlg.querySelectorAll('input, select, textarea')];
          expect(controls.map((el) => accessibleName(el))).toEqual([
            'Group name',
            'Git repository',
            'Base branch',
            'Your name in the group',
            'Pull requests and hosted checks',
            'Check commands',
            'Home computer',
          ]);
          expect(describedText(controls[1])).toContain('clone the project from');
          expect(describedText(controls[5])).toContain('One per line');
          dlg.dispatchEvent(
            new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
          );
          flushSync();
          expect(dlg.open).toBe(false);
          expect(document.activeElement).toBe(
            buttonNamed(one('section.list'), 'New project group'),
          );
        },
      );
      When('the person opens the chat of "Website" on a 1280 px desktop', () => {
        go(`#/groups/${website}`);
      });
      Then(
        'the chat, main line, waiting bar, composer and every card expose localized accessible names and linked status descriptions in English and Chinese',
        () => {
          expect(accessibleName(one('section.chat'))).toBe('Website');
          expect(accessibleName(one('.scroller'))).toBe('Main line');
          expect(accessibleName(one('nav.needbar'))).toBe('Waiting on you');
          expect(accessibleName(field())).toBe('Message');
          for (const btn of all('.chat button, .chat a')) {
            expect(accessibleName(btn).length).toBeGreaterThan(0);
          }

          type('a'.repeat(9000));
          expect(field().getAttribute('aria-invalid')).toBe('true');
          expect(describedText(field())).toContain('Too long to send');
          type('');
          expect(field().getAttribute('aria-invalid')).toBeNull();

          expect(accessibleName(one(`#m-${briefId} section.card`))).toBe(
            'Task brief · Show connection paths',
          );
          expect(accessibleName(one(`#m-${approvalId} section.card`))).toBe('Approval');
          expect(describedText(one(`#m-${approvalId} .btn-dawn`))).toBe(
            'Applies to this one request',
          );
          expect(describedText(one('#m-ap-mina .card-foot button'))).toBe(
            "Only the computer's owner Mina can approve",
          );
          expect(accessibleName(one(`#m-${decisionId} section.card`))).toBe('Your call');
          const decInput = one<HTMLInputElement>(`#m-${decisionId} input`);
          fill(decInput, 'z'.repeat(1023));
          flushSync();
          expect(decInput.getAttribute('aria-invalid')).toBe('true');
          expect(describedText(decInput)).toBe('Too long: shorten your answer to fit 1 KB.');
          fill(decInput, '');
          flushSync();
          expect(accessibleName(one('#m-msg-task-12 section.card'))).toBe(
            'T-12 Show connection path',
          );
          expect(accessibleName(one('#m-sum-1 section.card'))).toBe(
            'Done · T-12 · Show connection path',
          );

          prefs.lang = 'zh-CN';
          flushSync();
          expect(accessibleName(one('.scroller'))).toBe('主线');
          expect(accessibleName(one(`#m-${briefId} section.card`))).toBe(
            '任务说明 · Show connection paths',
          );
          expect(accessibleName(one('#m-sum-1 section.card'))).toBe(
            '完成总结 · T-12 · Show connection path',
          );
        },
      );
    },
  );

  Scenario(
    'Acting on cards, threads, dialogs and computer accounts never drops focus to the document body',
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
          home.holdAct = true;
          buttonNamed(apCard, 'Approve').click();
          flushSync();
          expect(document.activeElement).toBe(apCard);
          home.releaseAct?.();
          home.holdAct = false;
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
        'the person opens "#/nodes", declares an AI account type from the keyboard, and opens, copies and closes "Add a browser" with Escape',
        async () => {
          await closePage();
          const rig = makeNodeRig('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q');
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
            expect(all('.card.node .acct')).toEqual([]);
          });
          expect(document.activeElement).toBe(one('.card.node'));

          const pairBtn = buttonNamed(one('.page-head'), 'Add a browser');
          pairBtn.focus();
          pairBtn.click();
          flushSync();
          const dlg = one<HTMLDialogElement>('dialog');
          expect(dlg.contains(document.activeElement)).toBe(true);
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
          expect(one('.app > [role="status"]').textContent).toBe('Copied');
          dlg.dispatchEvent(
            new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
          );
          flushSync();
        },
      );
      Then(
        'choosing the account keeps focus on the computer card, copying the pairing link announces "Copied", and Escape returns focus to "Add a browser"',
        () => {
          expect(one<HTMLDialogElement>('dialog').open).toBe(false);
          expect(document.activeElement).toBe(buttonNamed(one('.page-head'), 'Add a browser'));
        },
      );
    },
  );

  Scenario(
    'Phone, tablet and desktop widths adapt whole-screen mode, connection indicator placement and side panel reachability',
    ({ Given, When, Then, And }) => {
      interface WidthSnap {
        full: string | undefined;
        barConn: number;
        headConn: string[];
        closedInert: boolean;
        openBehindInert: boolean[];
        openSideInert: boolean;
        afterEscFocus: string | null;
      }
      const snaps = new Map<number, WidthSnap>();

      Given('a group "Website" with a side panel area and an active home connection', async () => {
        const rig = makeNodeRig('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q');
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
            const sideEl = one('#chat-side');
            const closedInert = sideEl.hasAttribute('inert');
            one<HTMLButtonElement>('.side-open').click();
            await tick();
            flushSync();
            const openBehindInert = all('.chat > :not(.side)').map((el) =>
              el.hasAttribute('inert'),
            );
            const openSideInert = sideEl.hasAttribute('inert');
            window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
            flushSync();
            await tick();
            await tick();
            flushSync();
            snaps.set(width, {
              full: one('.app').dataset.full,
              barConn: all('.bar .conn').length,
              headConn: all('.chat-head .conn').map((el) => accessibleName(el)),
              closedInert,
              openBehindInert,
              openSideInert,
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
        'at 390 and 820 px the closed side panel is inert until opened, traps behind parts with inert when open, and restores focus on Escape',
        () => {
          for (const width of [390, 820]) {
            const snap = snaps.get(width)!;
            expect(snap.closedInert).toBe(true);
            expect(snap.openSideInert).toBe(false);
            expect(snap.openBehindInert).toEqual([true, true, true, true]);
            expect(snap.afterEscFocus).toBe('People and details');
          }
          expect(snaps.get(820)!.barConn).toBe(1);
          expect(snaps.get(820)!.headConn).toEqual([]);
        },
      );
      And(
        'at 1024 and 1440 px the side panel stands beside the main line without inert on either column',
        () => {
          for (const width of [1024, 1440]) {
            const snap = snaps.get(width)!;
            expect(snap.closedInert).toBe(false);
            expect(snap.openSideInert).toBe(false);
            expect(snap.openBehindInert).toEqual([false, false, false, false]);
          }
        },
      );
    },
  );

  Scenario(
    'Design tokens meet WCAG AA contrast in both themes and reduced motion stills scripted scrolling and transitions',
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
            ['text', 'bg'],
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
