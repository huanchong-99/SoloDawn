<script lang="ts" generics="P extends Point">
  import { mounts, type Point, type Points } from '../lib/mounts.svelte';

  /** Draws every component mounted at a point, in order, each given what the point gives. */
  let { at, props }: { at: P; props: Points[P] } = $props();
</script>

{#each mounts[at] as entry (entry)}
  <entry.component {...props} />
{/each}
