import 'fake-indexeddb/auto';
import { flushSync, mount, unmount } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { guideState, resetGuideUi } from '../../lib/guide.svelte';
import type { Net } from '../../lib/net.svelte';
import { announcement, prefs } from '../../lib/prefs.svelte';
import Welcome from './Welcome.svelte';
import { beacon, clearBeacon, setBeaconHost } from './beacon';

function mockRect(target: HTMLElement, rect: Partial<DOMRect>) {
  const full = {
    x: 0,
    y: 0,
    left: 0,
    top: 0,
    width: 100,
    height: 36,
    right: 100,
    bottom: 36,
    toJSON: () => ({}),
    ...rect,
  } as DOMRect;
  return vi.spyOn(target, 'getBoundingClientRect').mockReturnValue(full);
}

function setReducedMotion(matches: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation((query: string) => ({
      matches: query === '(prefers-reduced-motion: reduce)' ? matches : false,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  );
}

describe('beacon', () => {
  let host: HTMLDivElement;

  beforeEach(() => {
    vi.useFakeTimers();
    setReducedMotion(false);
    document.body.innerHTML = '';
    host = document.createElement('div');
    document.body.appendChild(host);
    setBeaconHost(host);
  });

  afterEach(() => {
    clearBeacon();
    setBeaconHost(undefined);
    document.body.innerHTML = '';
    vi.restoreAllMocks();
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('positions a beacon around an immediate target inside the registered host', async () => {
    const target = document.createElement('button');
    target.dataset.guide = 'defaults';
    target.style.borderTopLeftRadius = '8px';
    target.scrollIntoView = vi.fn();
    mockRect(target, { left: 20, top: 30, width: 120, height: 40 });
    document.body.appendChild(target);

    await beacon('[data-guide="defaults"]');

    expect(target.scrollIntoView).toHaveBeenCalledWith({
      block: 'center',
      behavior: 'smooth',
    });
    const el = host.querySelector<HTMLElement>('.guide-beacon');
    expect(el).not.toBeNull();
    expect(el?.getAttribute('aria-hidden')).toBe('true');
    expect(el?.dataset.reduced).toBeUndefined();
    expect(el?.style.left).toBe('16px');
    expect(el?.style.top).toBe('26px');
    expect(el?.style.width).toBe('128px');
    expect(el?.style.height).toBe('48px');
    expect(el?.style.borderRadius).toBe('12px');
    expect(el?.querySelector('.guide-beacon-ring')).not.toBeNull();
    expect(el?.querySelector('.guide-beacon-pulse')).not.toBeNull();

    target.style.borderTopLeftRadius = '';
    await beacon('[data-guide="defaults"]');
    expect(host.querySelector<HTMLElement>('.guide-beacon')?.style.borderRadius).toBe('4px');
  });

  it('waits via requestAnimationFrame for delayed targets and gives up at 3000 ms', async () => {
    const callbacks = new Map<number, FrameRequestCallback>();
    let nextId = 1;
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      const id = nextId++;
      callbacks.set(id, cb);
      return id;
    });
    const cancelSpy = vi.fn((id: number) => {
      callbacks.delete(id);
    });
    vi.stubGlobal('cancelAnimationFrame', cancelSpy);

    const flushFrame = () => {
      const entries = [...callbacks.entries()];
      callbacks.clear();
      for (const [, cb] of entries) cb(performance.now());
    };

    const pending = beacon('#later');
    await vi.advanceTimersByTimeAsync(2999);
    flushFrame();
    const target = document.createElement('div');
    target.id = 'later';
    mockRect(target, { left: 0, top: 0, width: 10, height: 10 });
    document.body.appendChild(target);
    flushFrame();
    await pending;
    expect(host.querySelector('.guide-beacon')).not.toBeNull();

    clearBeacon();
    const missing = beacon('#never');
    await vi.advanceTimersByTimeAsync(2999);
    flushFrame();
    expect(callbacks.size).toBe(1);
    await vi.advanceTimersByTimeAsync(1);
    flushFrame();
    await missing;
    expect(host.querySelector('.guide-beacon')).toBeNull();
    expect(callbacks.size).toBe(0);

    const aborted = beacon('#aborted');
    expect(callbacks.size).toBe(1);
    const pendingId = [...callbacks.keys()][0]!;
    clearBeacon();
    expect(cancelSpy).toHaveBeenCalledWith(pendingId);
    await aborted;
    expect(host.querySelector('.guide-beacon')).toBeNull();
  });

  it('does not mount if aborted before or during poll continuation', async () => {
    const callbacks: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      callbacks.push(cb);
      return callbacks.length;
    });
    vi.stubGlobal('cancelAnimationFrame', vi.fn());

    const stalePoll = beacon('#stale-poll');
    clearBeacon();
    callbacks[0]?.(0);
    await stalePoll;
    expect(callbacks).toHaveLength(1);
    expect(host.querySelector('.guide-beacon')).toBeNull();

    const p = beacon('#race');
    const target = document.createElement('div');
    target.id = 'race';
    mockRect(target, { left: 0, top: 0, width: 10, height: 10 });
    document.body.appendChild(target);
    callbacks[1]?.(0);
    clearBeacon();
    await p;
    expect(host.querySelector('.guide-beacon')).toBeNull();
  });

  it('tracks capture scroll/resize with passive listeners and cleans them up on remove', async () => {
    const addSpy = vi.spyOn(window, 'addEventListener');
    const removeSpy = vi.spyOn(window, 'removeEventListener');
    const scroller = document.createElement('div');
    const target = document.createElement('button');
    target.id = 'move';
    scroller.appendChild(target);
    document.body.appendChild(scroller);
    let left = 20;
    const rectSpy = vi
      .spyOn(target, 'getBoundingClientRect')
      .mockImplementation(() => ({ left, top: 40, width: 80, height: 30 }) as DOMRect);

    await beacon('#move');
    expect(addSpy).toHaveBeenCalledWith('scroll', expect.any(Function), {
      capture: true,
      passive: true,
    });
    expect(addSpy).toHaveBeenCalledWith('resize', expect.any(Function), { passive: true });
    expect(addSpy).toHaveBeenCalledWith('animationend', expect.any(Function), {
      capture: true,
      passive: true,
    });
    const el = host.querySelector<HTMLElement>('.guide-beacon')!;
    expect(el.style.left).toBe('16px');

    left = 60;
    window.dispatchEvent(new Event('resize'));
    expect(el.style.left).toBe('56px');

    left = 90;
    scroller.dispatchEvent(new Event('scroll', { bubbles: false }));
    expect(el.style.left).toBe('86px');

    left = 110;
    scroller.dispatchEvent(new Event('animationend', { bubbles: false }));
    expect(el.style.left).toBe('106px');

    clearBeacon();
    expect(removeSpy).toHaveBeenCalledWith('scroll', expect.any(Function), true);
    expect(removeSpy).toHaveBeenCalledWith('animationend', expect.any(Function), true);
    rectSpy.mockClear();
    scroller.dispatchEvent(new Event('scroll', { bubbles: false }));
    scroller.dispatchEvent(new Event('animationend', { bubbles: false }));
    window.dispatchEvent(new Event('resize'));
    expect(rectSpy).not.toHaveBeenCalled();

    await beacon('#move');
    target.remove();
    window.dispatchEvent(new Event('scroll'));
    expect(host.querySelector('.guide-beacon')).toBeNull();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('dismisses once via capture click, focusin, auto-dismiss, or reduced motion without leaking timers or listeners', async () => {
    const target = document.createElement('button');
    const removeTargetSpy = vi.spyOn(target, 'removeEventListener');
    const child = document.createElement('span');
    child.addEventListener('click', (event) => event.stopPropagation());
    target.id = 'act';
    target.appendChild(child);
    mockRect(target, { left: 10, top: 10, width: 40, height: 20 });
    document.body.appendChild(target);

    await beacon('#act');
    expect(vi.getTimerCount()).toBe(1);
    const first = host.querySelector<HTMLElement>('.guide-beacon')!;
    child.click();
    expect(first.classList.contains('leaving')).toBe(true);
    expect(vi.getTimerCount()).toBe(1);
    target.dispatchEvent(new FocusEvent('focusin'));
    expect(vi.getTimerCount()).toBe(1);
    await vi.advanceTimersByTimeAsync(139);
    expect(host.querySelector('.guide-beacon')).not.toBeNull();
    await vi.advanceTimersByTimeAsync(1);
    expect(host.querySelector('.guide-beacon')).toBeNull();
    expect(vi.getTimerCount()).toBe(0);

    await beacon('#act');
    await vi.advanceTimersByTimeAsync(5999);
    expect(host.querySelector('.guide-beacon')?.classList.contains('leaving')).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(host.querySelector('.guide-beacon')?.classList.contains('leaving')).toBe(true);
    await vi.advanceTimersByTimeAsync(140);
    expect(host.querySelector('.guide-beacon')).toBeNull();

    await beacon('#act');
    target.dispatchEvent(new FocusEvent('focusin'));
    expect(host.querySelector('.guide-beacon')?.classList.contains('leaving')).toBe(true);
    expect(vi.getTimerCount()).toBe(1);
    clearBeacon();
    expect(host.querySelector('.guide-beacon')).toBeNull();
    expect(vi.getTimerCount()).toBe(0);

    await beacon('#act');
    expect(vi.getTimerCount()).toBe(1);
    clearBeacon();
    expect(vi.getTimerCount()).toBe(0);
    expect(removeTargetSpy).toHaveBeenCalledWith('click', expect.any(Function), true);
    child.click();
    target.dispatchEvent(new FocusEvent('focusin'));
    expect(vi.getTimerCount()).toBe(0);

    setReducedMotion(true);
    target.scrollIntoView = vi.fn();
    await beacon('#act');
    expect(target.scrollIntoView).toHaveBeenCalledWith({
      block: 'center',
      behavior: 'instant',
    });
    const reducedEl = host.querySelector<HTMLElement>('.guide-beacon')!;
    expect(reducedEl.dataset.reduced).toBe('true');
    expect(reducedEl.querySelector('.guide-beacon-pulse')).toBeNull();
    target.click();
    expect(host.querySelector('.guide-beacon')).toBeNull();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('skips hidden matches and polls until a matching target is visible with visibilityProperty check', async () => {
    const callbacks = new Map<number, FrameRequestCallback>();
    let nextId = 1;
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      const id = nextId++;
      callbacks.set(id, cb);
      return id;
    });
    vi.stubGlobal('cancelAnimationFrame', (id: number) => {
      callbacks.delete(id);
    });
    const flushFrame = () => {
      const entries = [...callbacks.entries()];
      callbacks.clear();
      for (const [, cb] of entries) cb(performance.now());
    };

    const hiddenOffscreen = document.createElement('button');
    hiddenOffscreen.className = 'step4-target';
    hiddenOffscreen.style.visibility = 'hidden';
    const visSpy = vi.spyOn(hiddenOffscreen, 'checkVisibility');
    mockRect(hiddenOffscreen, { left: 1000, top: 20, width: 80, height: 30 });

    const visibleToggle = document.createElement('button');
    visibleToggle.className = 'step4-target';
    visibleToggle.scrollIntoView = vi.fn();
    mockRect(visibleToggle, { left: 20, top: 12, width: 40, height: 32 });
    document.body.append(hiddenOffscreen, visibleToggle);

    await beacon('.step4-target');
    expect(visSpy).toHaveBeenCalledWith({ visibilityProperty: true });
    expect(host.querySelector<HTMLElement>('.guide-beacon')?.style.left).toBe('16px');
    clearBeacon();

    // Poll keeps waiting while match is hidden, then highlights once visible.
    visibleToggle.style.display = 'none';
    const pending = beacon('.step4-target');
    expect(callbacks.size).toBe(1);
    flushFrame();
    expect(host.querySelector('.guide-beacon')).toBeNull();
    expect(callbacks.size).toBe(1);

    visibleToggle.style.display = '';
    flushFrame();
    await pending;
    expect(host.querySelector<HTMLElement>('.guide-beacon')?.style.left).toBe('16px');
  });
});

describe('Welcome', () => {
  let mounted: ReturnType<typeof mount> | undefined;

  function setHash(hash: string) {
    location.hash = hash;
    dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
  }

  beforeEach(() => {
    localStorage.clear();
    document.body.innerHTML = '';
    prefs.lang = 'en';
    guideState.value = 'new';
    announcement.text = '';
    resetGuideUi();
    setHash('#/groups');
  });

  afterEach(async () => {
    if (mounted) {
      await unmount(mounted);
      mounted = undefined;
    }
    setHash('');
    resetGuideUi();
    guideState.value = 'new';
    prefs.lang = 'en';
    announcement.text = '';
    localStorage.clear();
    document.body.innerHTML = '';
    vi.restoreAllMocks();
  });

  it('switches language inside the open welcome dialog without closing it', async () => {
    const net: Net = { source: 'live', homes: [], path: () => undefined };
    mounted = mount(Welcome, { target: document.body, props: { net } });

    await vi.waitFor(() => {
      flushSync();
      expect(document.querySelector<HTMLDialogElement>('dialog.welcome')?.open).toBe(true);
    });

    const dlg = document.querySelector<HTMLDialogElement>('dialog.welcome')!;
    const acts = dlg.querySelector<HTMLElement>('footer.acts')!;
    const ghostButtons = [...acts.querySelectorAll<HTMLButtonElement>('button.btn-ghost')];
    expect(ghostButtons).toHaveLength(2);

    const langBtn = ghostButtons[1]!;
    expect(langBtn.querySelector<HTMLElement>('.swap')?.textContent).toBe('中');
    expect(langBtn.querySelector<HTMLElement>('.swap')?.getAttribute('lang')).toBe('zh-CN');
    expect(langBtn.querySelector('.sr span[lang="en"]')?.textContent).toBe('Switch language');
    expect(langBtn.querySelector('.sr span[lang="zh-CN"]')?.textContent).toBe('切换语言');

    langBtn.click();
    flushSync();

    expect(prefs.lang).toBe('zh-CN');
    expect(dlg.querySelector('h2')?.textContent).toBe('欢迎使用 SoloDawn');
    expect(dlg.open).toBe(true);
    expect(guideState.value).toBe('new');
    expect(langBtn.querySelector<HTMLElement>('.swap')?.textContent).toBe('EN');
    expect(langBtn.querySelector<HTMLElement>('.swap')?.getAttribute('lang')).toBe('en');

    langBtn.click();
    flushSync();

    expect(prefs.lang).toBe('en');
    expect(dlg.querySelector('h2')?.textContent).toBe('Welcome to SoloDawn');
    expect(dlg.open).toBe(true);
    expect(guideState.value).toBe('new');
    expect(langBtn.querySelector<HTMLElement>('.swap')?.textContent).toBe('中');
    expect(langBtn.querySelector<HTMLElement>('.swap')?.getAttribute('lang')).toBe('zh-CN');
  });
});
