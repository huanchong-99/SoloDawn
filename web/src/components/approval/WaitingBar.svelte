<script lang="ts">
  import { prefersReducedMotion } from 'svelte/motion';
  import { subject, type Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Icon from '../Icon.svelte';

  /** Pinned above a group's main line while cards in this group wait on the reader. */
  let { group }: { group: Group } = $props();

  type Asked = 'brief' | 'approval' | 'decision';

  function jump(event: MouseEvent, id: string, kind: Asked) {
    event.preventDefault();
    const item = document.getElementById(`m-${id}`);
    if (!item) return;
    item.scrollIntoView({
      behavior: prefersReducedMotion.current ? 'auto' : 'smooth',
      block: 'nearest',
    });
    const selector =
      kind === 'decision'
        ? 'input:not([disabled]), textarea:not([disabled])'
        : 'button:not([disabled])';
    const action = item.querySelector<HTMLElement>(selector);
    if (action) {
      action.focus({ preventScroll: true });
      return;
    }
    if (!item.hasAttribute('tabindex')) item.setAttribute('tabindex', '-1');
    item.focus({ preventScroll: true });
  }
</script>

{#if group.waiting.length}
  <nav class="needbar" aria-label={t('approval.waiting')}>
    <b>
      <Icon name="alert" sm />
      <span>{t('approval.waiting.count').replace('{n}', String(group.waiting.length))}</span>
    </b>
    {#each group.waiting as msg (msg.id)}
      {@const kind = msg.kind as Asked}
      <a href={`#m-${msg.id}`} onclick={(event) => jump(event, msg.id, kind)}>
        {t(`approval.waiting.${kind}`)} · {subject(msg)}
      </a>
    {/each}
  </nav>
{/if}
