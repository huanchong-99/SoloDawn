// Steps of ../features/web_e2e.feature. They need Go, the binaries and a browser, so only the
// end-to-end configuration runs them: `pnpm exec vitest run --config e2e/config.ts`.
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { mkdtemp, readFile, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import type { Browser } from 'playwright';
import { expect, inject, vi } from 'vitest';
import { launch } from '../e2e/config';
import { Net, alive, startNet, sweep } from '../e2e/nodes';

const feature = await loadFeature('../features/web_e2e.feature');

const web = resolve(import.meta.dirname, '..');

type Started = { dir: string; pids: number[] };

function expectGone({ dir, pids }: Started) {
  expect(pids.length).toBeGreaterThan(0);
  expect(pids.filter(alive)).toStrictEqual([]);
  expect(existsSync(dir)).toBe(false);
}

const started = (net: Net): Started => ({ dir: net.dir, pids: net.pids() });

const own = () => ({ HOME: process.env.HOME, PATH: process.env.PATH, cwd: process.cwd() });

// A run of its own, whose three tests each start a net and then fail, time out, or kill their
// worker. Its config provides what the global setup would, under dir, so its teardown is ours.
async function failingRun(dir: string) {
  const provide = { e2e: { root: dir, bin: inject('e2e').bin } };
  const config = { include: ['*.test.mjs'], globals: true, pool: 'forks', provide };
  await writeFile(
    join(dir, 'vitest.config.mjs'),
    `export default ${JSON.stringify({ test: config })}`,
  );
  const record = JSON.stringify(join(dir, 'started'));
  await writeFile(
    join(dir, 'harness.test.mjs'),
    `import { appendFileSync } from 'node:fs';
    import { startNet } from ${JSON.stringify(join(web, 'e2e/nodes.ts'))};
    let net;
    afterEach(() => net?.stop());
    async function start(test) {
      net = await startNet({ nodes: 2, awaitNodes: false });
      appendFileSync(${record}, JSON.stringify({ test, dir: net.dir, pids: net.pids() }) + '\\n');
    }
    test('fails', async () => { await start('fails'); throw new Error('failed on purpose'); });
    test('times out', async () => { await start('times out'); await new Promise(() => {}); }, 5000);
    test('is killed', async () => { await start('is killed'); process.kill(process.pid, 'SIGKILL'); });`,
  );
  const vitest = join(web, 'node_modules/vitest/vitest.mjs');
  const args = [vitest, 'run', '--root', dir, '--config', 'vitest.config.mjs'];
  const child = spawn(process.execPath, args, { cwd: dir });
  let output = '';
  child.stdout.on('data', (chunk) => (output += chunk));
  child.stderr.on('data', (chunk) => (output += chunk));
  const code = await new Promise((done) => child.on('close', done));
  const text = await readFile(join(dir, 'started'), 'utf8').catch(() => '');
  const nets = text
    .split('\n')
    .filter(Boolean)
    .map((line) => JSON.parse(line) as Started & { test: string });
  return { code, output, nets: Object.fromEntries(nets.map(({ test, ...net }) => [test, net])) };
}

describeFeature(
  feature,
  ({ BeforeAllScenarios, AfterAllScenarios, AfterEachScenario, Scenario }) => {
    let browser: Browser;
    let net: Net | undefined;

    BeforeAllScenarios(async () => {
      browser = await launch(inject('browser'));
    });
    AfterAllScenarios(() => browser?.close());
    // How every test stops what it started: in an after-hook, which runs when a step fails too.
    AfterEachScenario(() => net?.stop());

    Scenario('A test starts a rendezvous and two nodes and stops them', ({ When, Then, And }) => {
      let before: ReturnType<typeof own>;
      When('a test starts a rendezvous and two nodes', async () => {
        before = own();
        // A node exits until N-4 wires its runtime, so the nodes are not awaited.
        net = await startNet({ nodes: 2, awaitNodes: false });
      });
      Then(
        'the browser loads the web client from the rendezvous at its 127.0.0.1 address',
        async () => {
          expect(net!.rendezvous.url).toMatch(/^http:\/\/127\.0\.0\.1:\d+\/$/);
          expect(net!.rendezvous.signal).toBe(net!.rendezvous.url.replace('http', 'ws') + 'rv');
          const page = await browser.newPage();
          expect((await page.goto(net!.rendezvous.url))?.status()).toBe(200);
          // The client names the page it shows before its own name.
          expect(await page.title()).toMatch(/SoloDawn$/);
          await page.close();
        },
      );
      And('each node has made its data directory in a HOME of its own', async () => {
        const [first, second] = net!.nodes;
        expect(second.home).not.toBe(first.home);
        for (const node of net!.nodes) {
          expect(node.home.startsWith(net!.dir + '/')).toBe(true);
          expect(node.env.HOME).toBe(node.home);
          expect(node.dataDir.startsWith(node.home + '/')).toBe(true);
          await vi.waitFor(() => expect(existsSync(node.dataDir)).toBe(true));
        }
      });
      And('each node finds the built solodawn and the fake CLIs first on its PATH', async () => {
        for (const node of net!.nodes) {
          expect(await node.run('solodawn', 'version')).toStrictEqual({
            code: 0,
            stdout: 'dev\n',
            stderr: '',
          });
          expect((await node.run('claude', '--version')).stdout).toBe('2.1.311 (Claude Code)\n');
          expect((await node.run('codex', '--version')).stdout).toBe('codex-cli 0.164.0\n');
        }
      });
      When('the test stops them', () => net!.stop());
      Then('none of their processes runs and their directory is gone', () => {
        expect(net!.pids()).toHaveLength(3);
        expectGone(started(net!));
      });
      And('the test process has the HOME, PATH and working directory it had before', () => {
        expect(own()).toStrictEqual(before);
      });
    });

    Scenario('A start that fails stops what it had started', ({ When, Then, And }) => {
      let failure: unknown;
      When(
        'a test starts a rendezvous and a node that cannot make its data directory',
        async () => {
          net = new Net();
          const nowhere = () => ({ HOME: '/dev/null', XDG_DATA_HOME: '/dev/null' });
          failure = await net.start({ nodes: 1, env: nowhere }).catch((error: unknown) => error);
        },
      );
      Then("the start fails with the node's own message", () => {
        expect(failure).toBeInstanceOf(Error);
        expect((failure as Error).message).toMatch(/^node 0 exited \(1\):\n.*not a directory/);
      });
      And('none of the processes it started runs and its directory is gone', () => {
        expect(net!.pids()).toHaveLength(2);
        expectGone(started(net!));
      });
    });

    Scenario(
      'What failed, timed-out and killed tests started is stopped and removed',
      ({ When, Then, And }) => {
        let dir: string;
        let run: Awaited<ReturnType<typeof failingRun>>;
        When(
          'a run has a test that fails, one that times out and one whose worker is killed, each after starting a rendezvous and two nodes',
          async () => {
            dir = await mkdtemp(join(inject('e2e').root, 'run-'));
            run = await failingRun(dir);
          },
        );
        Then('the run fails', () => {
          expect(run.code, run.output).not.toBe(0);
          expect(run.output).toContain('failed on purpose');
          expect(run.output).toContain('Test timed out in 5000ms');
          expect(Object.keys(run.nets), run.output).toStrictEqual([
            'fails',
            'times out',
            'is killed',
          ]);
        });
        And(
          'nothing that the failed and the timed-out test started runs, and their directories are gone',
          () => {
            expectGone(run.nets['fails']);
            expectGone(run.nets['times out']);
          },
        );
        And(
          'what the killed test started runs until the teardown of the run removes it',
          async () => {
            const killed = run.nets['is killed'];
            expect(alive(killed.pids[0])).toBe(true);
            expect(await sweep(dir, inject('e2e').bin)).toContain(killed.pids[0]);
            expectGone(killed);
            expect(existsSync(dir)).toBe(false);
          },
        );
      },
    );
  },
);
