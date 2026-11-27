<script lang="ts">
  import { subject, type Group } from '../../lib/groups.svelte';
  import { jumpToMessage } from '../../lib/jump';
  import { t } from '../../lib/prefs.svelte';
  import Icon from '../Icon.svelte';

  /** Pinned above a group's main line while cards in this group wait on the reader. */
  let { group }: { group: Group } = $props();

  type Asked = 'brief' | 'approval' | 'decision' | 'confirm';
</script>

{#if group.waiting.length}
  {@const first = group.waiting[0]}
  <nav class="needbar" aria-label={t('approval.waiting')}>
    <b>
      <a
        class="lead"
        href={`#m-${first.id}`}
        onclick={(event) => jumpToMessage(event, first.id, first.kind as Asked)}
      >
        <Icon name="alert" sm />
        <span>{t('approval.waiting.count').replace('{n}', String(group.waiting.length))}</span>
      </a>
    </b>
    {#each group.waiting as msg (msg.id)}
      {@const kind = msg.kind as Asked}
      <a href={`#m-${msg.id}`} onclick={(event) => jumpToMessage(event, msg.id, kind)}>
        {t(`approval.waiting.${kind}`)} · {subject(group, msg)}
      </a>
    {/each}
  </nav>
{/if}
