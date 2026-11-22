<script lang="ts">
  import { Group, Home } from '../../lib/groups.svelte';
  import { goldens } from '../../lib/mock';
  import { decode, Row, type Ctl, type HumanMemberRow } from '../../lib/proto';
  import KnowledgePanel from './KnowledgePanel.svelte';

  const owner = decode(Row, goldens['row-member-human']) as HumanMemberRow;
  const home = new Home({ user: owner.user, sign: async (p) => ({ p, k: 'demo', s: 'demo' }) });
  const group = new Group(home, owner.g);
  const document = decode(Row, goldens['row-note-kb']);
  group.apply([decode(Row, goldens['row-group']), owner]);
  let removed = false;
  home.ctl = {
    request: async (req) => {
      if (req.t === 'cmd') {
        const cmd = JSON.parse(req.env.p);
        removed = true;
        group.apply([{ r: 'note', id: cmd.id, seq: document.seq + 1, gone: true }]);
        return { re: 1, ok: true };
      }
      const matches = !('q' in req) || /home|writer|主机/u.test(req.q!);
      return { re: 1, ok: true, rows: !removed && matches ? [document] : [], more: false };
    },
  } as Ctl;
</script>

<KnowledgePanel {group} />
