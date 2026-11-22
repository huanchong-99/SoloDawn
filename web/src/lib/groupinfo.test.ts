import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  authorOf,
  forgetDisabledReason,
  forgetMemory,
  groupNotes,
  groupReviews,
  hasMemoryRow,
  MemoryView,
  previewText,
} from './groupinfo.svelte';
import { Group, Home } from './groups.svelte';
import { goldens } from './mock';
import { prefs } from './prefs.svelte';
import * as p from './proto';

const gid = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const meUser = 'alex-user';
const meMember = 'alex-member';

const sign = async (payload: string) => ({ p: payload, k: 'dev-k', s: 'dev-s' });

let seq = 500;
const nextSeq = () => (seq += 1);

function row<T extends p.Row = p.Row>(name: string, overrides: Record<string, unknown> = {}): T {
  const base = p.decode(p.Row, goldens[name]);
  return { ...base, seq: nextSeq(), ...overrides } as T;
}

function makeCtl(handler: (req: Record<string, unknown>) => Promise<unknown>): p.Ctl {
  return { request: handler as unknown as p.Ctl['request'] };
}

function makeGroup(
  opts: {
    role?: 'owner' | 'admin' | 'member' | 'none';
    online?: boolean;
    withInfo?: boolean;
    handler?: (req: Record<string, unknown>) => Promise<unknown>;
  } = {},
) {
  const { role = 'owner', online = true, withInfo = true, handler } = opts;
  const home = new Home({ user: meUser, sign });
  if (online) {
    home.ctl = makeCtl(handler ?? (async () => ({ re: 1, ok: true })));
  }
  const group = new Group(home, gid);
  const rows: p.Row[] = withInfo ? [row<p.GroupRow>('row-group', { id: gid })] : [];
  if (role !== 'none') {
    rows.push(
      row<p.HumanMemberRow>('row-member-human', {
        id: meMember,
        g: gid,
        user: meUser,
        name: 'Alex',
        role,
        online: true,
      }),
    );
  }
  group.apply(rows);
  return { home, group };
}

beforeEach(() => {
  prefs.lang = 'en';
  seq = 500;
});

describe('groupNotes, groupReviews and hasMemoryRow', () => {
  it('lists live context notes in pos order and omits gone markers and other rows', () => {
    const { group } = makeGroup();
    const n10 = row<p.NoteRow>('row-note', { id: 'n-10', pos: 10, text: 'First' });
    const n30 = row<p.NoteRow>('row-note', { id: 'n-30', pos: 30, text: 'Third' });
    const n20 = row<p.NoteRow>('row-note', { id: 'n-20', pos: 20, text: 'Second' });
    const msg = row<p.MsgRow>('row-msg-text', { id: 'm-1', pos: 5 });
    group.apply([n10, n30, n20, msg]);

    expect(groupNotes(group).map((n) => n.id)).toEqual(['n-10', 'n-20', 'n-30']);
    expect(hasMemoryRow(group, 'note', 'n-20')).toBe(true);
    expect(hasMemoryRow(group, 'note', 'missing')).toBe(false);

    group.apply([{ r: 'note', id: 'n-20', seq: nextSeq(), gone: true }]);
    expect(groupNotes(group).map((n) => n.id)).toEqual(['n-10', 'n-30']);
    expect(hasMemoryRow(group, 'note', 'n-20')).toBe(false);
  });

  it('lists live review messages in pos order and omits gone markers and non-review messages', () => {
    const { group } = makeGroup();
    const r15 = row<p.MsgRow>('row-msg-review', { id: 'r-15', pos: 15 });
    const r40 = row<p.MsgRow>('row-msg-review', { id: 'r-40', pos: 40 });
    const r25 = row<p.MsgRow>('row-msg-review', { id: 'r-25', pos: 25 });
    const txt = row<p.MsgRow>('row-msg-text', { id: 'm-txt', pos: 20 });
    const note = row<p.NoteRow>('row-note', { id: 'n-1', pos: 35 });
    group.apply([r15, r40, r25, txt, note]);

    expect(groupReviews(group).map((r) => r.id)).toEqual(['r-15', 'r-25', 'r-40']);
    expect(hasMemoryRow(group, 'msg', 'r-15')).toBe(true);

    group.apply([{ r: 'msg', id: 'r-15', seq: nextSeq(), gone: true }]);
    expect(groupReviews(group).map((r) => r.id)).toEqual(['r-25', 'r-40']);
    expect(hasMemoryRow(group, 'msg', 'r-15')).toBe(false);
  });
});

describe('authorOf, forgetDisabledReason, previewText and forgetMemory', () => {
  it('resolves authorOf for present and departed members', () => {
    const { group } = makeGroup();
    expect(authorOf(group, meMember)).toBe('Alex');
    expect(authorOf(group, 'departed-id')).toBe('Former member');
  });

  it('checks forgetDisabledReason for offline, loading, non-admin and admin roles', () => {
    expect(forgetDisabledReason(makeGroup({ online: false }).group)).toBe(
      'Cannot reach the group home.',
    );
    expect(
      forgetDisabledReason(makeGroup({ online: true, withInfo: false, role: 'member' }).group),
    ).toBeUndefined();
    expect(forgetDisabledReason(makeGroup({ role: 'member' }).group)).toBe(
      'Only an admin can delete project memory.',
    );
    expect(forgetDisabledReason(makeGroup({ role: 'admin' }).group)).toBeUndefined();
    expect(forgetDisabledReason(makeGroup({ role: 'owner' }).group)).toBeUndefined();
  });

  it('strips backticks, collapses whitespace and truncates previewText strictly above 60 characters', () => {
    expect(previewText('  Keep `rtt_ms`\n\tempty   on `relay`.  ')).toBe(
      'Keep rtt_ms empty on relay.',
    );
    const exact60 = 'a'.repeat(60);
    expect(previewText(exact60)).toBe(exact60);
    expect(previewText(exact60)).toHaveLength(60);
    const len61 = `${'b'.repeat(59)}xy`;
    expect(previewText(len61)).toBe(`${'b'.repeat(59)}…`);
    expect(previewText(len61)).toHaveLength(60);
  });

  it('signs forget in forgetMemory and returns refusal codes or empty string on ok', async () => {
    const sent: unknown[] = [];
    vi.spyOn(Date, 'now').mockReturnValue(1763600000500);
    const { home, group } = makeGroup({
      handler: async (req) => {
        sent.push(req);
        return { re: 1, ok: true };
      },
    });
    const noteId = '7bRlYkKR5AU8bF6kt-syDQ';
    expect(await forgetMemory(group, 'note', noteId)).toBe('');
    expect(sent).toHaveLength(1);
    const frame = sent[0] as { t: string; env: p.Envelope };
    expect(frame.t).toBe('cmd');
    expect(frame.env.k).toBe('dev-k');
    expect(JSON.parse(frame.env.p)).toEqual({
      t: 'forget',
      n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
      ts: 1763600000500,
      g: gid,
      r: 'note',
      id: noteId,
    });

    home.ctl = makeCtl(async () => ({ re: 2, err: { code: 'forbidden', msg: 'not admin' } }));
    expect(await forgetMemory(group, 'msg', noteId)).toBe('forbidden');

    home.ctl = undefined;
    expect(await forgetMemory(group, 'note', noteId)).toBe('lost');
    vi.restoreAllMocks();
  });
});

describe('MemoryView paging', () => {
  it('syncs once per online epoch, pages older entries without moving group.cursor, handles empty pages, and re-syncs after group.sync', async () => {
    const asked: Record<string, unknown>[] = [];
    const n3 = row<p.NoteRow>('row-note', { id: 'n-3', seq: 900, pos: 90, text: 'Newest' });
    const n2 = row<p.NoteRow>('row-note', { id: 'n-2', seq: 901, pos: 80, text: 'Middle' });
    const n1 = row<p.NoteRow>('row-note', { id: 'n-1', seq: 902, pos: 60, text: 'Older' });
    const n0 = row<p.NoteRow>('row-note', { id: 'n-0', seq: 903, pos: 40, text: 'Oldest' });
    const rev = row<p.MsgRow>('row-msg-review', { id: 'rev-1', seq: 904, pos: 70 });

    const { home, group } = makeGroup({ online: false });
    const view = new MemoryView(group);

    // Offline sync does nothing and leaves epoch unset, so coming online in the same epoch still fetches.
    view.sync();
    await view.older('note');
    expect(asked).toEqual([]);
    expect(view.note.more).toBe(true);

    const notePages = new Map<number, p.NoteRow[]>([
      [0, [n3, n2, n1]],
      [60, [n0]],
    ]);
    const ctl = makeCtl(async (req) => {
      asked.push(req);
      if (req.t === 'sync') return { t: 'synced', re: 1, ok: true, g: gid, seq: 600 };
      if (req.kind === 'note') {
        const rows = notePages.get(req.before as number) ?? [];
        return { t: 'fetched', re: 2, ok: true, rows, more: true };
      }
      return { t: 'fetched', re: 3, ok: true, rows: [rev], more: false };
    });
    home.ctl = ctl;
    view.sync();
    await vi.waitFor(() => expect(asked).toHaveLength(2));

    await group.sync(ctl);
    expect(group.cursor).toBe(600);
    asked.length = 0;

    view.sync();
    // Repeated sync in the same epoch does not re-fetch.
    view.sync();
    await vi.waitFor(() => expect(view.review.more).toBe(false));
    await vi.waitFor(() => expect(view.note.before).toBe(60));

    expect(asked).toEqual([
      { t: 'fetch', g: gid, kind: 'note', before: 0, limit: 50 },
      { t: 'fetch', g: gid, kind: 'review', before: 0, limit: 50 },
    ]);
    // Fetched rows must not advance the live sync cursor.
    expect(group.cursor).toBe(600);
    expect(groupNotes(group).map((n) => n.id)).toEqual(['n-1', 'n-2', 'n-3']);
    expect(groupReviews(group).map((r) => r.id)).toEqual(['rev-1']);

    await view.older('note');
    expect(groupNotes(group).map((n) => n.id)).toEqual(['n-0', 'n-1', 'n-2', 'n-3']);
    expect(view.note.more).toBe(true);
    expect(view.note.before).toBe(40);
    expect(group.cursor).toBe(600);

    // An empty page ends paging even if more was reported true, keeping the last before cursor.
    await view.older('note');
    expect(view.note.more).toBe(false);
    expect(view.note.before).toBe(40);

    // Calling older when ready is false sends nothing.
    const countBefore = asked.length;
    await view.older('note');
    expect(asked).toHaveLength(countBefore);

    // A new group.sync advances the paging epoch so view.sync reads fresh pages.
    await group.sync(ctl);
    asked.length = 0;
    view.sync();
    await vi.waitFor(() => expect(asked).toHaveLength(2));
  });

  it('records failed pages on refusal or rejection and drops stale answers across ctl or epoch changes', async () => {
    const { group } = makeGroup({
      handler: async (req) => {
        if (req.kind === 'note') return { re: 1, err: { code: 'forbidden', msg: 'no' } };
        throw new Error('dropped');
      },
    });
    const view = new MemoryView(group);
    await view.older('note');
    await view.older('review');
    expect(view.note.busy).toBe(false);
    expect(view.note.failed).toBe(true);
    expect(view.review.busy).toBe(false);
    expect(view.review.failed).toBe(true);

    // Stale case 1: home.ctl changes while fetch is in flight.
    let resolveCtlChange!: (v: unknown) => void;
    const { home: h1, group: g1 } = makeGroup({
      handler: () => new Promise((r) => (resolveCtlChange = r)),
    });
    const v1 = new MemoryView(g1);
    const p1 = v1.older('note');
    expect(v1.note.busy).toBe(true);
    h1.ctl = undefined;
    resolveCtlChange({
      t: 'fetched',
      re: 1,
      ok: true,
      rows: [row<p.NoteRow>('row-note', { id: 'late-ctl' })],
      more: false,
    });
    await p1;
    expect(v1.note.busy).toBe(false);
    expect(v1.note.more).toBe(true);
    expect(groupNotes(g1)).toEqual([]);

    // Stale case 2: same ctl stays connected, but group.sync finishes and advances epoch mid-flight.
    let resolveEpochChange!: (v: unknown) => void;
    const { home: h2, group: g2 } = makeGroup();
    const sharedCtl = makeCtl((req) => {
      if (req.t === 'sync')
        return Promise.resolve({ t: 'synced', re: 2, ok: true, g: gid, seq: 9 });
      return new Promise((r) => (resolveEpochChange = r));
    });
    h2.ctl = sharedCtl;
    const v2 = new MemoryView(g2);
    const p2 = v2.older('note');
    await g2.sync(sharedCtl);
    resolveEpochChange({
      t: 'fetched',
      re: 3,
      ok: true,
      rows: [row<p.NoteRow>('row-note', { id: 'late-epoch' })],
      more: false,
    });
    await p2;
    expect(v2.note.more).toBe(true);
    expect(groupNotes(g2)).toEqual([]);
  });
});
