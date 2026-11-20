<script lang="ts">
  import type { CardProps } from '../../src/lib/mounts.svelte';

  // A stand-in for the card of a kind of message: it names the message it was given.
  let { group, msg }: CardProps = $props();
</script>

<div class="probe card-of">{msg.kind} {msg.id} in {group.name}</div>
