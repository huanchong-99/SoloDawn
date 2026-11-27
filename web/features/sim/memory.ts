// The project memory and the knowledge base of the demo world: the notes and documents the website
// group starts with, and what a home does with `note`, `note_edit` and `fetch` of a kind
// (contracts/commands.md 1.6; contracts/step-3/knowledge.md sections 2 and 3).
import { newId } from '../../src/lib/groups.svelte';
import { ids } from './ids';
import type { SimNode } from './node';
import { row, type Fields, type Row } from './rows';
import type { World } from './world';

const hour = 3_600_000;

const refused = (code: string, msg: string) => ({ err: { code, msg } });

const named = (each: Row, type: unknown, name: unknown) =>
  each.r === 'note' && each.type === type && each.name === name;

/** `note`: a context note is written; a typed one replaces the note of its type and name. */
export function notePut(world: World, payload: Fields) {
  const g = payload.g as string;
  const { type, name } = payload;
  const old = type ? world.rows(g).find((each) => named(each, type, name)) : undefined;
  const fields = { id: old?.id ?? payload.id, g, task: payload.task ?? null, type, name };
  world.write(g, row('row-note', { ...fields, text: payload.text, by: ids.me, ts: Date.now() }));
  return { ok: true };
}

/** `note_edit`: the live note keeps its id and place, and a name no other note of its type has. */
export function noteEdit(world: World, payload: Fields) {
  const g = payload.g as string;
  const note = world.find(g, 'note', payload.id as string);
  if (!note) return refused('not_found', 'the note does not exist');
  const others = world.rows(g).filter((each) => each.id !== note.id);
  if (note.type && others.some((each) => named(each, note.type, payload.name)))
    return refused('conflict', 'another note of that type has that name');
  world.write(g, { ...note, name: payload.name, text: payload.text, by: ids.me, ts: Date.now() });
  return { ok: true };
}

const kinds: Record<string, (each: Row) => boolean> = {
  note: (each) => each.r === 'note' && !each.type,
  preset: (each) => each.r === 'note' && each.type === 'preset',
  kb: (each) => each.r === 'note' && each.type === 'kb',
  review: (each) => each.r === 'msg' && each.kind === 'review',
};

/** A search finds the documents whose name or text holds one of its words. */
const found = (each: Row, q: string) =>
  q
    .toLowerCase()
    .split(/\s+/)
    .some((word) => word && `${each.name} ${each.text}`.toLowerCase().includes(word));

/** `fetch` of a kind: a page by `pos`, newest first, or what a search of documents finds. */
export function fetchKind(node: SimNode, req: Fields) {
  const held = [...(node.groups.get(req.g as string)?.rows.values() ?? [])] as unknown as Row[];
  const before = req.before as number;
  const scope = held
    .filter((each) => !each.gone && kinds[req.kind as string](each))
    .filter((each) =>
      req.q ? found(each, req.q as string) : !before || (each.pos as number) < before,
    )
    .sort((a, b) => (b.pos as number) - (a.pos as number));
  const rows = scope.slice(0, req.limit as number);
  return { t: 'fetched', re: 0, ok: true, rows, more: scope.length > rows.length };
}

/** The notes and documents of the website group, written over the last days. */
export function memory(world: World, g: string, now: number) {
  const note = (by: string, ago: number, text: string, kb?: string) =>
    world.write(
      g,
      row('row-note', {
        id: newId(),
        g,
        task: null,
        ...(kb ? { type: 'kb', name: kb } : {}),
        text,
        by,
        ts: now - ago * hour,
      }),
    );
  note(ids.claude, 23, '价格表的断点统一用 `640px`，和设计稿一致。');
  note(ids.codex, 20, '跑 Lighthouse 要用无痕窗口，浏览器插件会拉低分数。');
  note(ids.me, 1, '字体先不换，等下周设计定稿再说。');
  note(
    ids.secretary,
    40,
    '官网的页面都在构建时生成静态 HTML，部署到 CDN。原因：内容一周只改一两次，静态页最快，也不用维护服务器。',
    '决策：官网用静态生成',
  );
  note(
    ids.me,
    30,
    '提交信息用中文，一行说清改了什么；涉及页面的写上页面名，比如「首页：价格表改成纵向卡片」。',
    '约定：提交信息的写法',
  );
}
