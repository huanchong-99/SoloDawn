import { flushSync } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Group, Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { decode, Row, type Ctl, type NoteRow, type HumanMemberRow } from '../../lib/proto';
import { canWrite, presets, PresetView, savePreset, validName } from './presets.svelte';

const row = (name: string, overrides: object = {}) =>
  ({ ...decode(Row, goldens[name]), ...overrides }) as Row;
let group: Group;
let request: ReturnType<typeof vi.fn>;

beforeEach(() => {
  const owner = decode(Row, goldens['row-member-human']) as HumanMemberRow;
  const home = new Home({ user: owner.user, sign: async (p) => ({ p, k: 'key', s: 'signature' }) });
  group = new Group(home, owner.g);
  group.apply([owner]);
  request = vi.fn(async () => ({ re: 1, ok: true, rows: [], more: false }));
  home.ctl = { request: request as Ctl['request'] };
});

describe('preset projections and writing', () => {
  it('lists only live presets in creation order', () => {
    expect(presets(group)).toEqual([]);
    const first = row('row-note-preset', { id: 'first', pos: 1 });
    const last = row('row-note-preset', { id: 'last', pos: 2 });
    group.apply([
      last,
      first,
      row('row-note'),
      row('row-note-preset', { id: 'kb', type: 'kb' }),
      { r: 'note', id: 'gone', seq: 1, gone: true },
    ]);
    expect(presets(group)).toEqual([first, last]);
  });
  it('allows only online human members with a connection to write', () => {
    expect(canWrite(group)).toBe(true);
    const owner = group.me!;
    group.apply([{ ...owner, online: false, seq: owner.seq + 1 }]);
    expect(canWrite(group)).toBe(false);
    group.apply([{ r: 'member', id: owner.id, seq: owner.seq + 2, gone: true }]);
    expect(canWrite(group)).toBe(false);
    group.home.ctl = undefined;
    expect(canWrite(group)).toBe(false);
  });
  it('validates exact names including Unicode character counts and controls', () => {
    expect(
      ['', 'x', 'x'.repeat(80), 'x'.repeat(81), '😀'.repeat(80), 'a\nb', 'a\u007fb'].map(validName),
    ).toEqual([false, true, true, false, true, false, false]);
  });
  it('sends a new id on each write and returns refusal codes', async () => {
    expect(await savePreset(group, 'Calm', 'Text')).toBe('');
    const cmd = JSON.parse(request.mock.calls[0][0].env.p);
    expect(cmd).toEqual({
      t: 'note',
      n: expect.any(String),
      ts: expect.any(Number),
      g: group.g,
      id: expect.stringMatching(/^[A-Za-z0-9_-]{22}$/),
      task: null,
      type: 'preset',
      name: 'Calm',
      text: 'Text',
    });
    request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden' } });
    expect(await savePreset(group, 'Calm', '')).toBe('forbidden');
    expect(JSON.parse(request.mock.calls[1][0].env.p).id).not.toBe(cmd.id);
  });
});

describe('preset fetch lifecycle', () => {
  it('fetches once per epoch and applies the first page', async () => {
    const note = row('row-note-preset') as NoteRow;
    request.mockResolvedValue({ re: 1, ok: true, rows: [note], more: true });
    const view = new PresetView(group);
    view.sync();
    view.sync();
    await vi.waitFor(() => expect(view.loaded).toBe(true));
    expect(request.mock.calls).toEqual([
      [{ t: 'fetch', g: group.g, kind: 'preset', before: 0, limit: 50 }],
    ]);
    expect(presets(group)).toEqual([note]);
    expect(view.busy).toBe(false);
    expect(view.failed).toBe('');
    await group.sync(group.home.ctl!);
    flushSync();
    view.sync();
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(3));
  });
  it('does not fetch offline and accepts an empty result', async () => {
    const view = new PresetView(group);
    const ctl = group.home.ctl;
    group.home.ctl = undefined;
    view.sync();
    await view.fetch();
    expect(request).not.toHaveBeenCalled();
    expect(view.loaded).toBe(false);
    group.home.ctl = ctl;
    await view.fetch();
    expect(view.loaded).toBe(true);
    expect(presets(group)).toEqual([]);
  });
  it('shows refusal and lost connection errors', async () => {
    const view = new PresetView(group);
    request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden' } });
    await view.fetch();
    expect(view.failed).toBe('forbidden');
    expect(view.loaded).toBe(false);
    request.mockRejectedValueOnce(new Error('lost'));
    await view.fetch();
    expect(view.failed).toBe('lost');
    expect(view.busy).toBe(false);
  });
  it.each(['connection', 'epoch'])('discards stale %s responses', async (stale) => {
    let done!: (value: unknown) => void;
    request.mockImplementation(
      () =>
        new Promise((resolve) => {
          done = resolve;
        }),
    );
    const view = new PresetView(group);
    const pending = view.fetch();
    if (stale === 'connection') group.home.ctl = undefined;
    else {
      request.mockResolvedValue({ re: 1, ok: true, rows: [], more: false });
      await group.sync(group.home.ctl!);
    }
    done({ re: 1, ok: true, rows: [row('row-note-preset')], more: false });
    await pending;
    expect(view.loaded).toBe(false);
    expect(presets(group)).toEqual([]);
  });
});
