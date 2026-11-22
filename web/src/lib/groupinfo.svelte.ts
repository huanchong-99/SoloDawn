// Project-memory projections, paging and forget command (planning/contracts/rows.md sections 5
// and 6; planning/contracts/commands.md section 1.6; planning/contracts/datachannel.md section 4).
import { untrack } from 'svelte';
import { lost, Paging, type Group } from './groups.svelte';
import { isAdmin } from './members';
import { t } from './prefs.svelte';
import type * as p from './proto';

type ReviewRow = Extract<p.MsgRow, { kind: 'review' }>;
type MemoryKind = 'note' | 'review';

/** Row types accepted by the `forget` command (`commands.md` section 1.6). */
export type ForgetRowType = 'note' | 'msg';

const memoryPageSize = 50;

const isNote = (row: p.Row): row is p.NoteRow => row.r === 'note' && !('gone' in row);
const isReview = (row: p.Row): row is ReviewRow => (row as { kind?: string }).kind === 'review';

/** Live context notes held for `group`, ordered by `pos`. */
export function groupNotes(group: Group): p.NoteRow[] {
  return [...group.rows.values()].filter(isNote).sort((a, b) => a.pos - b.pos);
}

/** Live scored-review ledger entries held for `group`, ordered by `pos`. */
export function groupReviews(group: Group): ReviewRow[] {
  return [...group.rows.values()].filter(isReview).sort((a, b) => a.pos - b.pos);
}

/** Whether a note or message row is still live in `group.rows`. */
export function hasMemoryRow(group: Group, r: ForgetRowType, id: string): boolean {
  const row = group.rows.get(`${r} ${id}`);
  return Boolean(row && !('gone' in row));
}

/** Author display name for a note or review, falling back to former member. */
export const authorOf = (group: Group, by: string): string =>
  group.member(by)?.name ?? t('chat.gone');

/** Reason why deleting project memory is unavailable right now, or undefined when allowed. */
export function forgetDisabledReason(group: Group): string | undefined {
  if (!group.home.ctl) return t('groupinfo.forget.offline');
  if (!group.info) return undefined;
  return isAdmin(group) ? undefined : t('groupinfo.forget.forbidden');
}

/** Strips backticks and collapses whitespace of `text` to a preview of at most 60 characters. */
export function previewText(text: string): string {
  const oneLine = text.replace(/`/g, '').replace(/\s+/g, ' ').trim();
  return oneLine.length > 60 ? `${oneLine.slice(0, 59)}…` : oneLine;
}

/** Signs and sends `forget` for a note or review, returning the refusal code or an empty string. */
export async function forgetMemory(group: Group, r: ForgetRowType, id: string): Promise<string> {
  const answer = await group.home.command({ t: 'forget', g: group.g, r, id });
  return answer.err ? answer.err.code : '';
}

/** Fetches and pages `note` and `review` project-memory rows across sync epochs of `group`. */
export class MemoryView {
  readonly #group: Group;
  note = $state(new Paging());
  review = $state(new Paging());
  #epoch: object | undefined;

  constructor(group: Group) {
    this.#group = group;
  }

  /** Starts the initial `note` and `review` reads once per online sync epoch of `group`. */
  sync(): void {
    const ctl = this.#group.home.ctl;
    const epoch = this.#group.paging();
    if (!ctl || this.#epoch === epoch) return;
    this.#epoch = epoch;
    untrack(() => {
      this.note = new Paging();
      this.review = new Paging();
      void this.older('note');
      void this.older('review');
    });
  }

  #stale(ctl: p.Ctl, epoch: object): boolean {
    return ctl !== this.#group.home.ctl || epoch !== this.#group.paging();
  }

  /** Fetches the next older page of `kind` (`note` or `review`). */
  async older(kind: MemoryKind): Promise<void> {
    const page = this[kind];
    const ctl = this.#group.home.ctl;
    if (!ctl || !page.ready) return;
    const epoch = this.#group.paging();
    const before = page.before ?? 0;
    page.busy = true;
    const answer = await ctl
      .request({ t: 'fetch', g: this.#group.g, kind, before, limit: memoryPageSize })
      .catch(lost);
    page.busy = false;
    if (this.#stale(ctl, epoch)) return;
    if (answer.err) {
      page.failed = true;
      return;
    }
    this.#group.apply(answer.rows, false);
    page.more = answer.rows.length > 0 && answer.more;
    page.before = (answer.rows.at(-1) as { pos: number } | undefined)?.pos ?? before;
  }
}
