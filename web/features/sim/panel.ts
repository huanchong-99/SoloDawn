// The demo panel: a small button at the edge of the page that opens what the person can set off
// outside the browser (events.ts), the pairing code for a computer, the speed of the scripts and a
// fresh start. Plain DOM in the colours of the client, so that it stays out of the client's code.
import { parseSecret } from '../../src/lib/pairing';
import { prefs } from '../../src/lib/prefs.svelte';
import { ids } from './ids';
import type { Events } from './events';
import { answerRoom } from './session';
import { address, type Mode, type World } from './world';

type Child = Node | string;

function el(tag: string, style: string, ...children: Child[]) {
  const made = document.createElement(tag);
  made.setAttribute('style', style);
  made.append(...children);
  return made;
}

const buttonStyle =
  'display:block;width:100%;text-align:left;margin:4px 0;padding:7px 10px;border-radius:8px;border:0;background:var(--surface-2);color:var(--text);font:inherit;cursor:pointer';
const headStyle = 'margin:12px 0 2px;font-size:12px;color:var(--text-2);font-weight:600';

function button(label: string, act: () => unknown) {
  const made = el('button', buttonStyle, label) as HTMLButtonElement;
  made.type = 'button';
  made.onclick = () => void act();
  return made;
}

/** The pairing code shown on the page outside the panel, if any: four groups of four (pairing.ts
 *  `shown`). The panel's own text holds an example code. */
const codeOnPage = (panel: Element) => {
  const page = [...document.body.children].filter((each) => each !== panel);
  const text = page.map((each) => (each as HTMLElement).innerText).join('\n');
  return /\b[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{4}\b/.exec(text)?.[0] ?? '';
};

const restart = (mode: Mode) => {
  location.href = `${address(mode)}&reset`;
};

export class Panel {
  #box = el(
    'div',
    'position:fixed;left:10px;bottom:90px;z-index:9999;font-size:13px;line-height:1.4',
  );
  #open = false;
  #note = '';
  /** What the person typed into the code field. A render (every 2 s while the panel is open)
   *  writes it back, and the page's code is only the placeholder, so that typing a code never adds
   *  to one already in the field. */
  #typed = '';

  constructor(
    readonly world: World,
    readonly events: Events,
  ) {
    this.#box.dataset.demo = 'panel';
    document.body.append(this.#box);
    this.render();
    addEventListener('hashchange', () => this.render());
    setInterval(() => {
      if (this.#open && !this.#box.matches(':hover, :focus-within')) this.render();
    }, 2000);
  }

  #say(note: string) {
    this.#note = note;
    this.render();
  }

  render() {
    const toggle = button(this.#open ? '收起演示面板' : '演示', () => {
      this.#open = !this.#open;
      this.render();
    });
    toggle.setAttribute(
      'style',
      `${buttonStyle};width:auto;border-radius:10px;box-shadow:var(--shadow-pop);background:var(--raised)`,
    );
    toggle.setAttribute('aria-expanded', String(this.#open));
    const body = this.#open ? [this.#sheet()] : [];
    this.#box.replaceChildren(...body, toggle);
  }

  #sheet() {
    const note = this.#note ? [el('p', 'margin:8px 0 0;color:var(--dawn-text)', this.#note)] : [];
    return el(
      'div',
      'width:280px;max-height:70vh;overflow:auto;margin-bottom:8px;padding:12px;border-radius:14px;background:var(--raised);color:var(--text);box-shadow:var(--shadow-pop)',
      el('b', '', '演示面板'),
      el(
        'p',
        'margin:4px 0 0;color:var(--text-2)',
        '模拟浏览器以外发生的事。没有真的电脑、AI 或网络。',
      ),
      ...this.#pairing(),
      ...this.#computers(),
      ...this.#groups(),
      ...this.#mina(),
      el('div', headStyle, '提醒与速度'),
      button('模拟一条提醒（网页关着时会收到）', () => this.#remind()),
      button(this.world.pace < 1 ? '恢复正常速度' : '快进（AI 动作快 3 倍）', () => {
        this.world.pace = this.world.pace < 1 ? 1 : 0.33;
        this.render();
      }),
      el('div', headStyle, '重新开始'),
      button('重置演示（回到最初的样子）', () => restart(this.world.mode)),
      button('从空白开始：引导和配对', () => restart('fresh')),
      button('看配对失败的样子', () => restart('pairfail')),
      ...(this.world.mode === 'full' ? [] : [button('回到完整演示', () => restart('full'))]),
      ...note,
    );
  }

  #pairing() {
    const node = this.events.unpaired();
    if (!node) return [];
    const input = el('input', `${buttonStyle};cursor:text`) as HTMLInputElement;
    input.value = this.#typed;
    input.placeholder = codeOnPage(this.#box) || '例如 7K2M-9QXD-4F8T-WB3N';
    input.oninput = () => {
      this.#typed = input.value;
    };
    return [
      el('div', headStyle, `配对：${node.name}`),
      el(
        'p',
        'margin:0;color:var(--text-2)',
        '把引导页的配对码交给这台电脑上的 AI（这里代替你在电脑上操作）。',
      ),
      input,
      button(`交给 ${node.name}`, () => this.#pair(this.#typed || codeOnPage(this.#box))),
    ];
  }

  async #pair(code: string) {
    const node = this.events.unpaired()!;
    let secret: string;
    try {
      secret = parseSecret(code.trim());
    } catch {
      return this.#say('这不是配对码：要 16 个字符，例如 7K2M-9QXD-4F8T-WB3N。');
    }
    const heard = await answerRoom(node, secret, 'b');
    if (heard) this.#typed = '';
    this.#say(
      heard
        ? `${node.name} 收到了配对码，看引导页。`
        : '没有浏览器在等这个码：先打开引导页，再把码交过来。',
    );
  }

  #computers() {
    const own = this.events.computers().filter((node) => node.paired);
    if (!own.length) return [];
    return [
      el('div', headStyle, '电脑'),
      ...own.map((node) =>
        button(node.down ? `${node.name}：恢复网络` : `${node.name}：断网`, () => {
          this.events.setDown(node.device, !node.down);
          this.render();
        }),
      ),
    ];
  }

  #groups() {
    const groups = this.world.groups().filter((g) => this.events.movable(g));
    if (!groups.length) return [];
    return [
      el('div', headStyle, '在电脑上用命令行搬家'),
      ...groups.map((g) => {
        const name = this.world.home(g)!.groups.get(g)!.name;
        return button(`把「${name}」搬到另一台电脑`, () =>
          this.#say(`已导出，正在 ${this.events.move(g)} 上导入「${name}」…`),
        );
      }),
    ];
  }

  #mina() {
    const ai = this.world.find<{ state: string }>(ids.site, 'member', ids.minaAi);
    if (!ai) return [];
    const taken = ai.state === 'takeover';
    return [
      el('div', headStyle, 'Mina'),
      button(taken ? 'Mina 交还她的终端' : 'Mina 接管她的 AI 的终端', () => {
        this.events.minaTerminal(!taken);
        this.render();
      }),
      button('Mina 请你的 Claude Code 帮忙', () => this.events.minaAsks()),
    ];
  }

  #remind() {
    const shown = this.events.remind(prefs.lang);
    this.#say(
      shown
        ? '提醒已发出，看系统通知。点它会打开对应的卡片。'
        : '没有发出：先在「设置 → 通知」里打开浏览器提醒，并允许这个网页发通知。',
    );
  }
}
