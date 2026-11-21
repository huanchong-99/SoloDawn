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
});
