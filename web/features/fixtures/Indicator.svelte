<script lang="ts">
  import type { Net } from '../../src/lib/net.svelte';

  // A stand-in for a part of the top bar: it says where the homes come from.
  let { net }: { net: Net } = $props();
</script>

<span class="probe indicator">{net.source}</span>
