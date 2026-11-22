import { untrack } from 'svelte';
import { lost, type Group } from '../../lib/groups.svelte';
import type { Ctl, Fetched, NoteRow, Row } from '../../lib/proto';

const isDocument = (row: Row | undefined): row is NoteRow =>
  row?.r === 'note' && (row as NoteRow).type === 'kb';

export class Knowledge {
  busy = $state(false);
  failed = $state('');
  loaded = $state(false);
  more = $state(true);
  narrow = $state(false);
  ids = $state<string[]>();
  #before = 0;
  #epoch: object | undefined;
  #read = {};

  constructor(readonly group: Group) {}

  get documents(): NoteRow[] {
    if (this.ids) return this.ids.map((id) => this.group.rows.get(`note ${id}`)).filter(isDocument);
    return [...this.group.rows.values()].filter(isDocument).sort((a, b) => a.pos - b.pos);
  }

  sync(): void {
    const epoch = this.group.paging();
    if (!this.group.home.ctl || epoch === this.#epoch) return;
    this.#epoch = epoch;
    untrack(() => {
      this.clear();
      this.#before = 0;
      this.more = true;
      this.loaded = false;
      void this.older();
    });
  }

  clear(): void {
    this.#read = {};
    this.busy = false;
    this.failed = '';
    this.ids = undefined;
    this.narrow = false;
  }

  async older(): Promise<void> {
    if (this.busy || !this.more) return;
    const answer = await this.#fetch({ before: this.#before, limit: 50 });
    if (!answer) return;
    this.loaded = true;
    this.more = answer.more && answer.rows.length > 0;
    if (answer.rows.length > 0) this.#before = (answer.rows.at(-1) as NoteRow).pos;
  }

  async search(q: string): Promise<void> {
    this.ids = [];
    this.narrow = false;
    const answer = await this.#fetch({ before: 0, limit: 20, q });
    if (!answer) return;
    this.ids = answer.rows.map((row) => row.id);
    this.narrow = answer.more;
  }

  #stale(ctl: Ctl, epoch: object): boolean {
    return ctl !== this.group.home.ctl || epoch !== this.group.paging();
  }

  async #fetch(fields: {
    before: number;
    limit: number;
    q?: string;
  }): Promise<Fetched | undefined> {
    const ctl = this.group.home.ctl;
    if (!ctl) return;
    const epoch = this.group.paging();
    const read = (this.#read = {});
    this.busy = true;
    this.failed = '';
    const answer = await ctl
      .request({ t: 'fetch', g: this.group.g, kind: 'kb', ...fields })
      .catch(lost);
    if (read !== this.#read) return;
    this.busy = false;
    if (this.#stale(ctl, epoch)) return;
    if (answer.err) {
      this.failed = answer.err.code;
      return;
    }
    this.group.apply(answer.rows, false);
    return answer;
  }
}
