// The processes of the end-to-end tests. Vitest runs this file as the global setup of
// e2e/config.ts, once per run; a test then starts its own rendezvous and nodes with startNet and
// stops them in an after-hook, which runs when the test fails or times out too.
import { execFile, execFileSync, spawn } from 'node:child_process';
import { appendFileSync } from 'node:fs';
import { mkdir, mkdtemp, readFile, readdir, rm } from 'node:fs/promises';
import { createConnection, createServer, type AddressInfo } from 'node:net';
import { delimiter, join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { inject } from 'vitest';
import type { TestProject } from 'vitest/node';
import { build } from './config';

declare module 'vitest' {
  export interface ProvidedContext {
    /** The run's directory and the directory of its binaries. */
    e2e: { root: string; bin: string };
  }
}

export type Env = Record<string, string>;
export type Ran = { code: number; stdout: string; stderr: string };

/** `url` is `http://127.0.0.1:<port>/`, which serves the web client; `signal` is `ws://…/rv`. */
export type Rendezvous = { url: string; signal: string; pid: number };

export interface Node {
  name: string;
  pid: number;
  home: string;
  dataDir: string;
  /** The local socket, through which every command but `node` reaches the node. */
  socket: string;
  env: Env;
  /** Resolves when the node answers on its local socket; rejects with its output if it exits. */
  ready(): Promise<void>;
  /** Runs a program found on the node's PATH, `solodawn` or a fake CLI, with the node's env. */
  run(command: string, ...args: string[]): Promise<Ran>;
}

export interface StartOptions {
  nodes: number;
  /** Wait until every node is ready (the default), or return once they are started. */
  awaitNodes?: boolean;
  /** More environment for node i, such as SOLODAWN_FAKE_CLAUDE_SCRIPT; it wins over the harness. */
  env?: (node: number) => Env;
}

export type Proc = ReturnType<typeof leader>;

/** Builds once for the whole run; the teardown kills what tests left and removes the run. */
export async function setup(project: TestProject) {
  if (process.platform === 'win32') throw new Error('the harness stops processes by process group');
  // Short: on macOS a node's socket is <HOME>/Library/Application Support/solodawn/sock, and a
  // Unix socket path has at most 103 bytes. The system's temporary directory is too long there.
  const root = await mkdtemp('/tmp/sd-');
  const bin = await build(root).catch(async (error: unknown) => {
    await rm(root, { recursive: true, force: true });
    throw error;
  });
  project.provide('e2e', { root, bin });
  return async () => {
    const left = await sweep(root, bin);
    if (left.length) console.warn(`e2e: killed process groups ${left} that a test left running`);
  };
}

/** A rendezvous and nodes for one test, under a directory of their own. */
export class Net {
  dir = '';
  rendezvous: Rendezvous = { url: '', signal: '', pid: 0 };
  readonly nodes: Node[] = [];
  readonly #procs: Proc[] = [];
  #record = ''; // <run>/pids, which outlives the net's directory for the run's teardown

  /** Starts them; when that fails, stops what it started and removes the directory first. */
  async start({ nodes, awaitNodes = true, env = () => ({}) }: StartOptions) {
    const { root, bin } = inject('e2e');
    this.#record = join(root, 'pids');
    this.dir = await mkdtemp(join(root, 'n-'));
    try {
      this.rendezvous = await this.#rendezvous(bin);
      for (let i = 0; i < nodes; i++) this.nodes.push(await this.#node(bin, i, env(i)));
      if (awaitNodes) await Promise.all(this.nodes.map((node) => node.ready()));
    } catch (error) {
      await this.stop().catch((also) => console.error('e2e: a stop after a failed start:', also));
      throw error;
    }
    return this;
  }

  /** The process groups it started, one per process: a leader and whatever that started. */
  pids() {
    return this.#procs.map((proc) => proc.pid);
  }

  /** Stops every group it started, also when a test fails, and removes its directory. */
  async stop() {
    const stopped = await Promise.allSettled(this.#procs.map(stop));
    await rm(this.dir, { recursive: true, force: true });
    const errors = stopped.flatMap((s) => (s.status === 'rejected' ? [s.reason] : []));
    if (errors.length) throw new AggregateError(errors, 'some processes did not stop');
  }

  async #rendezvous(bin: string): Promise<Rendezvous> {
    const env = environment(bin, await this.#home('rv'));
    for (let attempt = 1; ; attempt++) {
      const port = await freePort();
      const listen = ['rendezvous', '--listen', `127.0.0.1:${port}`];
      const proc = this.#start('rendezvous', [join(bin, 'solodawn'), ...listen], env);
      const url = `http://127.0.0.1:${port}/`;
      try {
        await ready(proc, () => answers(url));
        return { url, signal: `ws://127.0.0.1:${port}/rv`, pid: proc.pid };
      } catch (error) {
        // Another process can bind the port between freePort and the rendezvous.
        if (attempt === 3 || !proc.output().includes('address already in use')) throw error;
      }
    }
  }

  async #node(bin: string, i: number, extra: Env): Promise<Node> {
    const name = `node ${i}`;
    const home = await this.#home(`h${i}`);
    const env = { ...environment(bin, home), ...extra };
    const proc = this.#start(name, [join(bin, 'solodawn'), 'node'], env);
    // Where the node puts it (contracts/local.md 2, internal/node/dir.go).
    const dataDir =
      process.platform === 'darwin'
        ? join(env.HOME, 'Library/Application Support/solodawn')
        : join(env.XDG_DATA_HOME, 'solodawn');
    const socket = join(dataDir, 'sock');
    return {
      name,
      pid: proc.pid,
      home,
      dataDir,
      socket,
      env,
      ready: () => ready(proc, () => connects(socket)),
      run: (command, ...args) => run(command, args, { cwd: home, env }),
    };
  }

  async #home(name: string) {
    const home = join(this.dir, name);
    await mkdir(home);
    return home;
  }

  #start(name: string, argv: string[], env: Env) {
    const proc = leader(name, argv, env, this.dir, this.#record);
    this.#procs.push(proc);
    return proc;
  }
}

/** Starts a rendezvous and nodes, each with its own HOME and the fake CLIs first on its PATH. */
export function startNet(options: StartOptions) {
  return new Net().start(options);
}

/** Nothing else of the test process: PATH starts with the binaries, everything else is the net's. */
function environment(bin: string, home: string): Env {
  return {
    PATH: `${bin}${delimiter}${process.env.PATH ?? ''}`,
    HOME: home,
    XDG_DATA_HOME: join(home, '.local/share'),
    XDG_CONFIG_HOME: join(home, '.config'),
    SOLODAWN_FAKECLI_LOG: join(home, 'fakecli.log'),
  };
}

/** Starts a process in dir as the leader of a new group, and appends the group to record. */
export function leader(
  name: string,
  [command, ...args]: string[],
  env: Env,
  dir: string,
  record: string,
) {
  const child = spawn(command, args, { cwd: dir, env, detached: true });
  child.stdin.end();
  let output = '';
  // The exit code, or the signal, once the process and its output have ended.
  const closed = new Promise<number | string>((resolve) => {
    child.on('close', (code, signal) => resolve(code ?? signal ?? 'unknown'));
    child.on('error', (error) => {
      output += error.message;
      resolve('error');
    });
  });
  if (child.pid === undefined) throw new Error(`${name}: cannot start ${command}`);
  appendFileSync(record, `${child.pid}\n`);
  const keep = (chunk: Buffer) => (output = (output + chunk).slice(-16_384));
  child.stdout.on('data', keep);
  child.stderr.on('data', keep);
  return { name, pid: child.pid, closed, output: () => output };
}

/** Sends a signal to a process group; false when the group has no process left. */
function signal(pgid: number, name: NodeJS.Signals | 0) {
  // process.kill(-0) would signal the test process's own group.
  if (!(pgid > 0)) throw new Error(`not a process group: ${pgid}`);
  try {
    process.kill(-pgid, name);
  } catch (error) {
    const { code } = error as NodeJS.ErrnoException;
    // macOS answers EPERM for a group whose processes have all exited but are not reaped yet.
    if (code === 'ESRCH') return false;
    if (code !== 'EPERM') throw error;
  }
  return true;
}

/** Whether a process of the group runs: one that has exited counts even before it is reaped. */
export function alive(pgid: number) {
  if (!signal(pgid, 0)) return false;
  const table = execFileSync('ps', ['-A', '-o', 'pgid=,stat='], { encoding: 'utf8' });
  const rows = table.matchAll(/^\s*(\d+)\s+(\S+)/gm);
  return [...rows].some(([, group, stat]) => Number(group) === pgid && !stat.startsWith('Z'));
}

/** Stops a process group started by `leader` (SIGTERM, 3 s grace, SIGKILL, and reap check). */
export async function stop(proc: Proc) {
  signal(proc.pid, 'SIGTERM');
  await Promise.race([proc.closed, delay(3000)]);
  signal(proc.pid, 'SIGKILL');
  await proc.closed;
  const failed = () => `${proc.name} did not stop`;
  await until(() => !alive(proc.pid), failed);
}

async function until(check: () => boolean | Promise<boolean>, failure: () => string) {
  for (const end = Date.now() + 15_000; !(await check()); await delay(25))
    if (Date.now() > end) throw new Error(failure());
}

/** Waits until check holds; fails with the process's output when it exits first. */
async function ready(proc: Proc, check: () => Promise<boolean>) {
  let exit: number | string | undefined;
  void proc.closed.then((how) => (exit = how));
  const exited = () => `${proc.name} exited (${exit}):\n${proc.output()}`;
  const late = () => `${proc.name} was not ready after 15 s:\n${proc.output()}`;
  await until(() => (exit === undefined ? check() : Promise.reject(new Error(exited()))), late);
}

const answers = (url: string) => fetch(url, { method: 'HEAD' }).then(Boolean, () => false);

function connects(path: string) {
  return new Promise<boolean>((resolve) => {
    let reply = '';
    const socket = createConnection(path, () => {
      socket.write('{"cmd":"status"}\n');
    });
    socket.setTimeout(5000, () => socket.destroy());
    socket.on('data', (chunk: Buffer) => (reply += chunk.toString('utf8')));
    socket.once('error', () => resolve(false));
    socket.once('close', () => resolve(reply.includes('"ok":true')));
  });
}

function freePort() {
  return new Promise<number>((resolve, reject) => {
    const server = createServer().once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address() as AddressInfo;
      server.close(() => resolve(port));
    });
  });
}

function run(command: string, args: string[], options: { cwd: string; env?: Env }) {
  return new Promise<Ran>((resolve, reject) => {
    execFile(
      command,
      args,
      { ...options, maxBuffer: 64 * 1024 * 1024 },
      (error, stdout, stderr) => {
        const code = error ? error.code : 0;
        if (typeof code === 'number') resolve({ code, stdout, stderr });
        else reject(error);
      },
    );
  });
}

/**
 * Kills the groups recorded in the pids files under root that still run a program of bin (left by
 * a test process that died, or a stop that failed), removes root and returns the groups. A group
 * that runs no program of bin has ended, and its number may belong to someone else's by now.
 */
export async function sweep(root: string, bin: string) {
  const entries = await readdir(root, { recursive: true, withFileTypes: true });
  const lists = entries.filter((e) => e.isFile() && e.name === 'pids');
  const text = await Promise.all(lists.map((e) => readFile(join(e.parentPath, e.name), 'utf8')));
  const ours = groups(bin);
  const recorded = text.join('').split('\n');
  const left = recorded.map(Number).filter((pgid) => ours.has(pgid));
  for (const pgid of left) signal(pgid, 'SIGKILL');
  const failed = () => `process groups ${left} did not stop`;
  await until(() => !left.some(alive), failed);
  await rm(root, { recursive: true, force: true });
  return left;
}

/** The process groups that have a process started from a program in bin. */
function groups(bin: string) {
  const table = execFileSync('ps', ['-A', '-o', 'pgid=,command='], { encoding: 'utf8' });
  const found = new Set<number>();
  for (const [, pgid, command] of table.matchAll(/^\s*(\d+) (.*)$/gm))
    if (command.startsWith(bin + '/')) found.add(Number(pgid));
  return found;
}
