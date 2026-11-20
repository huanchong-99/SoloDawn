<script lang="ts">
  import type { Net } from '../../src/lib/net.svelte';

  // A stand-in for a banner or a layer of the shell.
  let { net }: { net: Net } = $props();
</script>

<p class="probe notice">notice · {net.source}</p>
