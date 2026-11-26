import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

let reducedMotion: boolean;

async function load() {
  vi.resetModules();
  return import('./jump');
}

beforeEach(() => {
  reducedMotion = false;
  document.body.innerHTML = '';
  vi.stubGlobal('matchMedia', (query: string) => ({
    get matches() {
      return query === '(prefers-reduced-motion: reduce)' && reducedMotion;
    },
  }));
});

afterEach(() => {
  document.body.innerHTML = '';
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('jumpToMessage', () => {
  it('prevents default navigation even when the target message is not in the DOM', async () => {
    const { jumpToMessage } = await load();
    const empty = document.body.appendChild(document.createElement('div'));
    empty.id = '';
    const unprefixed = document.body.appendChild(document.createElement('div'));
    unprefixed.id = 'card-1';
    const emptyScroll = vi.fn();
    const unprefixedScroll = vi.fn();
    empty.scrollIntoView = emptyScroll;
    unprefixed.scrollIntoView = unprefixedScroll;

    const event = new MouseEvent('click', { cancelable: true });
    jumpToMessage(event, 'card-1', 'approval');

    expect(event.defaultPrevented).toBe(true);
    expect(emptyScroll).not.toHaveBeenCalled();
    expect(unprefixedScroll).not.toHaveBeenCalled();
    expect(unprefixed.hasAttribute('tabindex')).toBe(false);
  });

  it('scrolls smoothly by default and focuses the first enabled button for non-decision kinds', async () => {
    const { jumpToMessage } = await load();
    const el = document.body.appendChild(document.createElement('article'));
    el.id = 'm-ap-1';
    el.innerHTML =
      '<input value="ignored" /><button disabled>Disabled</button><button>Approve</button>';
    const scroll = vi.fn();
    el.scrollIntoView = scroll;
    const approve = el.querySelectorAll('button')[1];
    const buttonFocus = vi.spyOn(approve, 'focus');
    const elFocus = vi.spyOn(el, 'focus');

    const event = new MouseEvent('click', { cancelable: true });
    jumpToMessage(event, 'ap-1', 'approval');

    expect(event.defaultPrevented).toBe(true);
    expect(scroll).toHaveBeenCalledExactlyOnceWith({ behavior: 'smooth', block: 'center' });
    expect(buttonFocus).toHaveBeenCalledExactlyOnceWith({ preventScroll: true });
    expect(elFocus).not.toHaveBeenCalled();
    expect(el.hasAttribute('tabindex')).toBe(false);
    expect(document.activeElement).toBe(approve);
  });

  it('uses auto scrolling under reduced motion and focuses enabled inputs or textareas for decisions', async () => {
    const { jumpToMessage } = await load();
    reducedMotion = true;
    const el = document.body.appendChild(document.createElement('article'));
    el.id = 'm-dc-1';
    el.innerHTML = '<button>Option</button><input disabled /><textarea></textarea>';
    const scroll = vi.fn();
    el.scrollIntoView = scroll;
    const textarea = el.querySelector('textarea')!;
    const areaFocus = vi.spyOn(textarea, 'focus');
    const elFocus = vi.spyOn(el, 'focus');

    const event = new MouseEvent('click', { cancelable: true });
    jumpToMessage(event, 'dc-1', 'decision');

    expect(event.defaultPrevented).toBe(true);
    expect(scroll).toHaveBeenCalledExactlyOnceWith({ behavior: 'auto', block: 'center' });
    expect(areaFocus).toHaveBeenCalledExactlyOnceWith({ preventScroll: true });
    expect(elFocus).not.toHaveBeenCalled();
    expect(el.hasAttribute('tabindex')).toBe(false);
    expect(document.activeElement).toBe(textarea);
  });

  it('falls back to focusing the message element with tabIndex -1 when no control is enabled', async () => {
    const { jumpToMessage } = await load();
    const el = document.body.appendChild(document.createElement('article'));
    el.id = 'm-sum-1';
    el.innerHTML = '<button disabled>Done</button>';
    const scroll = vi.fn();
    el.scrollIntoView = scroll;
    const elFocus = vi.spyOn(el, 'focus');

    const event = new MouseEvent('click', { cancelable: true });
    jumpToMessage(event, 'sum-1');

    expect(event.defaultPrevented).toBe(true);
    expect(scroll).toHaveBeenCalledExactlyOnceWith({ behavior: 'smooth', block: 'center' });
    expect(el.tabIndex).toBe(-1);
    expect(el.getAttribute('tabindex')).toBe('-1');
    expect(elFocus).toHaveBeenCalledExactlyOnceWith({ preventScroll: true });
    expect(document.activeElement).toBe(el);
  });

  it("opens the thread of a task's card and focuses its line", async () => {
    const { jumpToMessage } = await load();
    const el = document.body.appendChild(document.createElement('article'));
    el.id = 'm-task-1';
    el.innerHTML =
      '<button>Menu</button><details><summary>Other</summary></details>' +
      '<details class="thread-fold"><summary>Thread</summary></details>';
    const scroll = vi.fn();
    el.scrollIntoView = scroll;
    jumpToMessage(new MouseEvent('click'), 'task-1', 'mention');
    expect(scroll.mock.calls).toEqual([[{ behavior: 'auto', block: 'center' }]]);
    expect([...el.querySelectorAll('details')].map((each) => each.open)).toEqual([false, true]);
    expect(document.activeElement?.textContent).toBe('Thread');
    jumpToMessage(new MouseEvent('click'), 'task-1', 'mention');
    expect(scroll.mock.calls[1]).toEqual([{ behavior: 'smooth', block: 'center' }]);
  });
});

describe('jumpToMessage in the main line', () => {
  // The main line's layout: a reversed scroller 600 px high whose line is `height` px, with the
  // message's top `at` px below the line's top. The scroller ends where the line ends.
  function line(at: number, height = 3000) {
    document.body.innerHTML =
      '<div class="scroller"><div class="line"><article id="m-x-1"></article></div></div>';
    const scroller = document.querySelector<HTMLElement>('.scroller')!;
    const content = scroller.firstElementChild as HTMLElement;
    const el = content.firstElementChild as HTMLElement;
    const layout = { at, height, top: 0 };
    const sized = (node: HTMLElement, props: Record<string, () => number>) => {
      for (const [name, get] of Object.entries(props))
        Object.defineProperty(node, name, { configurable: true, get });
    };
    sized(scroller, {
      clientHeight: () => 600,
      scrollHeight: () => layout.height,
      scrollTop: () => layout.top,
    });
    sized(content, { offsetTop: () => 0, offsetHeight: () => layout.height });
    sized(el, { offsetTop: () => layout.at, offsetHeight: () => 100 });
    const scrollTo = vi.fn(({ top }: ScrollToOptions) => {
      layout.top = top!;
    });
    scroller.scrollTo = scrollTo as unknown as HTMLElement['scrollTo'];
    return { scroller, el, layout, scrollTo };
  }

  // The ResizeObserver of the line: what it calls back, and whether it still observes.
  let sizes: { changed: () => void; observed: Element[] };
  beforeEach(() => {
    sizes = { changed: () => {}, observed: [] };
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(changed: () => void) {
          sizes.changed = changed;
        }
        observe(el: Element) {
          sizes.observed.push(el);
        }
        disconnect() {
          sizes.observed = [];
        }
      },
    );
  });

  it('scrolls the message to the middle, smoothly, and turns the browser anchoring off', async () => {
    const { jumpToMessage } = await load();
    const { scroller, el, scrollTo } = line(1000);
    const scroll = vi.fn();
    el.scrollIntoView = scroll;
    jumpToMessage(new MouseEvent('click'), 'x-1');
    // Its top 250 px below the scroller's top: 600 - 250 - (3000 - 1000).
    expect(scrollTo.mock.calls).toEqual([[{ top: -1650, behavior: 'smooth' }]]);
    expect(scroll).not.toHaveBeenCalled();
    expect(scroller.style.overflowAnchor).toBe('none');
    expect(sizes.observed).toEqual([scroller.firstElementChild]);
    expect(document.activeElement).toBe(el);
  });

  it('keeps the message in place as the line grows below and above it', async () => {
    const { jumpToMessage } = await load();
    reducedMotion = true;
    const { layout, scrollTo } = line(1000);
    jumpToMessage(new MouseEvent('click'), 'x-1');
    layout.height += 500;
    sizes.changed();
    layout.height += 700;
    layout.at += 700;
    sizes.changed();
    expect(scrollTo.mock.calls.map(([options]) => options)).toEqual([
      { top: -1650, behavior: 'auto' },
      { top: -2150, behavior: 'auto' },
    ]);
  });

  // Its first call, in the frame that the main line's IntersectionObserver may first look in.
  it('leaves a smooth jump on its way while the line has not moved the message', async () => {
    const { jumpToMessage } = await load();
    const { scrollTo } = line(1000);
    scrollTo.mockImplementation(() => {});
    jumpToMessage(new MouseEvent('click'), 'x-1');
    sizes.changed();
    expect(scrollTo.mock.calls).toEqual([[{ top: -1650, behavior: 'smooth' }]]);
  });

  it('keeps the message where it was before its thread opened', async () => {
    const { jumpToMessage } = await load();
    const { el, layout, scrollTo } = line(1000);
    el.innerHTML = '<details class="thread-fold"><summary>Thread</summary></details>';
    Object.defineProperty(el, 'offsetHeight', { configurable: true, value: 100 });
    const open = Object.getOwnPropertyDescriptor(HTMLDetailsElement.prototype, 'open')!;
    vi.spyOn(el.querySelector('details')!, 'open', 'set').mockImplementation(function (
      this: HTMLDetailsElement,
      value: boolean,
    ) {
      open.set!.call(this, value);
      layout.height += 400;
      Object.defineProperty(el, 'offsetHeight', { configurable: true, value: 500 });
    });
    jumpToMessage(new MouseEvent('click'), 'x-1');
    // The top of the closed card in the middle, 250 px down: 600 - 250 - (3400 - 1000).
    expect(scrollTo.mock.calls).toEqual([[{ top: -2050, behavior: 'auto' }]]);
    sizes.changed();
    expect(scrollTo).toHaveBeenCalledTimes(1);
  });

  it('puts the message back at once, also while the smooth scroll of the jump is on its way', async () => {
    const { jumpToMessage } = await load();
    const { layout, scrollTo } = line(1000);
    scrollTo.mockImplementation(() => {});
    jumpToMessage(new MouseEvent('click'), 'x-1');
    layout.top = -900;
    layout.height += 500;
    sizes.changed();
    expect(scrollTo.mock.lastCall).toEqual([{ top: -2150, behavior: 'auto' }]);
  });

  it('does not hold a message the jump puts at the end of the line', async () => {
    const { jumpToMessage } = await load();
    const { scroller, scrollTo } = line(2950);
    jumpToMessage(new MouseEvent('click'), 'x-1');
    expect(scrollTo.mock.calls).toEqual([[{ top: 0, behavior: 'smooth' }]]);
    expect([scroller.style.overflowAnchor, sizes.observed]).toEqual(['', []]);
  });

  // A scroll by Space, Tab to a control out of view, the scrollbar, assistive technology or find.
  it('lets the message go on a scroll that is not its own, once the jump has landed', async () => {
    const { jumpToMessage } = await load();
    const { scroller, layout, scrollTo } = line(1000);
    scrollTo.mockImplementation(() => {});
    jumpToMessage(new MouseEvent('click'), 'x-1');
    const scroll = (top: number) => {
      layout.top = top;
      scroller.dispatchEvent(new Event('scroll'));
    };
    scroll(-900);
    expect(scroller.style.overflowAnchor).toBe('none');
    scroll(-1649);
    scroll(-1651);
    expect(scroller.style.overflowAnchor).toBe('none');
    scroll(-1300);
    expect([scroller.style.overflowAnchor, sizes.observed]).toEqual(['', []]);
  });

  it('keeps the message on its own scroll that a growth of the line overtook', async () => {
    const { jumpToMessage } = await load();
    reducedMotion = true;
    const { scroller, layout } = line(1000);
    jumpToMessage(new MouseEvent('click'), 'x-1');
    layout.height += 1400;
    scroller.dispatchEvent(new Event('scroll'));
    expect(scroller.style.overflowAnchor).toBe('none');
  });

  // The line, at its top, grows below the message: the browser keeps its top, and the message.
  it('keeps the message on a scroll that leaves it in place', async () => {
    const { jumpToMessage } = await load();
    const { scroller, layout } = line(50);
    jumpToMessage(new MouseEvent('click'), 'x-1');
    layout.height += 1400;
    layout.top -= 1400;
    scroller.dispatchEvent(new Event('scroll'));
    expect(scroller.style.overflowAnchor).toBe('none');
  });

  it('lets the message go on a scroll that is not its own after an instant jump', async () => {
    const { jumpToMessage } = await load();
    reducedMotion = true;
    const { scroller, layout } = line(1000);
    jumpToMessage(new MouseEvent('click'), 'x-1');
    layout.height += 500;
    sizes.changed();
    scroller.dispatchEvent(new Event('scroll'));
    expect(scroller.style.overflowAnchor).toBe('none');
    layout.top += 300;
    scroller.dispatchEvent(new Event('scroll'));
    expect(scroller.style.overflowAnchor).toBe('');
  });

  it('keeps a message the line cannot bring to the middle where the jump put it', async () => {
    const { jumpToMessage } = await load();
    reducedMotion = true;
    const { layout, scrollTo } = line(50);
    jumpToMessage(new MouseEvent('click'), 'x-1');
    // The line's top: 3000 - 600. Messages before it then make room above.
    layout.height += 1000;
    layout.at += 1000;
    sizes.changed();
    layout.height += 300;
    sizes.changed();
    // 600 - 50 - (4300 - 1050), not the middle.
    expect(scrollTo.mock.calls.map(([options]) => options.top)).toEqual([-2400, -2700]);
  });

  it.each([
    ['wheel', new WheelEvent('wheel')],
    ['touchmove', new Event('touchmove')],
  ])(
    'lets the message go when the person scrolls during the smooth scroll (%s)',
    async (_, event) => {
      const { jumpToMessage } = await load();
      const { scroller, scrollTo } = line(1000);
      jumpToMessage(new MouseEvent('click'), 'x-1');
      scroller.dispatchEvent(event);
      expect(scroller.style.overflowAnchor).toBe('');
      expect(sizes.observed).toEqual([]);
      scroller.dispatchEvent(event);
      expect(scrollTo).toHaveBeenCalledTimes(1);
    },
  );

  it('keeps the message, once landed, while the person clicks or types in it', async () => {
    const { jumpToMessage } = await load();
    reducedMotion = true;
    const { scroller } = line(1000);
    jumpToMessage(new MouseEvent('click'), 'x-1');
    scroller.dispatchEvent(new Event('pointerdown'));
    scroller.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    expect(scroller.style.overflowAnchor).toBe('none');
  });

  it.each([
    ['a press on the scrollbar', new Event('pointerdown')],
    ['Space', new KeyboardEvent('keydown', { key: ' ', bubbles: true })],
    ['PageDown', new KeyboardEvent('keydown', { key: 'PageDown', bubbles: true })],
  ])('lets the message go on %s during the smooth scroll of the jump', async (_, event) => {
    const { jumpToMessage } = await load();
    const { scroller, scrollTo } = line(1000);
    scrollTo.mockImplementation(() => {});
    jumpToMessage(new MouseEvent('click'), 'x-1');
    scroller.dispatchEvent(new KeyboardEvent('keydown', { key: 'a', bubbles: true }));
    expect(scroller.style.overflowAnchor).toBe('none');
    scroller.dispatchEvent(event);
    expect([scroller.style.overflowAnchor, sizes.observed]).toEqual(['', []]);
  });

  it('lets the message go when it is released, gone, or another is jumped to', async () => {
    const { jumpToMessage, release } = await load();
    const { scroller, el, scrollTo } = line(1000);
    jumpToMessage(new MouseEvent('click'), 'x-1');
    release();
    expect([scroller.style.overflowAnchor, sizes.observed]).toEqual(['', []]);

    jumpToMessage(new MouseEvent('click'), 'x-1');
    el.remove();
    sizes.changed();
    expect([scroller.style.overflowAnchor, sizes.observed]).toEqual(['', []]);
    expect(scrollTo).toHaveBeenCalledTimes(2);

    const { scroller: next } = line(1000);
    jumpToMessage(new MouseEvent('click'), 'x-1');
    const first = sizes.changed;
    jumpToMessage(new MouseEvent('click'), 'x-1');
    expect(sizes.changed).not.toBe(first);
    expect(sizes.observed).toEqual([next.firstElementChild]);
  });
});
