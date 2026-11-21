<script lang="ts">
  import type { CardProps } from '../../lib/mounts.svelte';
  import { t } from '../../lib/prefs.svelte';

  /** Files, diff counts and commits recorded when a terminal takeover hands back. */
  let { msg }: CardProps<'handback'> = $props();
  const body = $derived(msg.body);
  const count = $derived(
    body.files.length === 1
      ? t('thread.file.one')
      : t('thread.files').replace('{n}', String(body.files.length)),
  );
</script>

<div class="handback">
  <p class="meta">
    {count}
    {#each body.files as file (file)}
      · <code>{file}</code>
    {/each}
    ·
    <span class="mono">
      <span class="add">+{body.added}</span> <span class="del">−{body.removed}</span>
    </span>
  </p>
  {#if body.commits.length}
    <ul class="commits">
      {#each body.commits as commit (commit.sha)}
        <li><code>{commit.sha.slice(0, 7)}</code> {commit.subject}</li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .handback {
    display: grid;
    gap: 4px;
    font-size: var(--fs-xs);
    color: var(--text-2);
  }
  .meta {
    color: var(--text-3);
  }
  .commits {
    display: grid;
    gap: 2px;
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
