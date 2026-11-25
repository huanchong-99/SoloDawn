<script lang="ts" generics="P extends Point">
  import { mounts, type Point, type Points } from '../lib/mounts.svelte';

  /** Draws every component mounted at a point, in order, each given what the point gives. */
  let { at, props }: { at: P; props: Points[P] } = $props();
  const team = $derived('group' in props && props.group.info?.kind === 'team');
</script>

{#each mounts[at] as entry (entry)}
  {#if !team || !entry.project}<entry.component {...props} />{/if}
{/each}
