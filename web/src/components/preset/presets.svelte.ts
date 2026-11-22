import { untrack } from 'svelte';
import { lost, newId, type Group } from '../../lib/groups.svelte';
import type { NoteRow, Row } from '../../lib/proto';

const isPreset = (row: Row): row is NoteRow =>
  row.r === 'note' && !('gone' in row) && row.type === 'preset';

export function presets(group: Group): NoteRow[] {
  return [...group.rows.values()].filter(isPreset).sort((a, b) => a.pos - b.pos);
}

export function canWrite(group: Group): boolean {
  return Boolean(group.home.ctl && group.me?.kind === 'human' && group.me.online);
}

export function validName(name: string): boolean {
  return name.length > 0 && [...name].length <= 80 && !/\p{Cc}/u.test(name);
}

export async function savePreset(group: Group, name: string, text: string): Promise<string> {
  const answer = await group.home.command({
    t: 'note',
    g: group.g,
    id: newId(),
    task: null,
    type: 'preset',
    name,
    text,
  });
  return answer.err ? answer.err.code : '';
}

export class PresetView {
  failed = $state('');
  busy = $state(false);
  loaded = $state(false);
  #epoch: object | undefined;

  constructor(readonly group: Group) {}

  sync(): void {
    const epoch = this.group.paging();
    const ctl = this.group.home.ctl;
    if (!ctl || epoch === this.#epoch) return;
    this.#epoch = epoch;
    untrack(() => {
      void this.fetch();
    });
  }

  async fetch(): Promise<void> {
    const ctl = this.group.home.ctl;
    if (!ctl) return;
    const epoch = this.group.paging();
    this.busy = true;
    this.failed = '';
    const answer = await ctl
      .request({
        t: 'fetch',
        g: this.group.g,
        kind: 'preset',
        before: 0,
        limit: 50,
      })
      .catch(lost);
    this.busy = false;
    if (ctl !== this.group.home.ctl || epoch !== this.group.paging()) return;
    if (answer.err) {
      this.failed = answer.err.code;
      return;
    }
    this.group.apply(answer.rows, false);
    this.loaded = true;
  }
}
