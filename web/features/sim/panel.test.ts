// The pairing part of the demo panel: the code the person types is handed to the computer as they
// typed it, also when the page shows a code and when the panel refreshes while they are away from
// it, and the field is empty again once the computer heard it; an empty field hands over the
// page's code; the panel never takes its own example code.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Events } from './events';
import { Panel } from './panel';
import type { World } from './world';

const answerRoom = vi.hoisted(() => vi.fn(async () => true));
vi.mock('./session', () => ({ answerRoom }));

const world = {
  mode: 'fresh',
  pace: 1,
  groups: () => [],
  find: () => undefined,
} as unknown as World;
const node = { name: 'MacBook Pro' };
const events = {
  unpaired: () => node,
  computers: () => [],
  movable: () => false,
} as unknown as Events;

const code = 'WXM9-GF9D-4KXZ-WQR5';

beforeEach(() => {
  vi.useFakeTimers();
  answerRoom.mockClear();
});

afterEach(() => {
  document.body.replaceChildren();
  vi.useRealTimers();
});

/** A panel over a page that shows `shown`, opened by the person. */
function open(shown: string) {
  const page = document.createElement('main');
  page.textContent = shown;
  document.body.append(page);
  new Panel(world, events);
  const panel = document.querySelector<HTMLElement>('[data-demo=panel]')!;
  button(panel, '演示').click();
  return panel;
}

const input = (panel: HTMLElement) => panel.querySelector('input')!;
const button = (panel: HTMLElement, label: string) =>
  [...panel.querySelectorAll('button')].find((each) => each.textContent === label)!;

/** Types `text` where a click into the field puts the caret: after what the field holds. */
function type(panel: HTMLElement, text: string) {
  const field = input(panel);
  field.focus();
  field.value += text;
  field.dispatchEvent(new Event('input'));
}

async function handOver(panel: HTMLElement) {
  button(panel, '交给 MacBook Pro').click();
  await vi.advanceTimersByTimeAsync(0);
}

describe('the demo panel pairing a computer', () => {
  it('hands over a code typed while the page shows it, on the first try', async () => {
    const panel = open(`3. 用配对码 ${code}，加上 --rendezvous`);
    type(panel, code);
    await handOver(panel);
    expect(answerRoom).toHaveBeenCalledExactlyOnceWith(node, 'WXM9GF9D4KXZWQR5', 'b');
    expect(panel.textContent).toContain('MacBook Pro 收到了配对码');
    expect(input(panel).value).toBe('');
  });

  it('keeps what the person typed when it refreshes while they are away from it', async () => {
    const panel = open(`用配对码 ${code}`);
    type(panel, 'WXM9-GF9D');
    input(panel).blur();
    await vi.advanceTimersByTimeAsync(2000);
    expect(input(panel).value).toBe('WXM9-GF9D');
  });

  it("hands over the page's code from an empty field, which shows it", async () => {
    const panel = open(`用配对码 ${code}`);
    expect(input(panel)).toMatchObject({ value: '', placeholder: code });
    await handOver(panel);
    expect(answerRoom).toHaveBeenCalledExactlyOnceWith(node, 'WXM9GF9D4KXZWQR5', 'b');
  });

  it('does not take the example code of its own message for a code on the page', async () => {
    const panel = open('配对码已使用并作废。');
    await handOver(panel);
    expect(panel.textContent).toContain('这不是配对码');
    await handOver(panel);
    expect(answerRoom).not.toHaveBeenCalled();
  });
});
