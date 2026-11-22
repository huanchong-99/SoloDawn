<script lang="ts">
  import type { CardProps } from '../../lib/mounts.svelte';
  import { t } from '../../lib/prefs.svelte';

  /** Compact metadata line for a task completion report. */
  let { msg }: CardProps<'report'> = $props();
  const body = $derived(msg.body);
  const files = $derived(
    body.files === 1 ? t('thread.file.one') : t('thread.files').replace('{n}', String(body.files)),
  );
</script>

<p class="meta">
  {#if body.pr !== undefined}<span class="mono">PR #{body.pr}</span> ·{/if}
  <code>{body.commit.slice(0, 7)}</code> · {files} ·
  <span class="mono">
    <span class="add">+{body.added}</span> <span class="del">−{body.removed}</span>
  </span>
</p>

<style>
  .meta {
    color: var(--text-3);
    font-size: var(--fs-xs);
  }
  code {
    padding: 1px 5px;
    border-radius: var(--r-sm);
    background: var(--surface-2);
    color: var(--text-2);
  }
  .add {
    color: var(--ok);
  }
  .del {
    color: var(--danger);
  }
</style>
