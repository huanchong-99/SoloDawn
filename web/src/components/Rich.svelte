<script lang="ts">
  import { parts } from '../lib/compose';
  import type { Group } from '../lib/groups.svelte';

  /** The text of a message as it was written, with its code spans and the members it names
   *  marked. Only the members the row itself names are marked. It is drawn inline, inside the
   *  paragraph of whoever shows it. */
  let { group, text, mentions }: { group: Group; text: string; mentions: string[] } = $props();

  const segs = $derived(
    parts(
      text,
      group.members.filter((member) => mentions.includes(member.id)),
    ),
  );
</script>

{#each segs as part, i (i)}
  {#if part.kind === 'code'}<code>{part.text}</code>{:else if part.kind === 'at'}<span class="at">
      {part.text}
    </span>{:else}{part.text}{/if}
{/each}

<style>
  code {
    padding: 1px 5px;
    border-radius: var(--r-sm);
    background: var(--surface-2);
  }
</style>
