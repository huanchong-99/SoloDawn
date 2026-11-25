import { beforeEach, describe, expect, it } from 'vitest';
import { confirmSentence } from './confirm';
import { Group, Home } from './groups.svelte';
import { goldens } from './mock';
import { prefs } from './prefs.svelte';
import * as p from './proto';

type Confirm = Extract<p.MsgRow, { kind: 'confirm' }>;

const row = (name: string, changes: object = {}) =>
  ({ ...p.decode(p.Row, goldens[name]), ...changes }) as p.Row;

const card = (what: string, changes: object = {}) =>
  row('row-msg-confirm-pending', {
    by: 'system',
    re: 'request',
    body: { what, ai: 'codex', to: ['me'] },
    ...changes,
  }) as Confirm;

/** A group with Pat, Codex and the secretary, and what else is given. */
function groupWith(...rows: p.Row[]) {
  const group = new Group(new Home({ user: 'me', sign: () => new Promise(() => {}) }), 'g');
  group.apply([
    row('row-member-human', { id: 'pat', user: 'pat-user', name: 'Pat' }),
    row('row-member-ai', { id: 'codex', name: 'Codex' }),
    row('row-member-ai', { id: 'secretary', name: 'Secretary' }),
    ...rows,
  ]);
  return group;
}

describe('the sentence of a confirm card', () => {
  beforeEach(() => {
    prefs.lang = 'en';
  });

  it('names the AI and the author of the request', () => {
    const group = groupWith(row('row-msg-text', { id: 'request', by: 'pat' }));
    expect(confirmSentence(group, card('direct'))).toBe('Let Codex act on the request from Pat.');
    expect(confirmSentence(group, card('ask'))).toBe('Let Codex answer the call from Pat.');
    prefs.lang = 'zh-CN';
    expect(confirmSentence(group, card('direct'))).toBe('允许 Codex 按 Pat 的请求行动。');
  });

  it('names the person who posted a watch card', () => {
    expect(confirmSentence(groupWith(), card('watch', { by: 'pat', re: null }))).toBe(
      'Let Pat watch Codex’s terminal.',
    );
  });

  it('names the person who confirmed a brief, and the author of a brief not confirmed', () => {
    const at = 1763600000000;
    const brief = (name: string, changes: object = {}) =>
      groupWith(row(name, { id: 'request', by: 'secretary', ...changes }));
    const confirmed = brief('row-msg-brief-confirmed', {
      state: { status: 'confirmed', by: 'pat', at, act: { p: '', k: '', s: '' } },
    });
    expect(confirmSentence(confirmed, card('direct'))).toBe(
      'Let Codex act on the request from Pat.',
    );
    expect(confirmSentence(brief('row-msg-brief-pending'), card('direct'))).toBe(
      'Let Codex act on the request from Secretary.',
    );
  });

  it('names the task when the request is the message of a task without a brief', () => {
    const task = row('row-task', { id: 'request', num: 12, title: "Cost $& $'" });
    const message = row('row-msg-task', { id: 'request', body: { task: 'request' } });
    for (const group of [groupWith(task, message), groupWith(task)]) {
      prefs.lang = 'en';
      expect(confirmSentence(group, card('direct'))).toBe(
        "Let Codex take part in T-12 “Cost $& $'”.",
      );
      prefs.lang = 'zh-CN';
      expect(confirmSentence(group, card('direct'))).toBe("允许 Codex 参与 T-12「Cost $& $'」。");
    }
  });

  it('says when who asked is not known, and when a member has left', () => {
    const unknown = 'Let Codex act on the request from an unknown requester.';
    expect(confirmSentence(groupWith(), card('direct'))).toBe(unknown);
    const removed = groupWith({ r: 'msg', id: 'request', seq: 900, gone: true });
    expect(confirmSentence(removed, card('direct'))).toBe(unknown);
    const left = groupWith(row('row-msg-text', { id: 'request', by: 'mina' }));
    expect(
      confirmSentence(left, card('direct', { body: { what: 'direct', ai: 'old', to: ['me'] } })),
    ).toBe('Let Former member act on the request from Former member.');
  });

  it('takes a name as it is written', () => {
    const group = groupWith(
      row('row-member-human', { id: 'sam', user: 'sam-user', name: "$& $'" }),
      row('row-member-ai', { id: 'codex', name: '$&', seq: 900 }),
      row('row-msg-text', { id: 'request', by: 'sam' }),
    );
    expect(confirmSentence(group, card('direct'))).toBe("Let $& act on the request from $& $'.");
  });
});
