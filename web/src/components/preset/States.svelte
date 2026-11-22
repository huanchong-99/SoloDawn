<script lang="ts">
  import { Group, Home } from '../../lib/groups.svelte';
  import { goldens } from '../../lib/mock';
  import {
    decode,
    Row,
    type Ctl,
    type NoteRow,
    type HumanMemberRow,
    type MsgRow,
  } from '../../lib/proto';
  import { t } from '../../lib/prefs.svelte';
  import BriefCard from '../brief/BriefCard.svelte';
  import PresetPanel from './PresetPanel.svelte';
  import { presets } from './presets.svelte';

  const owner = decode(Row, goldens['row-member-human']) as HumanMemberRow;
  const home = new Home({ user: owner.user, sign: async (p) => ({ p, k: 'demo', s: 'demo' }) });
  const group = new Group(home, owner.g);
  const note = decode(Row, goldens['row-note-preset']) as NoteRow;
  const brief = decode(Row, goldens['row-msg-brief-preset']) as Extract<MsgRow, { kind: 'brief' }>;
  group.apply([decode(Row, goldens['row-group']), owner, note]);
  let seq = 10000;
  home.ctl = {
    request: async (req) => {
      if (req.t === 'cmd') {
        const cmd = JSON.parse(req.env.p);
        if (cmd.t === 'forget') group.apply([{ r: 'note', id: cmd.id, seq: ++seq, gone: true }]);
        if (cmd.t === 'note') {
          const id = presets(group).find((row) => row.name === cmd.name)?.id ?? cmd.id;
          group.apply([
            { ...note, ...cmd, id, r: 'note', by: owner.id, ts: Date.now(), seq: ++seq },
          ]);
        }
        return { re: 1, ok: true };
      }
      return { re: 1, ok: true, rows: [], more: false };
    },
  } as Ctl;
</script>

<section class="kind" aria-label={t('preset.title')}>
  <h3>{t('preset.title')}</h3>
  <BriefCard {group} msg={brief} />
  <PresetPanel {group} />
</section>
