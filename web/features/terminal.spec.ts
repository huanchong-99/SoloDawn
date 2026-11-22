import 'fake-indexeddb/auto';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync, tick } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { expect, vi } from 'vitest';
import { find, Home, homes } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import type { Path, Source } from '../src/lib/net.svelte';
import { nodes } from '../src/lib/nodes.svelte';
import * as p from '../src/lib/proto';
import { example, TestHome, user } from './home';
import { all, closePage, go, one, openPage, texts } from './page';

const feature = await loadFeature('../features/web_terminal.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const workstationId = 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
const claudeId = '1PC8WineBrUQ-apCjx7tug';
const secId = 'qBSFMsr2hHYKOMblEA_kdA';
const task12Id = 'DrtCn6htSBwmMPrFPbHJHA';
const task15Id = 'O21RWBqThVezWCWeVoSoSQ';
const taskRefA = '4WxEK_P2UaD2DylCloORfg';
const taskRefB = 'qiMu2CuTcXhg2GyH0haRfA';
const taskRefC = 'netluHiPQf-UoheEIBfbLw';
const sign = async (payload: string) => ({ p: payload, k: 'this-browser', s: 'signature' });

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

const xtermState = vi.hoisted(() => ({
  instances: [] as {
    cols: number;
    rows: number;
    written: (Uint8Array | string)[];
    onDataCb?: (data: string) => void;
    onBinaryCb?: (data: string) => void;
    cleared: number;
    disposed: boolean;
  }[],
}));

vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    cols = 80;
    rows = 24;
    written: (Uint8Array | string)[] = [];
    onDataCb?: (data: string) => void;
    onBinaryCb?: (data: string) => void;
    cleared = 0;
    disposed = false;
    constructor() {
      xtermState.instances.push(this);
    }
    loadAddon() {}
    open(el: HTMLElement) {
      el.dataset.xterm = 'mounted';
    }
    write(chunk: Uint8Array | string) {
      this.written.push(chunk);
    }
    onData(cb: (data: string) => void) {
      this.onDataCb = cb;
      return { dispose: () => (this.onDataCb = undefined) };
    }
    onBinary(cb: (data: string) => void) {
      this.onBinaryCb = cb;
      return { dispose: () => (this.onBinaryCb = undefined) };
    }
    focus() {}
    clear() {
      this.cleared += 1;
    }
    dispose() {
      this.disposed = true;
    }
  },
}));

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit() {
      const latest = xtermState.instances.at(-1);
      if (latest) {
        latest.cols = 120;
        latest.rows = 32;
      }
    }
  },
}));

vi.mock('@xterm/xterm/css/xterm.css', () => ({}));

interface FakePty {
  label: string;
  sent: Uint8Array[];
  closed: boolean;
  deliver(data: Uint8Array | string): void;
  remoteClose(): void;
}

interface HostRig {
  home: TestHome;
  store: Home;
  starts: p.TakeoverStart[];
  ends: p.TakeoverEnd[];
  resizes: p.PTYResize[];
  channels: FakePty[];
  refuseStart?: string;
  hostInfoCalls: number;
  pushFrame(frame: p.Push): void;
  connect(): Promise<void>;
  drop(): void;
}

function createHostRig(): HostRig {
  const home = new TestHome('workstation');
  const store = new Home({ user, sign });
  const starts: p.TakeoverStart[] = [];
  const ends: p.TakeoverEnd[] = [];
  const resizes: p.PTYResize[] = [];
  const channels: FakePty[] = [];
  let pushLink: (frame: p.Push) => void = () => {};
  let drop = () => {};
  let seq = 200;

  const rig: HostRig = {
    home,
    store,
    starts,
    ends,
    resizes,
    channels,
    hostInfoCalls: 0,
    pushFrame: (frame) => pushLink(frame),
    async connect() {
      const link = store.connect();
      pushLink = link.push;
      const base = home.connect(link.push);
      drop = () => {
        base.close();
        link.offline();
      };
      function makeChannel(label: string): p.PtyChannel {
        let state = 'open';
        const sent: Uint8Array[] = [];
        let onmsg: ((ev: { data: unknown }) => void) | null = null;
        let oncls: (() => void) | null = null;
        const fake: FakePty = {
          label,
          sent,
          get closed() {
            return state === 'closed';
          },
          deliver(data) {
            const raw = data instanceof Uint8Array ? data.buffer : data;
            onmsg?.({ data: raw });
          },
          remoteClose() {
            state = 'closed';
            oncls?.();
          },
        };
        channels.push(fake);
        const ch: p.PtyChannel = {
          binaryType: 'blob',
          get readyState() {
            return state;
          },
          onopen: null,
          set onmessage(fn: ((ev: { data: unknown }) => void) | null) {
            onmsg = fn;
          },
          get onmessage() {
            return onmsg;
          },
          set onclose(fn: (() => void) | null) {
            oncls = fn;
          },
          get onclose() {
            return oncls;
          },
          send(bytes) {
            sent.push(bytes);
          },
          close() {
            state = 'closed';
            oncls?.();
          },
        };
        return ch;
      }
      async function handleHostCmd(env: p.Envelope): Promise<object | undefined> {
        const kind = (JSON.parse(env.p) as { t?: string }).t;
        if (kind === 'takeover.start') {
          await new Promise((later) => setTimeout(later));
          starts.push(p.decode(p.TakeoverStart, env.p));
          if (rig.refuseStart) {
            return { re: (seq += 1), err: { code: rig.refuseStart, msg: 'refused' } };
          }
          return { re: (seq += 1), ok: true as const };
        }
        if (kind === 'takeover.end') {
          ends.push(p.decode(p.TakeoverEnd, env.p));
          return { re: (seq += 1), ok: true as const };
        }
        return undefined;
      }
      const ctl: p.Ctl = {
        request: (async (req: { t: string; env?: p.Envelope }) => {
          if (req.t === 'host.info') {
            rig.hostInfoCalls += 1;
            const info = p.decode(p.HostInfo, goldens['host-info-answer']);
            return { ...info, re: (seq += 1), device: workstationId, name: 'workstation' };
          }
          const cmdOut = req.t === 'cmd' && req.env ? await handleHostCmd(req.env) : undefined;
          return cmdOut ?? base.ctl.request(req as never);
        }) as p.Ctl['request'],
        send(frame) {
          if (frame.t === 'pty.resize') resizes.push(frame);
        },
        channel: (label) => makeChannel(label),
      };
      const welcome = (await base.ctl.request({
        t: 'hello',
        v: 1,
        kind: 'browser',
        name: 'test',
        addrs: [],
      })) as p.Welcome;
      link.online(ctl, { ...welcome, device: workstationId });
    },
    drop: () => drop(),
  };
  return rig;
}

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let workstation: HostRig;

  AfterEachScenario(async () => {
    await closePage();
    paths.clear();
    nodes.infos.clear();
    xtermState.instances.length = 0;
    vi.unstubAllEnvs();
  });

  Background(({ Given }) => {
    Given(
      'a group "Website" with an owned AI "Claude Code" on "workstation", in English',
      async () => {
        workstation = createHostRig();
        workstation.home.add(website, 'Website');
        workstation.home.write(
          website,
          example('row-member-ai', {
            id: claudeId,
            g: website,
            user,
            name: 'Claude Code',
            cli: 'claude',
            host: workstationId,
            host_name: 'workstation',
            secretary: false,
            state: 'working',
            task: undefined,
            path: 'local',
          }),
        );
        homes.push(workstation.store);
        await workstation.connect();
        await vi.waitFor(() => expect(find(website)?.info).toBeDefined());
      },
    );
  });

  Scenario(
    'Opening an owned AI terminal with a task streams terminal bytes, resizes, updates queued count and renders handback',
    ({ Given, When, Then, And }) => {
      Given(
        '"Claude Code" is working on task 12 "Fix path events" on branch "task/12-conn-events" with a direct path of 4 ms',
        () => {
          workstation.home.write(
            website,
            example('row-task', {
              id: task12Id,
              g: website,
              num: 12,
              title: 'Fix path events',
              assignee: claudeId,
              branch: 'task/12-conn-events',
            }),
          );
          workstation.home.write(
            website,
            example('row-member-ai', {
              id: claudeId,
              g: website,
              user,
              name: 'Claude Code',
              cli: 'claude',
              host: workstationId,
              host_name: 'workstation',
              state: 'working',
              task: task12Id,
              path: 'local',
            }),
          );
          paths.set(workstation.store, { kind: 'direct', rtt: 4, since: Date.now() });
        },
      );
      When('the owner clicks "Open terminal" on the "Claude Code" member card', async () => {
        openPage(`#/groups/${website}`, 1440);
        const link = one<HTMLAnchorElement>('.member .card-foot a');
        expect(link.getAttribute('href')).toBe(`#/terminal/${website}/${claudeId}`);
        go(link.getAttribute('href')!);
        await vi.waitFor(() => expect(workstation.channels).toHaveLength(1));
        await vi.waitFor(() => expect(xtermState.instances).toHaveLength(1));
        flushSync();
      });
      Then(
        '"workstation" receives a signed "takeover.start" for "Claude Code" without an explicit task id',
        () => {
          expect(workstation.starts).toHaveLength(1);
          expect(workstation.starts[0]).toMatchObject({
            t: 'takeover.start',
            to: workstationId,
            g: website,
            member: claudeId,
          });
          expect(workstation.starts[0].task).toBeUndefined();
        },
      );
      And(
        'the browser opens channel "pty:rZNvy-1jH6Z-BcPqA5U5BQ:1PC8WineBrUQ-apCjx7tug" and pushes "pty.resize"',
        () => {
          expect(workstation.channels[0].label).toBe(`pty:${website}:${claudeId}`);
          expect(workstation.resizes.at(-1)).toEqual({
            t: 'pty.resize',
            g: website,
            ai: claudeId,
            cols: 120,
            rows: 32,
          });
        },
      );
      And(
        'the terminal header reads "Claude Code · Terminal" and "workstation · T-12 · task/12-conn-events"',
        () => {
          expect(texts('.term-head h2')).toEqual(['Claude Code · Terminal']);
          expect(texts('.term-head p')).toEqual(['workstation · T-12 · task/12-conn-events']);
        },
      );
      And(
        'the takeover bar reads "Owner is driving" and the session card shows "T-12 Fix path events", "task/12-conn-events" and "Direct · 4 ms"',
        () => {
          expect(texts('.takeover b')).toEqual(['Owner is driving']);
          const infoText = one('.info').textContent!.replace(/\s+/g, ' ');
          expect(infoText).toContain('T-12 Fix path events');
          expect(infoText).toContain('task/12-conn-events');
          expect(infoText).toContain('Direct · 4 ms');
        },
      );
      When(
        'the host streams prompt bytes, pushes 2 queued messages in "host.info", and the owner presses "Ctrl" then types "c" and presses "Enter"',
        () => {
          workstation.channels[0].deliver(new TextEncoder().encode('claude> '));
          const pushBase = p.decode(p.HostInfoPush, goldens['host-info-push']);
          workstation.pushFrame({
            ...pushBase,
            device: workstationId,
            agents: [
              {
                agent: 'claude',
                cli_version: '2.1.311',
                account: 'personal',
                login_ok: true,
                caps: {},
                groups: [{ g: website, member: claudeId, level: 3, state: 'takeover', queued: 2 }],
              },
            ],
          });
          flushSync();
          const buttons = all<HTMLButtonElement>('.keys button');
          const ctrlBtn = buttons.find((b) => b.textContent!.trim() === 'Ctrl')!;
          const enterBtn = buttons.find((b) => b.getAttribute('aria-label') === 'Enter')!;
          ctrlBtn.click();
          flushSync();
          expect(ctrlBtn.getAttribute('aria-pressed')).toBe('true');
          xtermState.instances[0].onDataCb!('c');
          flushSync();
          expect(ctrlBtn.getAttribute('aria-pressed')).toBe('false');
          enterBtn.click();
          flushSync();
        },
      );
      Then(
        'the takeover bar and queue card show 2 queued messages and the channel receives Ctrl-C and Enter',
        () => {
          expect(xtermState.instances[0].written).toHaveLength(1);
          expect(one('.takeover .grow').textContent).toContain(
            '2 messages are queued until you hand back.',
          );
          expect(texts('.info .badge.count')).toEqual(['2']);
          const sentStrings = workstation.channels[0].sent.map((b) => new TextDecoder().decode(b));
          expect(sentStrings).toEqual(['\x03', '\r']);
        },
      );
      When(
        'the owner clicks "Hand back" and the home writes a handback message in the main line',
        async () => {
          const handbackLink = one<HTMLAnchorElement>('.takeover .btn-dawn');
          expect(handbackLink.getAttribute('href')).toBe(`#/groups/${website}`);
          handbackLink.click();
          go(`#/groups/${website}`);
          workstation.home.write(
            website,
            example('row-msg-handback', {
              g: website,
              by: claudeId,
              thread: null,
              main: true,
            }),
          );
          await tick();
          flushSync();
        },
      );
      Then(
        'the channel closes, "workstation" receives a signed "takeover.end", and the chat renders the handback card with "path.go", "9f2c1ab", "Emit path changes" and "+40 −3"',
        () => {
          expect(workstation.channels[0].closed).toBe(true);
          expect(workstation.ends).toHaveLength(1);
          expect(workstation.ends[0]).toMatchObject({
            t: 'takeover.end',
            to: workstationId,
            g: website,
            member: claudeId,
          });
          const hb = one('.handback').textContent!.replace(/\s+/g, ' ');
          expect(hb).toContain('path.go');
          expect(hb).toContain('9f2c1ab');
          expect(hb).toContain('Emit path changes');
          expect(hb).toContain('+40 −3');
        },
      );
    },
  );

  Scenario(
    'Opening an AI without a task explains new-task creation, route task ids are forwarded, and the secretary is read-only',
    ({ Given, When, Then }) => {
      Given(
        '"Claude Code" has no task and "Secretary" is the group\'s secretary on a direct path',
        () => {
          workstation.home.write(
            website,
            example('row-member-ai', {
              id: secId,
              g: website,
              user,
              name: 'Secretary',
              cli: 'claude',
              host: workstationId,
              host_name: 'workstation',
              secretary: true,
              state: 'waiting',
              task: undefined,
              path: 'local',
            }),
          );
          paths.set(workstation.store, { kind: 'direct', rtt: 6, since: Date.now() });
        },
      );
      When(
        'the owner opens "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug"',
        async () => {
          openPage(`#/terminal/${website}/${claudeId}`, 1440);
          await vi.waitFor(() => expect(workstation.starts).toHaveLength(1));
          flushSync();
        },
      );
      Then(
        'the session card says "Opening the terminal of an AI without a task creates one so changes stay on a task branch."',
        () => {
          expect(texts('.info .card .hint')[0]).toBe(
            'Opening the terminal of an AI without a task creates one so changes stay on a task branch.',
          );
        },
      );
      When(
        'the host creates task 15 "Owner takeover" on branch "task/15-owner-takeover" for "Claude Code"',
        () => {
          workstation.home.write(
            website,
            example('row-task', {
              id: task15Id,
              g: website,
              num: 15,
              title: 'Owner takeover',
              assignee: claudeId,
              branch: 'task/15-owner-takeover',
            }),
          );
          workstation.home.write(
            website,
            example('row-member-ai', {
              id: claudeId,
              g: website,
              user,
              name: 'Claude Code',
              cli: 'claude',
              host: workstationId,
              host_name: 'workstation',
              state: 'takeover',
              task: task15Id,
              path: 'local',
            }),
          );
          flushSync();
        },
      );
      Then('the terminal header updates to "workstation · T-15 · task/15-owner-takeover"', () => {
        expect(texts('.term-head p')).toEqual(['workstation · T-15 · task/15-owner-takeover']);
      });
      When(
        'the owner navigates to "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug/O21RWBqThVezWCWeVoSoSQ"',
        async () => {
          go(`#/terminal/${website}/${claudeId}/${task15Id}`);
          await vi.waitFor(() => expect(workstation.starts).toHaveLength(2));
          flushSync();
        },
      );
      Then(
        '"workstation" receives "takeover.end" for the previous session and "takeover.start" naming task "O21RWBqThVezWCWeVoSoSQ"',
        () => {
          expect(workstation.ends).toHaveLength(1);
          expect(workstation.starts[1].task).toBe(task15Id);
        },
      );
      When(
        'the owner navigates to "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/qBSFMsr2hHYKOMblEA_kdA"',
        async () => {
          go(`#/terminal/${website}/${secId}`);
          await vi.waitFor(() => expect(workstation.starts).toHaveLength(3));
          flushSync();
        },
      );
      Then(
        'the session card shows "Read-only group session" and "The secretary\'s session is read-only and changes no files."',
        () => {
          expect(one('.info .card').textContent).toContain('Read-only group session');
          expect(texts('.info .card .hint')[0]).toBe(
            "The secretary's session is read-only and changes no files.",
          );
        },
      );
    },
  );

  Scenario(
    'Relayed paths and takeover refusals block terminal takeover clearly',
    ({ Given, When, Then }) => {
      Given('the path to "workstation" is relayed', () => {
        paths.set(workstation.store, { kind: 'relay', rtt: 140, since: Date.now() });
      });
      When('the owner opens the chat of "Website"', () => {
        openPage(`#/groups/${website}`, 1440);
      });
      Then(
        'the "Open terminal" button on "Claude Code" is disabled with reason "Direct link only"',
        () => {
          const btn = one<HTMLButtonElement>('.member .card-foot button');
          expect(btn.disabled).toBe(true);
          expect(texts('.member .card-foot .hint')).toEqual(['Direct link only']);
        },
      );
      When(
        'the owner opens "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug" directly',
        () => {
          go(`#/terminal/${website}/${claudeId}`);
        },
      );
      Then(
        'no "takeover.start" is sent and the takeover bar says "Direct connection required"',
        () => {
          expect(workstation.starts).toEqual([]);
          expect(texts('.takeover b')).toEqual(['Direct connection required']);
        },
      );
      When(
        'the path becomes direct and "workstation" refuses "takeover.start" with "relay", then "busy", then "internal"',
        async () => {
          workstation.refuseStart = 'relay';
          paths.set(workstation.store, { kind: 'direct', rtt: 8, since: Date.now() });
          go(`#/terminal/${website}/${claudeId}/${taskRefA}`);
          await vi.waitFor(() => expect(workstation.starts).toHaveLength(1));
          flushSync();
          expect(texts('.takeover b')).toEqual(['Direct connection required']);

          workstation.refuseStart = 'busy';
          go(`#/terminal/${website}/${claudeId}/${taskRefB}`);
          await vi.waitFor(() => expect(workstation.starts).toHaveLength(2));
          flushSync();
          expect(one('.takeover .grow').textContent!.trim()).toBe(
            'This AI is busy with another terminal or a merge right now.',
          );

          workstation.refuseStart = 'internal';
          go(`#/terminal/${website}/${claudeId}/${taskRefC}`);
          await vi.waitFor(() => expect(workstation.starts).toHaveLength(3));
          flushSync();
        },
      );
      Then(
        'the takeover bar shows "Direct connection required", then "This AI is busy with another terminal or a merge right now.", then error code "internal"',
        () => {
          expect(texts('.takeover code')).toEqual(['internal']);
        },
      );
    },
  );

  Scenario(
    'Tab close via pagehide and remote channel close end takeover cleanly',
    ({ Given, When, Then }) => {
      Given(
        '"Claude Code" is working on task 12 "Fix path events" on branch "task/12-conn-events" with a direct path of 4 ms',
        () => {
          paths.set(workstation.store, { kind: 'direct', rtt: 4, since: Date.now() });
        },
      );
      When(
        'the owner opens "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug" and closes the tab',
        async () => {
          openPage(`#/terminal/${website}/${claudeId}`, 1440);
          await vi.waitFor(() => expect(workstation.channels).toHaveLength(1));
          dispatchEvent(new Event('pagehide'));
          dispatchEvent(new Event('beforeunload'));
          flushSync();
        },
      );
      Then(
        'the channel closes and "workstation" receives exactly one signed "takeover.end"',
        () => {
          expect(workstation.channels[0].closed).toBe(true);
          expect(workstation.ends).toHaveLength(1);
        },
      );
      When(
        'the owner reopens "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug" and the host closes the pty channel',
        async () => {
          await closePage();
          homes.push(workstation.store);
          openPage(`#/terminal/${website}/${claudeId}`, 1440);
          await vi.waitFor(() => expect(workstation.channels).toHaveLength(2));
          workstation.channels[1].remoteClose();
          flushSync();
        },
      );
      Then(
        'the takeover bar updates to "Handed back" without sending a duplicate "takeover.end"',
        () => {
          expect(texts('.takeover b')).toEqual(['Handed back']);
          expect(workstation.ends).toHaveLength(1);
        },
      );
    },
  );

  Scenario(
    'An active terminal session closes pty and keeps ctl open when the path switches to relay',
    ({ Given, When, Then }) => {
      Given(
        '"Claude Code" is working on task 12 "Fix path events" on branch "task/12-conn-events" with a direct path of 4 ms',
        () => {
          paths.set(workstation.store, { kind: 'direct', rtt: 4, since: Date.now() });
        },
      );
      When(
        'the owner opens "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug" and the path to "workstation" switches to relay',
        async () => {
          openPage(`#/terminal/${website}/${claudeId}`, 1440);
          await vi.waitFor(() => expect(workstation.channels).toHaveLength(1));
          paths.set(workstation.store, { kind: 'relay', rtt: 190, since: Date.now() });
          flushSync();
        },
      );
      Then(
        'the pty channel is closed, the takeover bar says "Handed back · switched to relay", and "host.info" still succeeds on the existing ctl connection',
        async () => {
          expect(workstation.channels[0].closed).toBe(true);
          expect(texts('.takeover b')).toEqual(['Handed back · switched to relay']);
          const answer = await workstation.store.ctl!.request({ t: 'host.info' });
          expect(answer).toMatchObject({ t: 'host.info', ok: true, device: workstationId });
        },
      );
    },
  );

  Scenario(
    'The development states sheet mounts the handback card for the wire example',
    ({ Given, When, Then }) => {
      Given('a development server', () => {
        vi.stubEnv('MODE', 'development');
      });
      When('the owner opens "#/states"', () => openPage('#/states'));
      Then(
        'the "handback" section is marked "Mounted by terminal" and renders "path.go", "9f2c1ab", "Emit path changes" and "+40 −3"',
        async () => {
          await vi.waitFor(() => {
            flushSync();
            expect(document.getElementById('states-handback')).not.toBeNull();
          });
          const section = document.getElementById('states-handback')!.closest('section')!;
          expect(section.querySelector('header .badge')!.textContent!.trim()).toBe(
            'Mounted by terminal',
          );
          const content = section.textContent!.replace(/\s+/g, ' ');
          expect(content).toContain('path.go');
          expect(content).toContain('9f2c1ab');
          expect(content).toContain('Emit path changes');
          expect(content).toContain('+40 −3');
        },
      );
    },
  );
});
