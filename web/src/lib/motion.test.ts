import { readFileSync } from 'node:fs';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const tokens = readFileSync('src/styles/tokens.css', 'utf8');

let reducedMotion: boolean;

/** A page load, so that the system's motion setting is read anew. */
async function load() {
  vi.resetModules();
  return import('./motion');
}

beforeEach(() => {
  reducedMotion = false;
  vi.stubGlobal('matchMedia', (query: string) => ({
    get matches() {
      return query === '(prefers-reduced-motion: reduce)' && reducedMotion;
    },
  }));
});
afterEach(() => {
  vi.unstubAllGlobals();
  Reflect.deleteProperty(document, 'startViewTransition');
  delete document.documentElement.dataset.live;
});

describe('spring', () => {
  it('starts at 0, overshoots and approaches 1', async () => {
    const { spring } = await load();
    expect(spring(0)).toBe(0);
    expect(spring(0.25).toFixed(6)).toBe('0.933259');
    expect(spring(0.4).toFixed(6)).toBe('1.043072');
    expect(spring(0.5).toFixed(6)).toBe('1.025833');
    expect(spring(1).toFixed(6)).toBe('0.999717');
  });

  it('is the --spring of tokens.css, a sample every 30th of the way', async () => {
    const { spring } = await load();
    const stops = /--spring: linear\(([^)]+)\)/.exec(tokens)![1].split(',').map(Number);
    expect(stops).toEqual(Array.from({ length: 31 }, (_, i) => +spring(i / 30).toFixed(3)));
  });
});

describe('shift', () => {
  /** View transitions that run their update, and end, when the test says so. */
  const viewTransitions = () => {
    const ends: (() => void)[] = [];
    const start = vi.fn((_update: () => void) => ({
      finished: new Promise<void>((end) => ends.push(end)),
    }));
    Object.assign(document, { startViewTransition: start });
    return { start, end: () => ends.shift()!() };
  };
  const live = () => document.documentElement.dataset.live;

  it('hands the change to a view transition of the document', async () => {
    const { shift } = await load();
    const { start } = viewTransitions();
    const change = vi.fn();
    shift(change);
    expect(start).toHaveBeenCalledOnce();
    expect(start.mock.contexts).toEqual([document]);
    expect(change).not.toHaveBeenCalled();
    start.mock.calls[0][0]();
    expect(change.mock.calls).toEqual([[]]);
  });

  it('marks the new page live from the change to the end of the fade', async () => {
    const { shift } = await load();
    const { start, end } = viewTransitions();
    shift(() => expect(live()).toBeUndefined());
    expect(live()).toBeUndefined();
    start.mock.calls[0][0]();
    expect(live()).toBe('');
    await Promise.resolve();
    expect(live()).toBe('');
    end();
    await Promise.resolve();
    expect(live()).toBeUndefined();
  });

  it('makes the change at once in a browser without view transitions', async () => {
    const { shift } = await load();
    const change = vi.fn();
    shift(change);
    expect(change).toHaveBeenCalledOnce();
  });

  it('makes the change at once when the system asks for reduced motion', async () => {
    const { shift } = await load();
    const { start } = viewTransitions();
    const change = vi.fn();
    reducedMotion = true;
    shift(change);
    expect(change.mock.calls).toEqual([[]]);
    expect(start.mock.calls).toEqual([]);
    expect(live()).toBeUndefined();
    reducedMotion = false;
    shift(change);
    expect(start).toHaveBeenCalledOnce();
    expect(change.mock.calls).toEqual([[]]);
  });
});
