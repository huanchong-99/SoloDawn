import { browser, example, ownedNode, TestHome } from '../features/home';
import { homes } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import { AIMemberRow, HostJoin, decode } from '../src/lib/proto';

const member = decode(AIMemberRow, goldens['row-member-ai-external']);
const { g, id } = member;
const host = 'bWNwLXdvcmtzdGF0aW9uLWRldmljZQ';
export const exe = 'C:\\Program Files\\SoloDawn\\solodawn.exe';
const home = new TestHome();
const node = ownedNode('MCP workstation', host, { exe });
const laptop = ownedNode('Laptop', 'bGFwdG9wLWRldmljZS1pZA');

async function cast() {
  home.add(g, 'Website');
  home.reply = (cmd) => {
    if (cmd.op === 'remove' && cmd.id === id) home.remove(g, 'member', id);
    return { ok: true };
  };
  node.home.reply = () => ({ ok: true });
  const connected = [browser(home), node, laptop];
  homes.push(...connected.map((each) => each.store));
  await Promise.all(connected.map((each) => each.connect()));
  return { g, id, exe, host };
}

function join() {
  const cmd = node.home.asked.find(({ t }) => t === 'cmd');
  return cmd && decode(HostJoin, (cmd.env as { p: string }).p);
}

function publish(state: string, usage = {}) {
  const command = join()!;
  home.write(
    g,
    example('row-member-ai-external', {
      id,
      g,
      host,
      state,
      usage,
      path: 'direct',
      name: command.name,
      account: command.account,
      tool: command.tool,
      host_name: 'MCP workstation',
    }),
  );
}

function answer() {
  home.say(g, id, 'Ready.', { thread: null, mentions: [] });
}

function groupCommands() {
  return home.asked
    .filter(({ t }) => t === 'cmd')
    .map(({ env }) => JSON.parse((env as { p: string }).p) as Record<string, unknown>)
    .map(({ n, ts, ...command }) => command);
}

declare global {
  interface Window {
    externalCast: typeof externalCast;
  }
}

const externalCast = { cast, join, publish, answer, groupCommands };
window.externalCast = externalCast;
