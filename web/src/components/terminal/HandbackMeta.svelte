<script lang="ts">
  import type { CardProps } from '../../lib/mounts.svelte';
  import { handbackFilesLabel } from '../../lib/terminal';

  /** Metadata summary for a terminal `handback` message: changed files, commits and diff stats. */
  let { msg }: CardProps<'handback'> = $props();
  const body = $derived(msg.body);
  const files = $derived(handbackFilesLabel(body.files.length));
</script>

<div class="handback">
  <p class="meta">
    <span>{files}</span>
    {#if body.files.length > 0 || body.added > 0 || body.removed > 0}
      {' · '}
      <span class="mono">
        <span class="add">+{body.added}</span> <span class="del">−{body.removed}</span>
      </span>
    {/if}
  </p>
  {#if body.files.length > 0}
    <p class="files">
      {#each body.files as file (file)}<code>{file}</code>{/each}
    </p>
  {/if}
  {#if body.commits.length > 0}
    <ul class="commits">
      {#each body.commits as commit (commit.sha)}
        <li><code>{commit.sha.slice(0, 7)}</code> <span>{commit.subject}</span></li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .handback {
    display: grid;
    gap: 6px;
    font-size: var(--fs-xs);
  }
  .meta {
    color: var(--text-3);
  }
  .files {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
  .commits {
    display: grid;
    gap: 4px;
    color: var(--text-2);
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
