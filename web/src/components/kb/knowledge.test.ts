import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Group, Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { decode, Row, type Ctl, type HumanMemberRow, type NoteRow } from '../../lib/proto';
import { Knowledge } from './knowledge.svelte';

const row = (id: string, pos = 1) =>
  ({ ...decode(Row, goldens['row-note-kb']), id, pos }) as NoteRow;
let group: Group;
let view: Knowledge;
let request: ReturnType<typeof vi.fn>;

beforeEach(() => {
  const owner = decode(Row, goldens['row-member-human']) as HumanMemberRow;
  const home = new Home({ user: owner.user, sign: async (p) => ({ p, k: 'key', s: 'signature' }) });
  request = vi.fn(async () => ({ re: 1, ok: true, rows: [], more: false }));
  home.ctl = { request: request as Ctl['request'] };
  group = new Group(home, owner.g);
  view = new Knowledge(group);
});

describe('documents and list paging', () => {
  it('lists only live kb notes in position order', () => {
    expect(view.documents).toEqual([]);
    expect(view.failed).toBe('');
    const a = row('a', 1);
    const b = row('b', 2);
    group.apply([
      b,
      a,
      { ...row('context'), type: undefined },
      { ...row('preset'), type: 'preset' },
      { r: 'note', id: 'gone', seq: 1, gone: true },
      decode(Row, goldens['row-group']),
    ]);
    expect(view.documents).toEqual([a, b]);
  });
  it('fetches once per online epoch and restarts paging after sync', async () => {
    group.home.ctl = undefined;
    view.sync();
    expect(request).not.toHaveBeenCalled();
    group.home.ctl = { request: request as Ctl['request'] };
    request.mockResolvedValueOnce({ re: 1, ok: true, rows: [row('b', 2)], more: true });
    view.sync();
    view.sync();
    await vi.waitFor(() => expect(view.loaded).toBe(true));
    expect(group.cursor).toBe(0);
    expect(request.mock.calls).toEqual([
      [{ t: 'fetch', g: group.g, kind: 'kb', before: 0, limit: 50 }],
    ]);
    request.mockResolvedValueOnce({ re: 1, ok: true, rows: [row('a', 1)], more: false });
    await view.older();
    await view.older();
    expect(request.mock.calls[1][0]).toEqual({
      t: 'fetch',
      g: group.g,
      kind: 'kb',
      before: 2,
      limit: 50,
    });
    expect(view.documents.map((row) => row.id)).toEqual(['a', 'b']);
    expect(view.more).toBe(false);
    await group.sync(group.home.ctl);
    view.sync();
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(4));
    expect(request.mock.calls[3][0]).toEqual({
      t: 'fetch',
      g: group.g,
      kind: 'kb',
      before: 0,
      limit: 50,
    });
  });
  it('ends pagination on an empty page even when more is true', async () => {
    request.mockResolvedValue({ re: 1, ok: true, rows: [], more: true });
    await view.older();
    expect(view.loaded).toBe(true);
    expect(view.more).toBe(false);
    expect(view.documents).toEqual([]);
  });
  it('returns to the list and clears a failed search on reconnect', async () => {
    request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden' } });
    await view.search('writer');
    expect(view.failed).toBe('forbidden');
    await group.sync(group.home.ctl!);
    view.sync();
    expect(view.ids).toBeUndefined();
    expect(view.failed).toBe('');
    await vi.waitFor(() => expect(view.loaded).toBe(true));
  });
  it('does not send when offline or while a page is busy', async () => {
    group.home.ctl = undefined;
    await view.older();
    expect(request).not.toHaveBeenCalled();
    view.busy = true;
    await view.older();
    expect(request).not.toHaveBeenCalled();
  });
});

describe('ranked search', () => {
  it('applies results, preserves response order and drops gone or absent documents', async () => {
    const a = row('a', 1);
    const b = row('b', 2);
    request.mockResolvedValueOnce({ re: 1, ok: true, rows: [b, a], more: true });
    await view.search('writer');
    expect(request.mock.calls[0][0]).toEqual({
      t: 'fetch',
      g: group.g,
      kind: 'kb',
      before: 0,
      limit: 20,
      q: 'writer',
    });
    expect(view.documents).toEqual([b, a]);
    expect(view.narrow).toBe(true);
    group.apply([{ r: 'note', id: 'b', seq: b.seq + 1, gone: true }]);
    expect(view.documents).toEqual([a]);
    group.rows.delete('note a');
    expect(view.documents).toEqual([]);
    view.clear();
    expect(view.ids).toBeUndefined();
    expect(view.narrow).toBe(false);
  });
  it('represents no match as an empty result, and clearing restores the cache list', async () => {
    const a = row('a');
    group.apply([a]);
    await view.search('absent');
    expect(view.ids).toEqual([]);
    expect(view.documents).toEqual([]);
    expect(view.narrow).toBe(false);
    view.clear();
    expect(view.documents).toEqual([a]);
  });
  it('shows refusal and connection loss without applying results', async () => {
    request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden' } });
    await view.search('writer');
    expect(view.failed).toBe('forbidden');
    expect(view.documents).toEqual([]);
    request.mockRejectedValueOnce(new Error('lost'));
    await view.older();
    expect(view.failed).toBe('lost');
    expect(view.loaded).toBe(false);
    expect(view.busy).toBe(false);
    view.clear();
    expect(view.failed).toBe('');
  });
  it.each(['connection', 'epoch', 'clear'])('drops an answer after %s changes', async (change) => {
    let done!: (value: unknown) => void;
    request.mockImplementation(
      () =>
        new Promise((resolve) => {
          done = resolve;
        }),
    );
    const pending = view.search('writer');
    expect(view.ids).toEqual([]);
    if (change === 'connection') group.home.ctl = undefined;
    if (change === 'epoch') {
      request.mockResolvedValue({ re: 1, ok: true, seq: 1 });
      await group.sync(group.home.ctl!);
    }
    if (change === 'clear') view.clear();
    done({ re: 1, ok: true, rows: [row('late')], more: true });
    await pending;
    expect(view.documents).toEqual([]);
    expect(view.narrow).toBe(false);
    expect(view.busy).toBe(false);
  });
});
