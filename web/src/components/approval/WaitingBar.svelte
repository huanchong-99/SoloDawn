<script lang="ts">
  import { subject, type Group } from '../../lib/groups.svelte';
  import { jumpToMessage } from '../../lib/jump';
  import { t } from '../../lib/prefs.svelte';
  import Icon from '../Icon.svelte';

  /** Pinned above a group's main line while cards in this group wait on the reader. */
  let { group }: { group: Group } = $props();

  type Asked = 'brief' | 'approval' | 'decision';
</script>

{#if group.waiting.length}
  <nav class="needbar" aria-label={t('approval.waiting')}>
    <b>
      <Icon name="alert" sm />
      <span>{t('approval.waiting.count').replace('{n}', String(group.waiting.length))}</span>
    </b>
    {#each group.waiting as msg (msg.id)}
      {@const kind = msg.kind as Asked}
      <a href={`#m-${msg.id}`} onclick={(event) => jumpToMessage(event, msg.id, kind)}>
        {t(`approval.waiting.${kind}`)} · {subject(msg)}
      </a>
    {/each}
  </nav>
{/if}
