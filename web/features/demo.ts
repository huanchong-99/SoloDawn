// A few groups with a morning of talk in them, for looking at the group list and the chat with
// no node: the screenshots of a pull request are taken from it. Loaded into a development page:
//   await (await import('/features/demo.ts')).demo()
import { homes } from '../src/lib/groups.svelte';
import { browser, example, owner, TestHome, user } from './home';

const minute = 60_000;
const ids = {
  rebuild: 'c29sb2Rhd24tcmVidWlsZA',
  site: 'd2Vic2l0ZS1hbmQtZG9jcw',
  sample: 'c2FtcGxlLXByb2plY3QtMQ',
  phone: 'cGhvbmUtcG9saXNoLTAwMQ',
  secretary: 'c2VjcmV0YXJ5LW1lbWJlcg',
  claude: 'Y2xhdWRlLWNvZGUtbWVtYg',
  codex: 'Y29kZXgtbWVtYmVyLTAwMQ',
  mina: 'bWluYS1tZW1iZXItMDAwMQ',
};

/** The members of a demo group besides the person: a secretary, two working AIs and Mina. */
function people(home: TestHome, g: string) {
  const ai = (id: string, name: string, cli: string, secretary = false) =>
    home.write(g, example('row-member-ai', { id, g, name, cli, secretary }));
  ai(ids.secretary, '秘书', 'claude', true);
  ai(ids.claude, 'Claude Code', 'claude');
  ai(ids.codex, 'Codex', 'codex');
  home.write(g, example('row-member-human', { id: ids.mina, g, name: 'Mina', user: 'mina' }));
  home.write(g, example('row-group', { id: g, name: home.groups.get(g)!.name }));
}

export async function demo(now = Date.now()) {
  const home = new TestHome("Alex's MacBook");
  const at = (minutesAgo: number) => ({ ts: now - minutesAgo * minute });
  const mentions = (...members: string[]) => ({ mentions: members });

  const g = home.add(ids.rebuild, 'SoloDawn 重建');
  people(home, g);
  home.say(g, owner, '@秘书 引导页的提示词还要再短一点，手机上一屏要能看完。', {
    ...at(26 * 60),
    ...mentions(ids.secretary),
  });
  home.say(g, ids.secretary, '收到。我让 @Claude Code 把第二段并进第一段，改完给你看。', {
    ...at(26 * 60 - 2),
    ...mentions(ids.claude),
  });
  home.say(
    g,
    ids.claude,
    '改好了，`web/src/routes/Onboarding.svelte` 少了 14 行，`pnpm test` 通过。',
    {
      ...at(25 * 60),
    },
  );
  home.say(
    g,
    owner,
    '@秘书 网页客户端要能看出现在是直连还是走中继。走中继的时候提示速度和流量受限，后台打通直连以后自动切回来，提示也跟着撤掉。',
    { ...at(68), ...mentions(ids.secretary) },
  );
  home.write(
    g,
    example('row-msg-brief-pending', {
      id: 'brief-1',
      by: ids.secretary,
      text: '需求清楚，不用追问。这是任务说明，你确认了我再派活：',
      ...at(66),
      body: {
        ...(example('row-msg-brief-pending').body as object),
        title: '显示连接方式，中继时提示受限',
        requester: owner,
      },
    }),
  );
  home.say(g, ids.mina, '在开会，十分钟后回来看。', at(31));
  home.say(
    g,
    ids.codex,
    '@Alex 指示组件有三种状态：直连、中继（受限）、重连中。字段我用 `path` 和 `limited`。',
    {
      ...at(24),
      ...mentions(owner),
    },
  );
  home.write(
    g,
    example('row-msg-decision-pending', {
      id: 'decision-1',
      by: ids.secretary,
      thread: null,
      text: 'Codex 两种都能做，工作量差不多。这是产品取舍，我不替你定。',
      ...at(22),
      body: {
        question: '走中继时，「受限」的提示放在哪？',
        options: [],
        to: [user],
      },
    }),
  );
  home.say(g, owner, '先按只标在连接指示上做。', at(6));
  home.say(g, owner, '另外，设置里要能改本地数据保留多少天，默认 90 天。', at(5));

  const site = home.add(ids.site, '官网与文档');
  people(home, site);
  home.say(site, ids.codex, '自建文档第 3 节已提交，等云端检查。', at(24));

  const sample = home.add(ids.sample, '「示例项目」');
  people(home, sample);
  home.say(sample, ids.secretary, 'T-4 已合并，全部任务完成。', at(27 * 60));

  // A group homed on a computer that is not there now: its rows stay, read-only.
  const away = new TestHome("Sam's Linux");
  const phone = away.add(ids.phone, '手机体验打磨');
  people(away, phone);
  away.say(phone, ids.mina, '我晚点把添加到主屏幕的截图发上来。', at(3 * 24 * 60));

  const here = browser(home);
  const there = browser(away);
  homes.length = 0;
  homes.push(here.store, there.store);
  await Promise.all([here.connect(), there.connect()]);
  await new Promise((read) => setTimeout(read, 50));
  there.drop();
  return { home, away, ids };
}
