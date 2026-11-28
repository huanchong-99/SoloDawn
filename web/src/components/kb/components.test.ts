import { flushSync, tick } from 'svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { Group, Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { prefs } from '../../lib/prefs.svelte';
import { decode, Row, type Ctl, type HumanMemberRow } from '../../lib/proto';
import { all, closePage, one, show, texts } from '../../../features/page';
import KnowledgePanel from './KnowledgePanel.svelte';

let group: Group;
let request: ReturnType<typeof vi.fn>;

beforeEach(() => {
  prefs.lang = 'en';
  const owner = decode(Row, goldens['row-member-human']) as HumanMemberRow;
  const home = new Home({ user: owner.user, sign: async (p) => ({ p, k: 'key', s: 'signature' }) });
  group = new Group(home, owner.g);
  group.apply([owner, decode(Row, goldens['row-group'])]);
  request = vi.fn(async () => ({ re: 1, ok: true, rows: [], more: false }));
  home.ctl = { request: request as Ctl['request'] };
});
afterEach(closePage);

async function settle() {
  await tick();
  await vi.waitFor(() => {
    flushSync();
    expect(all('.kb [role=status]')).toEqual([]);
  });
}

async function open() {
  one<HTMLDetailsElement>('.kb details').open = true;
  one('.kb details').dispatchEvent(new Event('toggle'));
  await settle();
}

async function search(value: string) {
  const input = one<HTMLInputElement>('.kb input');
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
  one('.kb form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await settle();
}

it('shows the empty-list hint and disables white-space searches', async () => {
  show(KnowledgePanel, { group });
  await open();
  expect(texts('.kb .hint')).toEqual(['No documents yet.']);
  expect(texts('.kb label .sr')).toEqual(['Search knowledge base']);
  expect(all('.kb form .btn-ghost')).toEqual([]);
  expect(one<HTMLButtonElement>('.kb form button').disabled).toBe(true);
  const input = one<HTMLInputElement>('.kb input');
  input.value = ' \t ';
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
  expect(one<HTMLButtonElement>('.kb form button').disabled).toBe(true);
  const list = one('.kb ul');
  expect(document.getElementById(list.getAttribute('aria-labelledby')!)?.textContent).toBe(
    'Knowledge base',
  );
});

it('shows fetch refusals and keeps admin-only deletion disabled for other members', async () => {
  request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden' } });
  const member = group.me!;
  group.apply([
    { ...member, role: 'member', seq: member.seq + 1 },
    decode(Row, goldens['row-note-kb']),
  ]);
  show(KnowledgePanel, { group });
  await open();
  expect(texts('.kb .hint')).toEqual([]);
  expect(texts('.kb [role=alert]')).toEqual(['Not sent: this device may not do that.']);
  expect(one<HTMLButtonElement>('.kb .del').disabled).toBe(true);
});

it('disables paging offline while cached documents stay visible', async () => {
  group.apply([decode(Row, goldens['row-note-kb'])]);
  group.home.ctl = undefined;
  show(KnowledgePanel, { group });
  await open();
  expect(one<HTMLButtonElement>('.kb .older').disabled).toBe(true);
  expect(texts('.kb .hint')).toEqual(['Cannot reach the group home. Documents are read-only.']);
});

it('supports list, match, no-match and deletion with a cancellable confirmation', async () => {
  const note = decode(Row, goldens['row-note-kb']);
  group.apply([note]);
  show(KnowledgePanel, { group });
  await open();
  expect(texts('.kb .mem-item > b')).toEqual(['Decision: the home is the single writer']);
  await search('absent');
  expect(texts('.kb .hint')).toEqual(['No matching documents.']);
  expect(texts('.kb form .btn-ghost')).toEqual(['Clear search']);
  request.mockResolvedValueOnce({ re: 1, ok: true, rows: [note], more: false });
  await search('writer');
  expect(texts('.kb .mem-item > b')).toEqual(['Decision: the home is the single writer']);
  one<HTMLButtonElement>('.kb .del').click();
  flushSync();
  request.mockImplementationOnce(async () => {
    group.apply([{ r: 'note', id: note.id, seq: note.seq + 1, gone: true }]);
    return { re: 1, ok: true };
  });
  one<HTMLButtonElement>('dialog .btn-ghost').click();
  await tick();
  flushSync();
  expect(all('dialog')).toEqual([]);
  one<HTMLButtonElement>('.kb .del').click();
  flushSync();
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await vi.waitFor(() => {
    flushSync();
    expect(all('dialog')).toEqual([]);
  });
  expect(texts('.kb .hint')).toEqual(['No matching documents.']);
  expect(document.activeElement).toBe(one('.kb summary'));
  await search('writer');
  expect(texts('.kb .mem-item > b')).toEqual([]);
});

it('shows field placeholders, groups row actions, and distinguishes document delete wording', async () => {
  group.apply([decode(Row, goldens['row-note-kb'])]);
  show(KnowledgePanel, { group });
  await open();

  const editBtn = one<HTMLButtonElement>('.kb .mem-meta .edit');
  const delBtn = one<HTMLButtonElement>('.kb .mem-meta .del');
  expect(editBtn.parentElement).toBe(delBtn.parentElement);
  expect(editBtn.parentElement?.classList.contains('mem-acts')).toBe(true);
  expect(delBtn.getAttribute('aria-label')).toBe(
    'Delete document: Decision: the home is the single writer',
  );

  editBtn.click();
  flushSync();
  expect(one<HTMLInputElement>('.kb .edit-field input').placeholder).toBe('Document name');
  expect(one<HTMLTextAreaElement>('.kb .edit-field textarea').placeholder).toBe('Document text');
  one<HTMLButtonElement>('.kb .edit-field .btn-ghost').click();
  await tick();
  flushSync();

  one<HTMLButtonElement>('.kb .add').click();
  flushSync();
  expect(one<HTMLInputElement>('.kb .edit-field input').placeholder).toBe('Document name');
  expect(one<HTMLTextAreaElement>('.kb .edit-field textarea').placeholder).toBe('Document text');

  delBtn.click();
  flushSync();
  expect(texts('dialog h2')).toEqual(['Delete this document?']);
  expect(texts('dialog .lead')).toEqual([
    "This removes it permanently from the group's knowledge base on every connected device.",
  ]);
  one<HTMLButtonElement>('dialog .btn-ghost').click();
  await tick();
  flushSync();

  prefs.lang = 'zh-CN';
  flushSync();
  expect(one<HTMLButtonElement>('.kb .mem-meta .del').getAttribute('aria-label')).toBe(
    '删除文档：Decision: the home is the single writer',
  );
  one<HTMLButtonElement>('.kb .mem-meta .del').click();
  flushSync();
  expect(texts('dialog h2')).toEqual(['删除这个文档？']);
  expect(texts('dialog .lead')).toEqual(['这会把它从群的知识库里永久删除。']);
});
