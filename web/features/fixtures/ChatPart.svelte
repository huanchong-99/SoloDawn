<script lang="ts">
  import type { Group } from '../../src/lib/groups.svelte';

  // A stand-in for a part that an area mounts in a chat: it names the group it was given.
  let { group }: { group: Group } = $props();
</script>

<p class="probe part">{group.name}</p>
