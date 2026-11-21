<script lang="ts">
  import { prefersReducedMotion } from 'svelte/motion';
  import { subject, type Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Icon from '../Icon.svelte';

  /** Pinned above a group's main line while cards in this group wait on the reader. */
  let { group }: { group: Group } = $props();

  type Asked = 'brief' | 'approval' | 'decision';

  function jump(event: MouseEvent, id: string) {
    event.preventDefault();
    const item = document.getElementById(`m-${id}`);
    if (!item) return;
    item.scrollIntoView({
      behavior: prefersReducedMotion.current ? 'auto' : 'smooth',
      block: 'nearest',
    });
    const action = item.querySelector<HTMLElement>(
      'button:not([disabled]), input:not([disabled]), textarea:not([disabled])',
    );
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
      <a href={`#m-${msg.id}`} onclick={(event) => jump(event, msg.id)}>
        <span class="cut">{t(`approval.waiting.${kind}`)} · {subject(msg)}</span>
      </a>
    {/each}
  </nav>
{/if}

<style>
  .needbar {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    padding: 8px var(--sp-4);
    border-bottom: 1px solid color-mix(in srgb, var(--dawn) 40%, var(--line));
    background: var(--dawn-soft);
    font-size: var(--fs-sm);
    white-space: nowrap;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .needbar b {
    display: inline-flex;
    align-items: center;
    flex: none;
    gap: 6px;
    color: var(--dawn-text);
  }
  .needbar a {
    display: inline-flex;
    align-items: center;
    flex: none;
    max-width: 280px;
    height: 26px;
    padding: 0 10px;
    border: 1px solid color-mix(in srgb, var(--dawn) 40%, var(--line));
    border-radius: var(--r-full);
    background: var(--surface);
    color: var(--text);
  }
</style>
