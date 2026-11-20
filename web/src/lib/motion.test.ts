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
    matches: query === '(prefers-reduced-motion: reduce)' && reducedMotion,
  }));
});
afterEach(() => {
  Reflect.deleteProperty(document, 'startViewTransition');
});

describe('spring', () => {
  it('goes from 0 past 1 once and comes to rest at 1', async () => {
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
  const viewTransitions = () => {
    const start = vi.fn();
    Object.assign(document, { startViewTransition: start });
    return start;
  };

  it('hands the change to a view transition of the document', async () => {
    const { shift } = await load();
    const start = viewTransitions();
    const change = vi.fn();
    shift(change);
    expect(start.mock.calls).toEqual([[change]]);
    expect(start.mock.contexts).toEqual([document]);
    expect(change).not.toHaveBeenCalled();
  });

  it('makes the change at once in a browser without view transitions', async () => {
    const { shift } = await load();
    const change = vi.fn();
    shift(change);
    expect(change).toHaveBeenCalledOnce();
  });

  it('makes the change at once when the system asks for reduced motion', async () => {
    reducedMotion = true;
    const { shift } = await load();
    const start = viewTransitions();
    const change = vi.fn();
    shift(change);
    expect(change).toHaveBeenCalledOnce();
    expect(start).not.toHaveBeenCalled();
  });
});
